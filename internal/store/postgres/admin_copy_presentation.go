package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/nathanxiangang-web/panta/internal/adminops"
	"github.com/nathanxiangang-web/panta/internal/adminread"
	"github.com/nathanxiangang-web/panta/internal/copypresentation"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var _ adminops.CopyPresentationApplier = (*AdminOperationsRepository)(nil)
var _ adminops.CopyPresentationReader = (*AdminOperationsRepository)(nil)

// Lock order: actor -> session -> action(write only) -> Binding -> projection
// cursor -> Copy -> overlay. Projector takes cursor then Copy; never the reverse.
// The cursor fence proves an existing PANTA projection, not a fresh Canonical read.
func lockedCopyPresentation(ctx context.Context, tx pgx.Tx, id, binding string, bindings []string, write bool) (copypresentation.Detail, error) {
	d := copypresentation.Detail{Source: "PANTA_COPY_PROJECTION", VisibilityEffect: "PRESENTATION_ONLY_NO_ACCESS_CONTROL", EmptyNameFallback: "CURRENT_CANONICAL_DISPLAY"}
	if !adminread.ValidID(id) || !adminread.ValidID(binding) || len(bindings) < 1 || len(bindings) > 64 {
		return d, adminops.ErrInvalid
	}
	allowed := false
	for _, b := range bindings {
		if !adminread.ValidID(b) {
			return d, adminops.ErrInvalid
		}
		if b == binding {
			allowed = true
		}
	}
	if !allowed {
		return d, adminops.ErrNotFound
	}
	var root string
	e := tx.QueryRow(ctx, `SELECT indexcore_root_id FROM storage_bindings WHERE storage_binding_id=$1 FOR SHARE`, binding).Scan(&root)
	if errors.Is(e, pgx.ErrNoRows) {
		return d, adminops.ErrNotFound
	}
	if e != nil {
		return d, e
	}
	e = tx.QueryRow(ctx, `SELECT last_event_seq FROM indexcore_projection_cursors WHERE storage_binding_id=$1 FOR SHARE`, binding).Scan(&d.Evidence.ProjectionCursor)
	if errors.Is(e, pgx.ErrNoRows) {
		return d, adminops.ErrNotFound
	}
	if e != nil {
		return d, e
	}
	lock := " FOR SHARE"
	if write {
		lock = " FOR UPDATE"
	}
	e = tx.QueryRow(ctx, `SELECT copy_id::text,storage_binding_id::text,indexcore_root_id,indexcore_resource_id,availability,updated_at FROM copies WHERE copy_id=$1 AND storage_binding_id=$2 AND indexcore_root_id=$3`+lock, id, binding, root).Scan(&d.Evidence.CopyID, &d.Evidence.BindingID, &d.Evidence.RootID, &d.Evidence.ResourceID, &d.Evidence.Availability, &d.Evidence.CopyUpdatedAt)
	if errors.Is(e, pgx.ErrNoRows) {
		return d, adminops.ErrNotFound
	}
	if e != nil {
		return d, e
	}
	validIdentity := func(s string) bool {
		if !utf8.ValidString(s) || strings.TrimSpace(s) == "" || len(s) > 1024 {
			return false
		}
		for _, r := range s {
			if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '<' || r == '>' {
				return false
			}
		}
		return true
	}
	if d.Evidence.ProjectionCursor < 1 || !validIdentity(root) || !validIdentity(d.Evidence.ResourceID) || d.Evidence.CopyUpdatedAt.IsZero() || d.Evidence.Availability != "PRESENT" && d.Evidence.Availability != "REMOVED" {
		return d, adminops.ErrUnavailable
	}
	d.Presentation = copypresentation.Overlay{Category: "AUTO"}
	var storedBinding string
	e = tx.QueryRow(ctx, `SELECT binding_id::text,display_name,category,hidden,admin_version,updated_at,updated_by::text FROM product_copy_presentation WHERE copy_id=$1`+lock, id).Scan(&storedBinding, &d.Presentation.DisplayName, &d.Presentation.Category, &d.Presentation.Hidden, &d.Presentation.Version, &d.Presentation.UpdatedAt, &d.Presentation.UpdatedBy)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return d, e
	}
	if e == nil && storedBinding != binding || !d.Presentation.Valid() {
		return d, adminops.ErrUnavailable
	}
	d.CanEdit = d.Evidence.Availability == "PRESENT" && d.Presentation.Version < copypresentation.MaxVersion
	e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&d.ReadAt)
	return d, e
}

func (r *AdminOperationsRepository) ReadCopyPresentation(ctx context.Context, a adminops.Actor, id, binding string, bindings []string) (copypresentation.Detail, error) {
	tx, e := r.begin(ctx, a)
	if e != nil {
		return copypresentation.Detail{}, e
	}
	defer adminRollback(tx)
	d, e := lockedCopyPresentation(ctx, tx, id, binding, bindings, false)
	if e != nil {
		return copypresentation.Detail{}, e
	}
	if e = adminActor(ctx, tx, a); e != nil {
		return copypresentation.Detail{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return copypresentation.Detail{}, e
	}
	return d, nil
}

func (r *AdminOperationsRepository) ApplyCopyPresentation(ctx context.Context, a adminops.Actor, c adminops.Command, bindings []string) (adminops.Result, error) {
	if !c.Valid() || c.Action != adminops.SetCopyPresentation {
		return adminops.Result{}, adminops.ErrInvalid
	}
	tx, e := r.begin(ctx, a)
	if e != nil {
		return adminops.Result{}, e
	}
	defer adminRollback(tx)
	result, done, e := lockedAdminAction(ctx, tx, a, c)
	if e != nil || done {
		return result, e
	}
	d, e := lockedCopyPresentation(ctx, tx, c.TargetID, c.Presentation.BindingID, bindings, true)
	if e != nil && !errors.Is(e, adminops.ErrNotFound) {
		return adminops.Result{}, e
	}
	result = rejected(c, "COPY_NOT_FOUND")
	if e == nil {
		if r.afterCopyPresentationLock != nil {
			if e = r.afterCopyPresentationLock(ctx, tx); e != nil {
				return adminops.Result{}, e
			}
		}
		if e = adminActor(ctx, tx, a); e != nil {
			return adminops.Result{}, e
		}
		switch {
		case !d.CanEdit:
			result.Code = "COPY_UNAVAILABLE"
		case d.Presentation.Version != c.ExpectedVersion:
			result.Code = "VERSION_CONFLICT"
		default:
			next := c.ExpectedVersion + 1
			tag, e := tx.Exec(ctx, `INSERT INTO product_copy_presentation(copy_id,binding_id,display_name,category,hidden,admin_version,updated_at,updated_by) VALUES($1,$2,$3,$4,$5,1,clock_timestamp(),$6) ON CONFLICT(copy_id) DO UPDATE SET display_name=EXCLUDED.display_name,category=EXCLUDED.category,hidden=EXCLUDED.hidden,admin_version=product_copy_presentation.admin_version+1,updated_at=GREATEST(clock_timestamp(),product_copy_presentation.updated_at+interval '1 microsecond'),updated_by=EXCLUDED.updated_by WHERE product_copy_presentation.binding_id=EXCLUDED.binding_id AND product_copy_presentation.admin_version=$7`, c.TargetID, c.Presentation.BindingID, c.Presentation.DisplayName, c.Presentation.Category, c.Presentation.Hidden, string(a.Principal.UserID), c.ExpectedVersion)
			if e != nil {
				return adminops.Result{}, e
			}
			if tag.RowsAffected() != 1 {
				return adminops.Result{}, adminops.ErrConflict
			}
			result.Status = "SUCCEEDED"
			result.Code = "COPY_PRESENTATION_SET"
			result.PresentationVersion = &next
		}
	}
	if r.beforeUserCommit != nil {
		if e = r.beforeUserCommit(ctx, tx); e != nil {
			return adminops.Result{}, e
		}
	}
	if e = adminActor(ctx, tx, a); e != nil {
		return adminops.Result{}, e
	}
	// Only version + safe code are audited. Alias/category/hidden are not receipts.
	// Failed/ambiguous Commit leaves the caller in status-only reconciliation.
	return appendAdminResult(ctx, tx, a, result)
}

// The generic status endpoint also uses this allowlist for this specific action.
// Never let corrupt receipts smuggle an alias, user/account data or a fake result.
func validCopyPresentationReceipt(raw []byte, h, r adminops.Result, at *time.Time) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return false
	}
	for k := range fields {
		switch k {
		case "request_id", "action", "target_id", "status", "code", "recorded_at", "presentation_version":
		default:
			return false
		}
	}
	if r.RequestID != h.RequestID || r.Action != adminops.SetCopyPresentation || r.TargetID != h.TargetID || !adminread.ValidID(r.TargetID) || at == nil || !r.RecordedAt.Equal(*at) || r.RecordedAt.Before(h.RecordedAt) {
		return false
	}
	if r.Status == "SUCCEEDED" {
		return r.Code == "COPY_PRESENTATION_SET" && r.PresentationVersion != nil && *r.PresentationVersion >= 1 && *r.PresentationVersion <= copypresentation.MaxVersion
	}
	return r.Status == "REJECTED" && r.PresentationVersion == nil && (r.Code == "COPY_NOT_FOUND" || r.Code == "COPY_UNAVAILABLE" || r.Code == "VERSION_CONFLICT")
}

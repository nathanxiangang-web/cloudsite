package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/product"
)

func TestPostgresProductOperationLookupIsSourceFreeAndOwnerBound(t *testing.T) {
	f := newAdmissionFixture(t)
	if _, err := f.store.Operation(f.ctx, f.p, "missing"); !errors.Is(err, product.ErrNotFound) {
		t.Fatal(err)
	}
	assertAdmissionCounts(t, f, 0, 0, 0)
	f.activation.afterJobInsert = func(context.Context, pgx.Tx) error { return errors.New("controlled activation interruption") }
	pending, err := f.service.Admit(f.ctx, f.p, f.input)
	if err != nil {
		t.Fatal(err)
	}
	found, err := f.store.Operation(f.ctx, f.p, f.input.OperationKey)
	if err != nil || found.ManifestID != pending.ManifestID || found.JobID != pending.JobID || found.Status != acquisition.SubmissionPendingActivation {
		t.Fatal("pending operation missing", err)
	}
	if _, err = f.store.Operation(f.ctx, f.other, f.input.OperationKey); !errors.Is(err, product.ErrNotFound) {
		t.Fatal("cross-owner lookup")
	}
	forged := f.p
	forged.Role = auth.RoleAdmin
	if _, err = f.store.Operation(f.ctx, forged, f.input.OperationKey); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatal("stale principal")
	}
	if _, err = f.store.Operation(f.ctx, f.p, "bad/key"); !errors.Is(err, product.ErrInvalid) {
		t.Fatal("invalid key")
	}
	f.activation.afterJobInsert = nil
	active, err := f.service.Admit(f.ctx, f.p, f.input)
	if err != nil {
		t.Fatal(err)
	}
	found, err = f.store.Operation(f.ctx, f.p, f.input.OperationKey)
	if err != nil || found.ManifestID != active.ManifestID || found.Status != acquisition.SubmissionReplay {
		t.Fatal("active lookup", err)
	}
	assertAdmissionCounts(t, f, 1, 1, 1)
	if _, err = f.pool.Exec(f.ctx, `UPDATE product_users SET enabled=false WHERE user_id=$1`, string(f.p.UserID)); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.Operation(f.ctx, f.p, f.input.OperationKey); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatal("disabled user lookup")
	}
}

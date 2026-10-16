package postgres

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/jobs"
	"github.com/nathanxiangang-web/panta/internal/product"
)

var _ product.OperationReader = (*ProductAdmissionRepository)(nil)

func (r *ProductAdmissionRepository) Operation(ctx context.Context, p auth.Principal, key string) (product.AdmissionDTO, error) {
	if r == nil || r.pool == nil {
		return product.AdmissionDTO{}, product.ErrUnavailable
	}
	if !p.Valid() {
		return product.AdmissionDTO{}, auth.ErrUnauthorized
	}
	if !product.ValidOperationKey(key) {
		return product.AdmissionDTO{}, product.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, product.ReadTimeout)
	defer cancel()
	var login, role string
	var enabled bool
	var mid, jid, owner, linked, state *string
	err := r.pool.QueryRow(ctx, `SELECT u.login,u.role,u.enabled,o.manifest_id::text,o.reserved_job_id::text,m.user_id::text,m.job_id::text,m.state
 FROM product_users u LEFT JOIN product_operations o ON o.user_id=u.user_id AND o.operation_key=$2
 LEFT JOIN acquisition_manifests m ON m.manifest_id=o.manifest_id WHERE u.user_id=$1`, string(p.UserID), key).Scan(&login, &role, &enabled, &mid, &jid, &owner, &linked, &state)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && (!enabled || login != p.Login || role != string(p.Role)) {
		return product.AdmissionDTO{}, auth.ErrUnauthorized
	}
	if err != nil {
		return product.AdmissionDTO{}, admissionPersistenceError(ctx, err)
	}
	if mid == nil {
		return product.AdmissionDTO{}, product.ErrNotFound
	}
	if jid == nil || owner == nil || *owner != string(p.UserID) || state == nil || !acquisition.State(*state).Valid() || !product.ValidID(*mid) || !product.ValidID(*jid) {
		return product.AdmissionDTO{}, product.ErrUnavailable
	}
	status := acquisition.SubmissionReplay
	if acquisition.State(*state) == acquisition.StatePending {
		if linked != nil {
			return product.AdmissionDTO{}, product.ErrUnavailable
		}
		status = acquisition.SubmissionPendingActivation
	} else if linked == nil || *linked != *jid {
		return product.AdmissionDTO{}, product.ErrUnavailable
	}
	return product.AdmissionDTO{ManifestID: acquisition.ManifestID(*mid), JobID: jobs.JobID(*jid), Status: status}, nil
}

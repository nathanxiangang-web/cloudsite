package postgres

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/jobs"
	"github.com/nathanxiangang-web/panta/internal/product"
)

type ProductAdmissionRepository struct {
	pool *pgxpool.Pool
	// Fault seams are private to isolated PostgreSQL tests, never runtime options.
	afterManifest func(context.Context, pgx.Tx) error
	commit        func(context.Context, pgx.Tx) error
}

var _ product.AdmissionStore = (*ProductAdmissionRepository)(nil)

func NewProductAdmissionRepository(pool *pgxpool.Pool) (*ProductAdmissionRepository, error) {
	if pool == nil {
		return nil, product.ErrUnavailable
	}
	return &ProductAdmissionRepository{pool: pool}, nil
}
func (r *ProductAdmissionRepository) Reserve(ctx context.Context, p auth.Principal, input product.AdmissionRequest, candidate product.Reservation) (product.Reservation, error) {
	if r == nil || r.pool == nil {
		return product.Reservation{}, product.ErrUnavailable
	}
	if !p.Valid() {
		return product.Reservation{}, auth.ErrUnauthorized
	}
	digest, err := product.AdmissionDigest(input)
	if err != nil || !product.ValidID(string(candidate.ManifestID)) || !product.ValidID(string(candidate.JobID)) || candidate.MaxAttempts != product.AdmissionMaxAttempts {
		return product.Reservation{}, product.ErrInvalid
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return product.Reservation{}, admissionPersistenceError(ctx, err)
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	var login, role string
	var enabled bool
	err = tx.QueryRow(ctx, `SELECT login,role,enabled FROM product_users WHERE user_id=$1 FOR SHARE`, string(p.UserID)).Scan(&login, &role, &enabled)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && (!enabled || login != p.Login || role != string(p.Role)) {
		return product.Reservation{}, auth.ErrUnauthorized
	}
	if err != nil {
		return product.Reservation{}, admissionPersistenceError(ctx, err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO product_operation_quotas(user_id,operation_limit,concurrency_limit) VALUES($1,$2,$3) ON CONFLICT(user_id) DO NOTHING`, string(p.UserID), product.DefaultOperationLimit, product.DefaultConcurrencyLimit)
	if err != nil {
		return product.Reservation{}, admissionPersistenceError(ctx, err)
	}
	var limit, concurrency int
	err = tx.QueryRow(ctx, `SELECT operation_limit,concurrency_limit FROM product_operation_quotas WHERE user_id=$1 FOR UPDATE`, string(p.UserID)).Scan(&limit, &concurrency)
	if err != nil {
		return product.Reservation{}, admissionPersistenceError(ctx, err)
	}
	existing, err := lookupAdmission(ctx, tx, p, input, digest)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return product.Reservation{}, err
	}
	var used, inFlight int
	err = tx.QueryRow(ctx, `SELECT count(*)::int,count(*) FILTER(WHERE m.state NOT IN ('READY','FAILED','CANCELED'))::int FROM product_operations o JOIN acquisition_manifests m ON m.manifest_id=o.manifest_id WHERE o.user_id=$1`, string(p.UserID)).Scan(&used, &inFlight)
	if err != nil {
		return product.Reservation{}, admissionPersistenceError(ctx, err)
	}
	if used >= limit {
		return product.Reservation{}, product.ErrOperationLimit
	}
	if inFlight >= concurrency {
		return product.Reservation{}, product.ErrConcurrencyLimit
	}
	// Prepare only a new operation, after replay lookup. Stored policy results
	// survive later policy changes, topology disablement, retries and claims.
	prepared, err := acquisition.PrepareSource(input.Source, acquisition.MagnetBTIHNameV1)
	if err != nil {
		return product.Reservation{}, product.ErrInvalid
	}
	candidate.Source, candidate.Policy = prepared.Reference.Value, prepared.Policy
	// Lock topology before delegating all acquisition-specific validation.
	var connection string
	err = tx.QueryRow(ctx, `SELECT storage_connection_id::text FROM storage_bindings WHERE storage_binding_id=$1 FOR SHARE`, string(input.BindingID)).Scan(&connection)
	if errors.Is(err, pgx.ErrNoRows) {
		return product.Reservation{}, product.ErrNotFound
	}
	if err != nil {
		return product.Reservation{}, admissionPersistenceError(ctx, err)
	}
	var lockedConnection string
	err = tx.QueryRow(ctx, `SELECT storage_connection_id::text FROM storage_connections WHERE storage_connection_id=$1 FOR SHARE`, connection).Scan(&lockedConnection)
	if err != nil {
		return product.Reservation{}, admissionPersistenceError(ctx, err)
	}
	topology, _ := NewStorageRepository(tx)
	identities, _ := NewCatalogRepository(tx)
	manifests, _ := NewAcquisitionManifestRepository(tx)
	var dbNow time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&dbNow); err != nil {
		return product.Reservation{}, admissionPersistenceError(ctx, err)
	}
	creator, _ := acquisition.NewService(topology, identities, manifests, acquisition.WithClock(func() time.Time { return dbNow }))
	source := prepared.Reference
	owner := acquisition.UserID(p.UserID)
	_, err = creator.CreateManifest(ctx, acquisition.CreateManifestRequest{ID: candidate.ManifestID, UserID: &owner, SourceType: source.Scheme, SourceRef: source.Value, ExpectedName: input.ExpectedName, TargetStorageBindingID: input.BindingID, TargetPath: input.TargetPath})
	if err != nil {
		for _, invalid := range []error{acquisition.ErrStorageBindingDisabled, acquisition.ErrStorageBindingNotAcquisitionCapable, acquisition.ErrStorageConnectionDisabled, acquisition.ErrInvalidProviderIdentity, acquisition.ErrStorageTopologyMismatch} {
			if errors.Is(err, invalid) {
				return product.Reservation{}, product.ErrNotFound
			}
		}
		return product.Reservation{}, admissionPersistenceError(ctx, err)
	}
	if r.afterManifest != nil {
		if err = r.afterManifest(ctx, tx); err != nil {
			return product.Reservation{}, admissionPersistenceError(ctx, err)
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO product_operations(user_id,operation_key,intent_digest,manifest_id,reserved_job_id,max_attempts,created_at,original_source,execution_source,source_policy) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, string(p.UserID), input.OperationKey, digest[:], string(candidate.ManifestID), string(candidate.JobID), candidate.MaxAttempts, dbNow, prepared.Original, candidate.Source, string(candidate.Policy))
	if err != nil {
		return product.Reservation{}, admissionPersistenceError(ctx, err)
	}
	if r.commit != nil {
		err = r.commit(ctx, tx)
	} else {
		err = tx.Commit(ctx)
	}
	if err == nil {
		candidate.Created = true
		return candidate, nil
	}
	// Never infer rollback from a failed Commit. Observe the immutable operation
	// on a new connection even when the request context has been canceled.
	readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	// Rollback is a no-op after a successful/ambiguous commit; if a test or driver
	// has left the transaction open it must release locks before evidence lookup.
	_ = tx.Rollback(readCtx)
	observed, readErr := lookupAdmission(readCtx, r.pool, p, input, digest)
	if readErr != nil {
		return product.Reservation{}, product.ErrAdmissionUncertain
	}
	return observed, nil
}
func lookupAdmission(ctx context.Context, db acquisitionManifestDB, p auth.Principal, input product.AdmissionRequest, digest [32]byte) (product.Reservation, error) {
	var result product.Reservation
	var stored []byte
	var manifestID, jobID string
	var original, policy string
	err := db.QueryRow(ctx, `SELECT intent_digest,manifest_id::text,reserved_job_id::text,max_attempts,original_source,execution_source,source_policy FROM product_operations WHERE user_id=$1 AND operation_key=$2`, string(p.UserID), input.OperationKey).Scan(&stored, &manifestID, &jobID, &result.MaxAttempts, &original, &result.Source, &policy)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return product.Reservation{}, err
		}
		return product.Reservation{}, admissionPersistenceError(ctx, err)
	}
	if !bytes.Equal(stored, digest[:]) {
		return product.Reservation{}, product.ErrOperationConflict
	}
	result.Policy = acquisition.SourcePolicy(policy)
	if original != input.Source || !product.ValidAdmissionExecution(result.Source, result.Policy) {
		return product.Reservation{}, product.ErrAdmissionUncertain
	}
	result.ManifestID = acquisition.ManifestID(manifestID)
	result.JobID = jobs.JobID(jobID)
	manifests, _ := NewAcquisitionManifestRepository(db)
	m, err := manifests.GetManifest(ctx, result.ManifestID)
	if err != nil || m.UserID == nil || string(*m.UserID) != string(p.UserID) || m.AssetID != nil || m.ReleaseID != nil || m.VariantID != nil || result.MaxAttempts != product.AdmissionMaxAttempts {
		return product.Reservation{}, product.ErrAdmissionUncertain
	}
	actual, err := product.AdmissionDigest(product.AdmissionRequest{OperationKey: input.OperationKey, Source: original, BindingID: m.TargetStorageBindingID, TargetPath: m.TargetPath, ExpectedName: m.ExpectedName})
	source, sourceErr := acquisition.ResolveSource(m.SourceRef)
	if err != nil || sourceErr != nil || m.SourceType != source.Scheme || m.SourceRef != result.Source || actual != digest || !m.State.Valid() || m.State == acquisition.StatePending && m.JobID != nil || m.State != acquisition.StatePending && (m.JobID == nil || *m.JobID != result.JobID) {
		return product.Reservation{}, product.ErrAdmissionUncertain
	}
	return result, nil
}
func (r *ProductAdmissionRepository) Usage(ctx context.Context, p auth.Principal) (product.UsageDTO, error) {
	if r == nil || r.pool == nil {
		return product.UsageDTO{}, product.ErrUnavailable
	}
	if !p.Valid() {
		return product.UsageDTO{}, auth.ErrUnauthorized
	}
	result := product.UsageDTO{Unit: "OPERATION", Price: "ZERO"}
	err := r.pool.QueryRow(ctx, `SELECT COALESCE(q.operation_limit,$4),COALESCE(q.concurrency_limit,$5),count(o.operation_key)::int,count(o.operation_key) FILTER(WHERE m.state NOT IN ('READY','FAILED','CANCELED'))::int
 FROM product_users u LEFT JOIN product_operation_quotas q ON q.user_id=u.user_id LEFT JOIN product_operations o ON o.user_id=u.user_id LEFT JOIN acquisition_manifests m ON m.manifest_id=o.manifest_id
 WHERE u.user_id=$1 AND u.login=$2 AND u.role=$3 AND u.enabled GROUP BY u.user_id,q.operation_limit,q.concurrency_limit`, string(p.UserID), p.Login, string(p.Role), product.DefaultOperationLimit, product.DefaultConcurrencyLimit).Scan(&result.OperationLimit, &result.ConcurrencyLimit, &result.Used, &result.InFlight)
	if errors.Is(err, pgx.ErrNoRows) {
		return product.UsageDTO{}, auth.ErrUnauthorized
	}
	if err != nil {
		return product.UsageDTO{}, admissionPersistenceError(ctx, err)
	}
	result.Remaining = max(0, result.OperationLimit-result.Used)
	return result, nil
}
func admissionPersistenceError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return product.ErrUnavailable
}

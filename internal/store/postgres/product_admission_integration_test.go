package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/jobs"
	"github.com/nathanxiangang-web/panta/internal/product"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

type admissionFixture struct {
	ctx        context.Context
	pool       *pgxpool.Pool
	p, other   auth.Principal
	input      product.AdmissionRequest
	store      *ProductAdmissionRepository
	activation *AcquisitionActivationRepository
	service    *product.AdmissionService
	manifests  *AcquisitionManifestRepository
}

func newAdmissionFixture(t *testing.T) admissionFixture {
	t.Helper()
	ctx := context.Background()
	pool := integrationPool(t, ctx)
	resetTestSchema(t, ctx, pool)
	applyIntegrationMigrations(t, ctx, pool)
	repo, _ := NewAuthRepository(pool)
	accounts, _ := auth.NewService(repo)
	p, err := accounts.Provision(ctx, "admission-user", "synthetic-password", auth.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	other, err := accounts.Provision(ctx, "admission-admin", "synthetic-password", auth.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	binding := storage.BindingID("98000000-0000-4000-8000-000000000001")
	seedStorageBinding(t, ctx, pool, binding, "admission-root")
	if _, err = pool.Exec(ctx, `UPDATE storage_bindings SET provider_scope='synthetic-save-dir' WHERE storage_binding_id=$1`, string(binding)); err != nil {
		t.Fatal(err)
	}
	topology, _ := NewStorageRepository(pool)
	catalog, _ := NewCatalogRepository(pool)
	manifests, _ := NewAcquisitionManifestRepository(pool)
	creator, _ := acquisition.NewService(topology, catalog, manifests)
	activation, _ := NewAcquisitionActivationRepository(pool)
	activator, _ := acquisition.NewActivationService(activation)
	jobRepo, _ := NewJobRepository(pool)
	submit, _ := acquisition.NewSubmissionService(creator, manifests, activator, jobRepo)
	store, _ := NewProductAdmissionRepository(pool)
	service, err := product.NewAdmissionService(store, submit, []storage.BindingID{binding})
	if err != nil {
		t.Fatal(err)
	}
	return admissionFixture{ctx: ctx, pool: pool, p: p, other: other, input: product.AdmissionRequest{OperationKey: "once", Source: "magnet:?xt=urn:btih:0123456789ABCDEF0123456789ABCDEF01234567&dn=private%20name&tr=https%3A%2F%2Ftracker.invalid%2Fa", BindingID: binding, TargetPath: "/downloads//item/"}, store: store, activation: activation, service: service, manifests: manifests}
}
func admissionCandidate(t *testing.T) product.Reservation {
	t.Helper()
	m, err := auth.NewUserID()
	if err != nil {
		t.Fatal(err)
	}
	j, err := auth.NewUserID()
	if err != nil {
		t.Fatal(err)
	}
	return product.Reservation{ManifestID: acquisition.ManifestID(m), JobID: jobs.JobID(j), MaxAttempts: product.AdmissionMaxAttempts}
}
func assertAdmissionCounts(t *testing.T, f admissionFixture, operations, manifests, jobsWant int) {
	t.Helper()
	for _, tc := range []struct {
		table string
		want  int
	}{{"product_operations", operations}, {"acquisition_manifests", manifests}, {"jobs", jobsWant}, {"acquisition_provider_tasks", 0}} {
		var got int
		if err := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM "+tc.table).Scan(&got); err != nil || got != tc.want {
			t.Fatalf("%s count=%d want=%d error=%v", tc.table, got, tc.want, err)
		}
	}
}
func TestPostgresProductAdmissionPersistsRawExecutionPolicyAndStableReplay(t *testing.T) {
	for _, ampTail := range []bool{false, true} {
		t.Run(fmt.Sprintf("amp_tail_%t", ampTail), func(t *testing.T) {
			f := newAdmissionFixture(t)
			if ampTail {
				f.input.Source = "magnet:?xt=urn:btih:0123456789ABCDEF0123456789ABCDEF01234567&amp;dn=private%20name&amp;tr=" + strings.Repeat("x", 10000)
			} else {
				f.input.Source += "&tr=" + strings.Repeat("x", 10000)
			}
			got, err := f.service.Admit(f.ctx, f.p, f.input)
			if err != nil || got.Status != acquisition.SubmissionCreated {
				t.Fatal(got, err)
			}
			var original, execution, policy string
			if err = f.pool.QueryRow(f.ctx, `SELECT original_source,execution_source,source_policy FROM product_operations WHERE user_id=$1`, string(f.p.UserID)).Scan(&original, &execution, &policy); err != nil {
				t.Fatal(err)
			}
			prepared, _ := acquisition.PrepareSource(f.input.Source, acquisition.MagnetBTIHNameV1)
			if ampTail && execution != "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567" {
				t.Fatal("AMP tail reached durable execution")
			}
			if original != f.input.Source || execution != prepared.Reference.Value || policy != string(acquisition.MagnetBTIHNameV1) {
				t.Fatal("durable preparation evidence differs")
			}
			m, err := f.manifests.GetManifest(f.ctx, got.ManifestID)
			if err != nil || m.SourceRef != execution || m.ExpectedName != nil || m.JobID == nil || *m.JobID != got.JobID {
				t.Fatal("Manifest execution/identity mismatch")
			}
			topology, _ := NewStorageRepository(f.pool)
			resolver, _ := acquisition.NewExecutionInputResolver(f.manifests, topology)
			input, err := resolver.Resolve(f.ctx, got.ManifestID)
			if err != nil || input.Download.Source.Value != execution {
				t.Fatal("provider source changed")
			}
			// Historical replay must not need an ACTIVE topology, nor charge twice.
			if _, err = f.pool.Exec(f.ctx, `UPDATE storage_bindings SET status='DISABLED' WHERE storage_binding_id=$1`, string(f.input.BindingID)); err != nil {
				t.Fatal(err)
			}
			if _, err = f.pool.Exec(f.ctx, `UPDATE product_operation_quotas SET operation_limit=1,concurrency_limit=1 WHERE user_id=$1`, string(f.p.UserID)); err != nil {
				t.Fatal(err)
			}
			replay, err := f.service.Admit(f.ctx, f.p, f.input)
			if err != nil || replay.Status != acquisition.SubmissionReplay || replay.ManifestID != got.ManifestID || replay.JobID != got.JobID {
				t.Fatal("replay changed", err)
			}
			usage, err := f.service.Usage(f.ctx, f.p)
			if err != nil || usage.Used != 1 || usage.InFlight != 1 || usage.Remaining != 0 {
				t.Fatal(usage, err)
			}
			encoded, _ := json.Marshal(got)
			if strings.Contains(string(encoded), "private") || strings.Contains(string(encoded), "source") || strings.Contains(string(encoded), "policy") {
				t.Fatal("unsafe DTO")
			}
			assertAdmissionCounts(t, f, 1, 1, 1)
		})
	}
}
func TestPostgresProductAdmissionPinnedPolicyAndConflictingRawIntent(t *testing.T) {
	f := newAdmissionFixture(t)
	got, err := f.service.Admit(f.ctx, f.p, f.input)
	if err != nil {
		t.Fatal(err)
	}
	changed := f.input
	changed.Source += "&tr=another"
	if _, err = f.service.Admit(f.ctx, f.p, changed); !errors.Is(err, product.ErrOperationConflict) {
		t.Fatal("equivalent execution hid raw conflict", err)
	}
	// Model an earlier accepted exact policy. Replay uses its stored source,
	// NOT today's magnet preparation. This SQL is isolated test setup only.
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(f.ctx, `UPDATE product_operations SET execution_source=original_source,source_policy='source-exact-v1' WHERE user_id=$1`, string(f.p.UserID)); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(f.ctx, `UPDATE acquisition_manifests SET source_ref=$1 WHERE manifest_id=$2`, f.input.Source, string(got.ManifestID)); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	replay, err := f.service.Admit(f.ctx, f.p, f.input)
	if err != nil || replay.ManifestID != got.ManifestID {
		t.Fatal("stored policy re-prepared", err)
	}
	reserved, err := f.store.Reserve(f.ctx, f.p, f.input, admissionCandidate(t))
	if err != nil || reserved.Source != f.input.Source || reserved.Policy != acquisition.SourceExactV1 {
		t.Fatal("policy lost", err)
	}
	other, err := f.service.Admit(f.ctx, f.other, f.input)
	if err != nil || other.ManifestID == got.ManifestID || other.JobID == got.JobID {
		t.Fatal("cross-user operation adopted", err)
	}
	usage, err := f.service.Usage(f.ctx, f.other)
	if err != nil || usage.Used != 1 {
		t.Fatal("cross-user quota leak")
	}
	// Detect corrupted Manifest/ledger identity, rather than activate it.
	if _, err = f.pool.Exec(f.ctx, `UPDATE acquisition_manifests SET source_ref='https://example.invalid/wrong' WHERE manifest_id=$1`, string(got.ManifestID)); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Admit(f.ctx, f.p, f.input); !errors.Is(err, product.ErrAdmissionUncertain) {
		t.Fatal("corrupt execution accepted", err)
	}
	assertAdmissionCounts(t, f, 2, 2, 2)
}
func TestPostgresProductAdmissionConcurrentReplayAndConflict(t *testing.T) {
	f := newAdmissionFixture(t)
	const n = 16
	type result struct {
		value product.AdmissionDTO
		err   error
	}
	results := make(chan result, n)
	start := make(chan struct{})
	var group sync.WaitGroup
	for i := 0; i < n; i++ {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			<-start
			r := f.input
			if i%2 == 1 {
				r.Source += "&tr=conflicting"
			}
			v, e := f.service.Admit(f.ctx, f.p, r)
			results <- result{v, e}
		}(i)
	}
	close(start)
	group.Wait()
	close(results)
	var first product.AdmissionDTO
	success, conflicts := 0, 0
	for result := range results {
		if errors.Is(result.err, product.ErrOperationConflict) {
			conflicts++
			continue
		}
		if result.err != nil {
			t.Fatal(result.err)
		}
		if success == 0 {
			first = result.value
		}
		success++
		if result.value.ManifestID != first.ManifestID || result.value.JobID != first.JobID {
			t.Fatal("parallel duplicate pair")
		}
	}
	if success != n/2 || conflicts != n/2 {
		t.Fatal(success, conflicts)
	}
	assertAdmissionCounts(t, f, 1, 1, 1)
}
func TestPostgresProductAdmissionConcurrentLimitsAndIdentity(t *testing.T) {
	f := newAdmissionFixture(t)
	type result struct {
		value product.AdmissionDTO
		err   error
	}
	results := make(chan result, 12)
	var group sync.WaitGroup
	for i := 0; i < 12; i++ {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			r := f.input
			r.OperationKey = fmt.Sprintf("op-%d", i)
			v, e := f.service.Admit(f.ctx, f.p, r)
			results <- result{v, e}
		}(i)
	}
	group.Wait()
	close(results)
	accepted := []product.AdmissionDTO{}
	for result := range results {
		if result.err == nil {
			accepted = append(accepted, result.value)
		} else if !errors.Is(result.err, product.ErrConcurrencyLimit) {
			t.Fatal(result.err)
		}
	}
	if len(accepted) != 2 {
		t.Fatal("concurrency overrun", len(accepted))
	}
	// Paired terminal fixture releases concurrency, never lifetime usage.
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(f.ctx, `UPDATE acquisition_manifests SET state='FAILED',updated_at=clock_timestamp() WHERE user_id=$1`, string(f.p.UserID)); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(f.ctx, `UPDATE jobs SET state='FAILED',finished_at=clock_timestamp(),updated_at=clock_timestamp()`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(f.ctx, `UPDATE product_operation_quotas SET operation_limit=2 WHERE user_id=$1`, string(f.p.UserID)); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	r := f.input
	r.OperationKey = "new"
	if _, err = f.service.Admit(f.ctx, f.p, r); !errors.Is(err, product.ErrOperationLimit) {
		t.Fatal("lifetime overrun", err)
	}
	usage, err := f.service.Usage(f.ctx, f.p)
	if err != nil || usage.Used != 2 || usage.InFlight != 0 || usage.Remaining != 0 {
		t.Fatal(usage, err)
	}
	forged := f.p
	forged.Role = auth.RoleAdmin
	if _, err = f.service.Admit(f.ctx, forged, r); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatal("stale role admitted")
	}
	if _, err = f.pool.Exec(f.ctx, `UPDATE product_users SET enabled=false WHERE user_id=$1`, string(f.p.UserID)); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Usage(f.ctx, f.p); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatal("disabled user usage")
	}
	if _, err = f.service.Admit(f.ctx, f.p, r); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatal("disabled user admitted")
	}
	assertAdmissionCounts(t, f, 2, 2, 2)
}
func TestPostgresProductAdmissionAtomicRollbackCancellationAndCrash(t *testing.T) {
	for _, mode := range []string{"rollback", "cancel", "backend-exit"} {
		t.Run(mode, func(t *testing.T) {
			f := newAdmissionFixture(t)
			ctx, cancel := context.WithCancel(f.ctx)
			defer cancel()
			f.store.afterManifest = func(ctx context.Context, tx pgx.Tx) error {
				switch mode {
				case "rollback":
					return errors.New("private-source sentinel")
				case "cancel":
					cancel()
					return context.Canceled
				case "backend-exit":
					var pid int32
					if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
						return err
					}
					var terminated bool
					return f.pool.QueryRow(ctx, `SELECT pg_terminate_backend($1)`, pid).Scan(&terminated)
				}
				return nil
			}
			_, err := f.service.Admit(ctx, f.p, f.input)
			if err == nil || strings.Contains(err.Error(), "sentinel") {
				t.Fatal("rollback unproven/leaked")
			}
			assertAdmissionCounts(t, f, 0, 0, 0)
			var quotas int
			if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM product_operation_quotas`).Scan(&quotas); err != nil || quotas != 0 {
				t.Fatal("quota reservation survived failed transaction")
			}
		})
	}
}
func TestPostgresProductAdmissionUnknownCommitAndPendingActivation(t *testing.T) {
	f := newAdmissionFixture(t)
	f.store.commit = func(context.Context, pgx.Tx) error { return errors.New("commit outcome unknown") }
	if _, err := f.service.Admit(f.ctx, f.p, f.input); !errors.Is(err, product.ErrAdmissionUncertain) {
		t.Fatal("missing evidence assumed rollback", err)
	}
	assertAdmissionCounts(t, f, 0, 0, 0)
	f.store.commit = func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		return errors.New("lost commit acknowledgement")
	}
	f.activation.afterJobInsert = func(context.Context, pgx.Tx) error { return errors.New("activation rollback") }
	pending, err := f.service.Admit(f.ctx, f.p, f.input)
	if err != nil || pending.Status != acquisition.SubmissionPendingActivation {
		t.Fatal("committed pending lost", err)
	}
	assertAdmissionCounts(t, f, 1, 1, 0)
	m, err := f.manifests.GetManifest(f.ctx, pending.ManifestID)
	if err != nil || m.State != acquisition.StatePending || m.JobID != nil {
		t.Fatal("partial activation")
	}
	f.store.commit = nil
	f.activation.afterJobInsert = nil
	active, err := f.service.Admit(f.ctx, f.p, f.input)
	if err != nil || active.ManifestID != pending.ManifestID || active.JobID != pending.JobID || active.Status != acquisition.SubmissionReplay {
		t.Fatal("pending retry changed identity", err)
	}
	assertAdmissionCounts(t, f, 1, 1, 1)
}
func TestPostgresProductAdmissionRejectsInvalidPolicyInputAndTopology(t *testing.T) {
	f := newAdmissionFixture(t)
	for _, source := range []string{"magnet:?xt=urn:btih:bad", f.input.Source + "&so=1", "file:///etc/passwd", strings.Repeat("x", 65537)} {
		r := f.input
		r.Source = source
		if _, err := f.service.Admit(f.ctx, f.p, r); !errors.Is(err, product.ErrInvalid) {
			t.Fatal("invalid source admitted", err)
		}
		assertAdmissionCounts(t, f, 0, 0, 0)
	}
	if _, err := f.pool.Exec(f.ctx, `UPDATE storage_bindings SET status='DISABLED' WHERE storage_binding_id=$1`, string(f.input.BindingID)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Admit(f.ctx, f.p, f.input); !errors.Is(err, product.ErrNotFound) {
		t.Fatal("disabled target admitted", err)
	}
	assertAdmissionCounts(t, f, 0, 0, 0)
}

func TestPostgresProductAdmissionCommitEvidenceSurvivesRequestCancellation(t *testing.T) {
	f := newAdmissionFixture(t)
	ctx, cancel := context.WithCancel(f.ctx)
	defer cancel()
	f.store.commit = func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		cancel()
		return context.Canceled
	}
	candidate := admissionCandidate(t)
	result, err := f.store.Reserve(ctx, f.p, f.input, candidate)
	if err != nil || result.ManifestID != candidate.ManifestID || result.JobID != candidate.JobID || result.Source == "" {
		t.Fatal("canceled request hid committed evidence", err)
	}
	assertAdmissionCounts(t, f, 1, 1, 0)
	f.store.commit = nil
	replay, err := f.service.Admit(f.ctx, f.p, f.input)
	if err != nil || replay.ManifestID != candidate.ManifestID || replay.JobID != candidate.JobID {
		t.Fatal("canceled commit created replacement", err)
	}
	assertAdmissionCounts(t, f, 1, 1, 1)
}

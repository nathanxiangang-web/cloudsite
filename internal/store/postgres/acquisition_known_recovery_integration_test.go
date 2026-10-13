package postgres

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/jobs"
	"github.com/nathanxiangang-web/panta/internal/providers/contracts"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

func seedKnownRecovery(t *testing.T) (context.Context, *pgxpool.Pool, *KnownRecoveryRepository, acquisition.KnownRecoveryRequest) {
	t.Helper()
	ctx, pool, _, manifests, jobStore, binding := newProviderOutcomeIntegrationFixture(t)
	fixture := seedProviderOutcomeJob(t, ctx, pool, binding, "11", "owner", 22, time.Now().Add(time.Hour))
	if _, err := pool.Exec(ctx, `UPDATE jobs SET state='RECOVERY_REQUIRED',lease_owner=NULL,lease_expires_at=NULL,last_error='prior error',attempt_count=1 WHERE job_id=$1`, string(fixture.jobID)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE acquisition_manifests SET state='RECOVERY_REQUIRED',result_name='existing-name' WHERE manifest_id=$1`, string(fixture.manifestID)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO acquisition_provider_tasks (manifest_id,job_id,provider_id,provider_task_ref,state,created_at,updated_at) VALUES($1,$2,'test','opaque-known-reference','REFERENCE_KNOWN',clock_timestamp(),clock_timestamp())`, string(fixture.manifestID), string(fixture.jobID)); err != nil {
		t.Fatal(err)
	}
	manifest, err := manifests.GetManifest(ctx, fixture.manifestID)
	if err != nil {
		t.Fatal(err)
	}
	job, err := jobStore.Get(ctx, fixture.jobID)
	if err != nil {
		t.Fatal(err)
	}
	repository, _ := NewKnownRecoveryRepository(pool)
	request := acquisition.KnownRecoveryRequest{Scope: acquisition.KnownPollScope{ManifestID: fixture.manifestID, JobID: fixture.jobID, BindingID: binding, ConnectionID: storage.ConnectionID(binding), ProviderID: "test", RootID: "provider-outcome-root", ReferenceSHA256: acquisition.EvidenceSHA256("opaque-known-reference"), SourceSHA256: acquisition.EvidenceSHA256(manifest.SourceRef)}, ExpectedClaim: 22, ManifestUpdatedAt: manifest.UpdatedAt, JobUpdatedAt: job.UpdatedAt, RetryDelay: time.Second}
	return ctx, pool, repository, request
}

func TestPostgresKnownRecoveryAtomicPreservesBudgetIdentityAndReplay(t *testing.T) {
	ctx, pool, repository, request := seedKnownRecovery(t)
	jobStore, _ := NewJobRepository(pool)
	before, _ := jobStore.Get(ctx, request.Scope.JobID)
	result, err := repository.ResumeKnownAcquisition(ctx, request)
	if err != nil || !result.Changed || result.Manifest.State != acquisition.StateActive || result.Job.State != jobs.StateRetryWait {
		t.Fatalf("resume=%#v %v", result, err)
	}
	if result.Job.ClaimAttempts != 22 || result.Job.FailureCount != before.FailureCount || result.Job.MaxAttempts != before.MaxAttempts || !bytes.Equal(result.Job.Payload, before.Payload) || *result.Job.IdempotencyKey != *before.IdempotencyKey || *result.Job.LastError != *before.LastError || *result.Manifest.ResultName != "existing-name" || result.Job.LeaseOwner != nil || result.Job.LeaseExpiresAt != nil {
		t.Fatal("resume rewrote identity, history, or budget")
	}
	var retryFuture bool
	if pool.QueryRow(ctx, `SELECT next_attempt_at>clock_timestamp() FROM jobs WHERE job_id=$1`, string(request.Scope.JobID)).Scan(&retryFuture) != nil || !retryFuture {
		t.Fatal("retry not scheduled by database time")
	}
	replay, err := repository.ResumeKnownAcquisition(ctx, request)
	if err != nil || replay.Changed || !replay.Job.UpdatedAt.Equal(result.Job.UpdatedAt) || !replay.Job.NextAttemptAt.Equal(*result.Job.NextAttemptAt) {
		t.Fatal("replay mutated or scheduled again")
	}
}

func TestPostgresKnownRecoveryRejectsUncertainAndMismatchedScopes(t *testing.T) {
	for _, mutation := range []string{"unknown", "missing", "reference", "claim", "timestamp", "root", "connection", "binding", "provider", "source", "canceled", "lease", "budget", "payload"} {
		t.Run(mutation, func(t *testing.T) {
			ctx, pool, repository, request := seedKnownRecovery(t)
			var sql string
			switch mutation {
			case "unknown":
				sql = `UPDATE acquisition_provider_tasks SET state='START_RESERVED',provider_task_ref=NULL`
			case "missing":
				sql = `DELETE FROM acquisition_provider_tasks`
			case "reference":
				request.Scope.ReferenceSHA256 = acquisition.EvidenceSHA256("wrong")
			case "claim":
				request.ExpectedClaim++
			case "timestamp":
				request.JobUpdatedAt = request.JobUpdatedAt.Add(time.Second)
			case "root":
				request.Scope.RootID = "wrong"
			case "connection":
				request.Scope.ConnectionID = "00000000-0000-4000-8000-000000000099"
			case "binding":
				request.Scope.BindingID = "00000000-0000-4000-8000-000000000099"
			case "provider":
				request.Scope.ProviderID = "wrong"
			case "source":
				request.Scope.SourceSHA256 = acquisition.EvidenceSHA256("wrong")
			case "canceled":
				sql = `UPDATE jobs SET state='CANCELED',finished_at=clock_timestamp()`
			case "lease":
				sql = `UPDATE jobs SET lease_owner='foreign',lease_expires_at=clock_timestamp()+interval '1 minute',state='RUNNING'`
			case "budget":
				sql = `UPDATE jobs SET attempt_count=max_attempts`
			case "payload":
				sql = `UPDATE jobs SET payload='{}'::jsonb`
			}
			if sql != "" {
				if _, err := pool.Exec(ctx, sql); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := repository.ResumeKnownAcquisition(ctx, request); !errors.Is(err, acquisition.ErrKnownRecoveryConflict) {
				t.Fatalf("error=%v", err)
			}
			var state string
			_ = pool.QueryRow(ctx, `SELECT state FROM acquisition_manifests WHERE manifest_id=$1`, string(request.Scope.ManifestID)).Scan(&state)
			if state != "RECOVERY_REQUIRED" {
				t.Fatal("rejected request changed manifest")
			}
		})
	}
}

func TestPostgresKnownRecoveryConcurrentReplayAndClaimFence(t *testing.T) {
	ctx, pool, repository, request := seedKnownRecovery(t)
	var wait sync.WaitGroup
	results := make(chan acquisition.ProviderOutcomeResult, 2)
	failures := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			r, e := repository.ResumeKnownAcquisition(ctx, request)
			results <- r
			failures <- e
		}()
	}
	wait.Wait()
	changed := 0
	for i := 0; i < 2; i++ {
		if e := <-failures; e != nil {
			t.Fatal(e)
		}
		if (<-results).Changed {
			changed++
		}
	}
	if changed != 1 {
		t.Fatalf("changed=%d", changed)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET next_attempt_at=clock_timestamp()-interval '1 second' WHERE job_id=$1`, string(request.Scope.JobID)); err != nil {
		t.Fatal(err)
	}
	jobStore, _ := NewJobRepository(pool)
	claimed, err := jobStore.ClaimNextByType(ctx, acquisition.JobTypeAcquisition, jobs.ClaimRequest{OnlyJobID: request.Scope.JobID, Owner: "new-owner", Now: time.Now(), LeaseDuration: time.Minute})
	if err != nil || claimed.ClaimAttempts != 23 {
		t.Fatalf("claim=%#v %v", claimed, err)
	}
	if _, err = repository.ResumeKnownAcquisition(ctx, request); !errors.Is(err, acquisition.ErrKnownRecoveryConflict) {
		t.Fatal("old resume can overwrite new claim")
	}
}

func TestPostgresKnownRecoveryRollbackAfterConnectionTermination(t *testing.T) {
	ctx, pool, repository, request := seedKnownRecovery(t)
	repository.afterManifestUpdate = func(ctx context.Context, tx pgx.Tx) error {
		var terminated bool
		if err := pool.QueryRow(ctx, `SELECT pg_terminate_backend($1)`, tx.Conn().PgConn().PID()).Scan(&terminated); err != nil {
			return err
		}
		return errors.New("synthetic crash")
	}
	if _, err := repository.ResumeKnownAcquisition(ctx, request); err == nil {
		t.Fatal("crash committed")
	}
	var manifestState, jobState string
	if err := pool.QueryRow(ctx, `SELECT m.state,j.state FROM acquisition_manifests m JOIN jobs j ON j.job_id=m.job_id WHERE m.manifest_id=$1`, string(request.Scope.ManifestID)).Scan(&manifestState, &jobState); err != nil {
		t.Fatal(err)
	}
	if manifestState != "RECOVERY_REQUIRED" || jobState != "RECOVERY_REQUIRED" {
		t.Fatal("half-committed recovery")
	}
}

func seedKnownPoll(t *testing.T, max int) (context.Context, *pgxpool.Pool, *ProviderOutcomeRepository, providerOutcomeFixture) {
	t.Helper()
	ctx, pool, repository, _, _, binding := newProviderOutcomeIntegrationFixture(t)
	fixture := seedProviderOutcomeJobWithBudget(t, ctx, pool, binding, "22", "poll-owner", 23, max, time.Now().Add(time.Minute))
	if _, err := pool.Exec(ctx, `INSERT INTO acquisition_provider_tasks(manifest_id,job_id,provider_id,provider_task_ref,state,created_at,updated_at) VALUES($1,$2,'test','opaque-ref','REFERENCE_KNOWN',clock_timestamp(),clock_timestamp())`, string(fixture.manifestID), string(fixture.jobID)); err != nil {
		t.Fatal(err)
	}
	return ctx, pool, repository, fixture
}
func pollPlan(t *testing.T, fixture providerOutcomeFixture, outcome acquisition.ProviderOutcome) acquisition.ProviderOutcomePlan {
	t.Helper()
	message := string(contracts.PollTimeout)
	_, plan, err := acquisition.BuildProviderOutcomePlan(acquisition.ProviderOutcomeRequest{ManifestID: fixture.manifestID, JobID: fixture.jobID, Owner: fixture.owner, ExpectedClaim: fixture.attempt, Outcome: outcome, Now: time.Now(), ErrorMessage: &message})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestPostgresKnownPollRetryBudgetReplayAndPermanentStop(t *testing.T) {
	for _, test := range []struct {
		max     int
		outcome acquisition.ProviderOutcome
		state   jobs.State
	}{{3, acquisition.ProviderOutcomePollRetry, jobs.StateRetryWait}, {1, acquisition.ProviderOutcomePollRetry, jobs.StateRecoveryRequired}, {3, acquisition.ProviderOutcomePollStop, jobs.StateRecoveryRequired}} {
		t.Run(fmt.Sprintf("%d-%s", test.max, test.outcome), func(t *testing.T) {
			ctx, _, repository, fixture := seedKnownPoll(t, test.max)
			plan := pollPlan(t, fixture, test.outcome)
			result, err := repository.CommitProviderOutcome(ctx, plan)
			if err != nil || result.Job.State != test.state || result.Job.FailureCount != 1 || result.Job.ClaimAttempts != 23 || result.Job.LeaseOwner != nil {
				t.Fatalf("handoff=%#v %v", result, err)
			}
			replay, err := repository.CommitProviderOutcome(ctx, plan)
			if err != nil || replay.Changed || replay.Job.FailureCount != 1 {
				t.Fatalf("replay=%#v %v", replay, err)
			}
		})
	}
}

func TestPostgresKnownPollExpiryOwnerGenerationAndUncertainReference(t *testing.T) {
	for _, mutation := range []string{"owner", "generation", "expired", "uncertain", "budget-tamper", "rollback"} {
		t.Run(mutation, func(t *testing.T) {
			ctx, pool, repository, fixture := seedKnownPoll(t, 3)
			plan := pollPlan(t, fixture, acquisition.ProviderOutcomePollRetry)
			switch mutation {
			case "owner":
				plan.Owner = "foreign"
			case "generation":
				plan.ExpectedClaim++
			case "expired":
				if _, err := pool.Exec(ctx, `UPDATE jobs SET lease_expires_at=clock_timestamp()-interval '1 second' WHERE job_id=$1`, string(fixture.jobID)); err != nil {
					t.Fatal(err)
				}
			case "uncertain":
				if _, err := pool.Exec(ctx, `UPDATE acquisition_provider_tasks SET state='START_RESERVED',provider_task_ref=NULL`); err != nil {
					t.Fatal(err)
				}
			case "budget-tamper":
				plan.ErrorMessage = new(string)
				*plan.ErrorMessage = "PRIVATE ARBITRARY ERROR"
			case "rollback":
				repository.afterManifestUpdate = func(context.Context, pgx.Tx) error { return errors.New("synthetic crash") }
			}
			if _, err := repository.CommitProviderOutcome(ctx, plan); err == nil {
				t.Fatal("invalid handoff committed")
			}
			var state string
			var failures int
			if err := pool.QueryRow(ctx, `SELECT state,attempt_count FROM jobs WHERE job_id=$1`, string(fixture.jobID)).Scan(&state, &failures); err != nil {
				t.Fatal(err)
			}
			if state != "RUNNING" || failures != 0 {
				t.Fatal("failed transaction changed state/budget")
			}
		})
	}
}

func TestPostgresKnownPollLeaseExpiresBetweenMutations(t *testing.T) {
	ctx, pool, repository, fixture := seedKnownPoll(t, 3)
	if _, err := pool.Exec(ctx, `UPDATE jobs SET lease_expires_at=clock_timestamp()+interval '0.05 second' WHERE job_id=$1`, string(fixture.jobID)); err != nil {
		t.Fatal(err)
	}
	repository.afterManifestUpdate = func(ctx context.Context, tx pgx.Tx) error { _, err := tx.Exec(ctx, `SELECT pg_sleep(0.1)`); return err }
	if _, err := repository.CommitProviderOutcome(ctx, pollPlan(t, fixture, acquisition.ProviderOutcomePollStop)); err == nil {
		t.Fatal("expired worker committed after lock wait")
	}
	var state string
	_ = pool.QueryRow(ctx, `SELECT state FROM acquisition_manifests WHERE manifest_id=$1`, string(fixture.manifestID)).Scan(&state)
	if state != "ACTIVE" {
		t.Fatal("manifest mutation failed to roll back")
	}
}

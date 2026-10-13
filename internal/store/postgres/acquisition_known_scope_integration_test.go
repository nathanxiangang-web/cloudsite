package postgres

import (
	"errors"
	"testing"
	"time"

	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/jobs"
)

func TestPostgresKnownScopedClaimUsesDatabaseTimeAndExcludesOtherJobs(t *testing.T) {
	ctx, pool, repository, request := seedKnownRecovery(t)
	if _, err := repository.ResumeKnownAcquisition(ctx, request); err != nil {
		t.Fatal(err)
	}
	jobStore, _ := NewJobRepository(pool)
	// Far-future request.Now cannot override the new scoped DB-time retry fence.
	_, err := jobStore.ClaimNextByType(ctx, acquisition.JobTypeAcquisition, jobs.ClaimRequest{OnlyJobID: request.Scope.JobID, Owner: "owner", Now: time.Now().Add(time.Hour), LeaseDuration: time.Minute})
	if !errors.Is(err, jobs.ErrNoClaimableJob) {
		t.Fatal("local clock bypassed database retry time")
	}
	other := seedProviderOutcomeJob(t, ctx, pool, request.Scope.BindingID, "33", "other", 1, time.Now().Add(time.Minute))
	if _, err := pool.Exec(ctx, `UPDATE jobs SET state='QUEUED',lease_owner=NULL,lease_expires_at=NULL WHERE job_id=$1`, string(other.jobID)); err != nil {
		t.Fatal(err)
	}
	_, err = jobStore.ClaimNextByType(ctx, acquisition.JobTypeAcquisition, jobs.ClaimRequest{OnlyJobID: request.Scope.JobID, Owner: "owner", Now: time.Now(), LeaseDuration: time.Minute})
	if !errors.Is(err, jobs.ErrNoClaimableJob) {
		t.Fatal("scoped worker took another job")
	}
	otherJob, _ := jobStore.Get(ctx, other.jobID)
	if otherJob.State != jobs.StateQueued || otherJob.ClaimAttempts != 1 {
		t.Fatal("other job changed")
	}
}

func TestPostgresKnownRecoveryLeavesEightUnknownReservationsFrozen(t *testing.T) {
	ctx, pool, repository, request := seedKnownRecovery(t)
	for _, suffix := range []string{"40", "41", "42", "43", "44", "45", "46", "47"} {
		fixture := seedProviderOutcomeJob(t, ctx, pool, request.Scope.BindingID, suffix, "old", 1, time.Now().Add(time.Minute))
		if _, err := pool.Exec(ctx, `UPDATE jobs SET state='RECOVERY_REQUIRED',lease_owner=NULL,lease_expires_at=NULL WHERE job_id=$1`, string(fixture.jobID)); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `UPDATE acquisition_manifests SET state='RECOVERY_REQUIRED' WHERE manifest_id=$1`, string(fixture.manifestID)); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO acquisition_provider_tasks(manifest_id,job_id,provider_id,state,created_at,updated_at) VALUES($1,$2,'test','START_RESERVED',clock_timestamp(),clock_timestamp())`, string(fixture.manifestID), string(fixture.jobID)); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := func() string {
		var digest string
		err := pool.QueryRow(ctx, `SELECT md5(string_agg(row_to_json(evidence)::text,',' ORDER BY manifest_id)) FROM (SELECT m.*,row_to_json(j) job,row_to_json(p) provider FROM acquisition_manifests m JOIN jobs j ON j.job_id=m.job_id JOIN acquisition_provider_tasks p ON p.manifest_id=m.manifest_id WHERE p.state='START_RESERVED') evidence`).Scan(&digest)
		if err != nil {
			t.Fatal(err)
		}
		return digest
	}
	before := snapshot()
	if _, err := repository.ResumeKnownAcquisition(ctx, request); err != nil {
		t.Fatal(err)
	}
	if before != snapshot() {
		t.Fatal("known recovery changed one of eight frozen reservations")
	}
}

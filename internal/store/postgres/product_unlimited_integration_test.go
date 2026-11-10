package postgres

import (
	"fmt"
	"testing"

	"github.com/nathanxiangang-web/panta/internal/acquisition"
)

func TestPostgresProductUnlimitedPastLegacyCapsAndRecoveryWithoutProviderCalls(t *testing.T) {
	f := newAdmissionFixture(t)
	before, err := f.service.Usage(f.ctx, f.p)
	if err != nil || !before.Unlimited || before.OperationLimit != 0 || before.ConcurrencyLimit != 0 || before.Used != 0 {
		t.Fatal(before, err)
	}
	var firstID acquisition.ManifestID
	for i := 0; i < 105; i++ {
		input := f.input
		input.OperationKey = fmt.Sprintf("unlimited-%d", i)
		got, err := f.service.Admit(f.ctx, f.p, input)
		if err != nil {
			t.Fatalf("operation %d: %v", i, err)
		}
		if i == 0 {
			firstID = got.ManifestID
			// Existing accounts may have exhausted even a zero lifetime limit.
			if _, err = f.pool.Exec(f.ctx, `UPDATE product_operation_quotas SET operation_limit=0,concurrency_limit=1 WHERE user_id=$1`, string(f.p.UserID)); err != nil {
				t.Fatal(err)
			}
		}
		if i == 1 {
			// Reproduce the incident with two paired paused tasks, without a
			// provider reservation, start or fabricated completion.
			if _, err = f.pool.Exec(f.ctx, `UPDATE acquisition_manifests SET state='RECOVERY_REQUIRED' WHERE user_id=$1`, string(f.p.UserID)); err != nil {
				t.Fatal(err)
			}
			if _, err = f.pool.Exec(f.ctx, `UPDATE jobs SET state='RECOVERY_REQUIRED'`); err != nil {
				t.Fatal(err)
			}
		}
	}
	input := f.input
	input.OperationKey = "unlimited-0"
	replay, err := f.service.Admit(f.ctx, f.p, input)
	if err != nil || replay.ManifestID != firstID || replay.Status != acquisition.SubmissionReplay {
		t.Fatal("replay changed historical task", replay, err)
	}
	after, err := f.service.Usage(f.ctx, f.p)
	if err != nil || !after.Unlimited || after.Used != 105 || after.InFlight != 105 || after.Remaining != 0 || after.OperationLimit != 0 || after.ConcurrencyLimit != 0 {
		t.Fatal(after, err)
	}
	assertAdmissionCounts(t, f, 105, 105, 105)
}

package postgres

import (
	"errors"
	"github.com/nathanxiangang-web/panta/internal/sessionhealth"
	"sync"
	"testing"
	"time"

	"github.com/nathanxiangang-web/panta/internal/product"
)

func TestPostgresProductSessionGateRejectsNewWithoutPersistentChanges(t *testing.T) {
	f := newAdmissionFixture(t)
	for _, gate := range []error{product.ErrProviderLoginRequired, product.ErrProviderSessionUnknown} {
		_, err := f.store.ReserveChecked(f.ctx, f.p, f.input, admissionCandidate(t), sessionRejection(gate))
		if !errors.Is(err, gate) {
			t.Fatal(err)
		}
		assertAdmissionCounts(t, f, 0, 0, 0)
		var quotas int
		if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM product_operation_quotas`).Scan(&quotas); err != nil || quotas != 0 {
			t.Fatal("quota changed", quotas, err)
		}
	}
	// The gate cannot hide an exact accepted operation or its conflict, even
	// after credentials expire. The original reservation and policy survive.
	original, err := f.service.Admit(f.ctx, f.p, f.input)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := f.store.ReserveChecked(f.ctx, f.p, f.input, admissionCandidate(t), sessionRejection(product.ErrProviderLoginRequired))
	if err != nil || replay.ManifestID != original.ManifestID || replay.JobID != original.JobID || replay.Created {
		t.Fatal(replay, err)
	}
	changed := f.input
	changed.Source = "https://example.invalid/changed"
	if _, err = f.store.ReserveChecked(f.ctx, f.p, changed, admissionCandidate(t), sessionRejection(product.ErrProviderLoginRequired)); !errors.Is(err, product.ErrOperationConflict) {
		t.Fatal(err)
	}
	assertAdmissionCounts(t, f, 1, 1, 1)
}
func TestPostgresProductSessionGateConcurrentRejectionThenRecovery(t *testing.T) {
	f := newAdmissionFixture(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		candidate := admissionCandidate(t)
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.store.ReserveChecked(f.ctx, f.p, f.input, candidate, sessionRejection(product.ErrProviderLoginRequired))
			if !errors.Is(err, product.ErrProviderLoginRequired) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	assertAdmissionCounts(t, f, 0, 0, 0)
	if _, err := f.service.Admit(f.ctx, f.p, f.input); err != nil {
		t.Fatal(err)
	}
	assertAdmissionCounts(t, f, 1, 1, 1)
}

func sessionRejection(err error) product.AdmissionCheck {
	status := sessionhealth.SessionUnknown
	if errors.Is(err, product.ErrProviderLoginRequired) {
		status = sessionhealth.SessionLoginRequired
	}
	return product.AdmissionCheck{View: sessionhealth.View{Status: status, ValidUntil: time.Now().Add(time.Minute)}}
}
func TestPostgresProductSessionGateExpiredPermissionFailsClosed(t *testing.T) {
	f := newAdmissionFixture(t)
	check := product.AdmissionCheck{View: sessionhealth.View{Status: sessionhealth.SessionAuthenticated, ValidUntil: time.Now().Add(-time.Second)}}
	if _, err := f.store.ReserveChecked(f.ctx, f.p, f.input, admissionCandidate(t), check); !errors.Is(err, product.ErrProviderSessionUnknown) {
		t.Fatal(err)
	}
	assertAdmissionCounts(t, f, 0, 0, 0)
}

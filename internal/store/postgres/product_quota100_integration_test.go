package postgres

import "testing"

func TestPostgresProductDefaultQuota100PersistsWithoutProviderCalls(t *testing.T) {
	f := newAdmissionFixture(t)
	before, err := f.store.Usage(f.ctx, f.p)
	if err != nil || before.OperationLimit != 100 || before.ConcurrencyLimit != 2 || before.Used != 0 {
		t.Fatal(before, err)
	}
	if _, err = f.service.Admit(f.ctx, f.p, f.input); err != nil {
		t.Fatal(err)
	}
	var limit, concurrency int
	if err = f.pool.QueryRow(f.ctx, `SELECT operation_limit,concurrency_limit FROM product_operation_quotas WHERE user_id=$1`, string(f.p.UserID)).Scan(&limit, &concurrency); err != nil || limit != 100 || concurrency != 2 {
		t.Fatal(limit, concurrency, err)
	}
	after, err := f.store.Usage(f.ctx, f.p)
	if err != nil || after.Used != 1 || after.Remaining != 99 || after.OperationLimit != 100 {
		t.Fatal(after, err)
	}
	assertAdmissionCounts(t, f, 1, 1, 1)
}

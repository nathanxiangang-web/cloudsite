package adminread

import (
	"context"
	"time"

	"github.com/nathanxiangang-web/panta/internal/auth"
)

const OverviewScope = "PRODUCT_ACQUISITIONS_IN_CONFIGURED_BINDINGS"
const OverviewSource = "CLOUDSITE_POSTGRES_MANIFESTS_AND_JOBS"
const maxOverviewCount int64 = 1<<53 - 1 // Exact JSON integer in browser clients.

type ManifestCounts struct {
	Total              int64 `json:"total"`
	Pending            int64 `json:"pending"`
	Active             int64 `json:"active"`
	AwaitingVisibility int64 `json:"awaiting_visibility"`
	AwaitingCanonical  int64 `json:"awaiting_canonical"`
	Ready              int64 `json:"ready"`
	Failed             int64 `json:"failed"`
	Canceled           int64 `json:"canceled"`
	CanceledWithoutJob int64 `json:"canceled_without_job,omitempty"`
	RecoveryRequired   int64 `json:"recovery_required"`
	Incomplete         int64 `json:"incomplete"`
}
type JobCounts struct {
	Total            int64 `json:"total"`
	Queued           int64 `json:"queued"`
	Running          int64 `json:"running"`
	RetryWait        int64 `json:"retry_wait"`
	Succeeded        int64 `json:"succeeded"`
	Failed           int64 `json:"failed"`
	Canceled         int64 `json:"canceled"`
	RecoveryRequired int64 `json:"recovery_required"`
}
type Overview struct {
	Source      string         `json:"source"`
	Scope       string         `json:"scope"`
	OwnerUserID string         `json:"owner_user_id,omitempty"`
	ReadAt      time.Time      `json:"read_at"`
	Manifests   ManifestCounts `json:"manifests"`
	Jobs        JobCounts      `json:"jobs"`
}

// InvalidRecords is adapter evidence, never a public metric or a silent exclusion.
type OverviewRecord struct {
	Overview
	InvalidRecords int64 `json:"-"`
}
type OverviewReader interface {
	ReadOverview(context.Context, []string, string) (OverviewRecord, error)
}
type OverviewReads interface {
	Overview(context.Context, auth.Principal, string) (Overview, error)
}

func (s *Service) Overview(ctx context.Context, p auth.Principal, owner string) (Overview, error) {
	if err := authorize(p, 1, ""); err != nil {
		return Overview{}, err
	}
	if owner != "" && !ValidID(owner) {
		return Overview{}, ErrInvalid
	}
	r, ok := s.reader.(OverviewReader)
	if !ok {
		return Overview{}, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return Overview{}, err
	}
	record, err := r.ReadOverview(ctx, append([]string(nil), s.bindings...), owner)
	if err != nil {
		return Overview{}, readError(err)
	}
	if err := ctx.Err(); err != nil {
		return Overview{}, err
	}
	if record.InvalidRecords != 0 || record.Source != OverviewSource || record.Scope != OverviewScope || record.OwnerUserID != owner || record.ReadAt.IsZero() || !validOverviewCounts(record.Overview) {
		return Overview{}, ErrUnavailable
	}
	return record.Overview, nil
}

func countSum(values ...int64) (int64, bool) {
	var sum int64
	for _, n := range values {
		if n < 0 || n > maxOverviewCount-sum {
			return 0, false
		}
		sum += n
	}
	return sum, true
}
func validOverviewCounts(o Overview) bool {
	m, j := o.Manifests, o.Jobs
	mt, mok := countSum(m.Pending, m.Active, m.AwaitingVisibility, m.AwaitingCanonical, m.Ready, m.Failed, m.Canceled, m.RecoveryRequired)
	jt, jok := countSum(j.Queued, j.Running, j.RetryWait, j.Succeeded, j.Failed, j.Canceled, j.RecoveryRequired)
	incomplete, iok := countSum(m.Pending, m.Active, m.AwaitingVisibility, m.AwaitingCanonical)
	return mok && jok && iok && m.Total == mt && j.Total == jt && m.Incomplete == incomplete &&
		m.CanceledWithoutJob >= 0 && m.CanceledWithoutJob <= m.Canceled &&
		jt == mt-m.Pending-m.CanceledWithoutJob && j.Queued <= m.Active && j.Succeeded == m.Ready &&
		j.Failed == m.Failed && j.Canceled == m.Canceled-m.CanceledWithoutJob && j.RecoveryRequired == m.RecoveryRequired
}

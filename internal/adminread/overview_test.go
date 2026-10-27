package adminread

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/nathanxiangang-web/panta/internal/auth"
)

type overviewReaderFake struct {
	readerFake
	record   OverviewRecord
	bindings []string
}

func (f *overviewReaderFake) ReadOverview(ctx context.Context, bindings []string, owner string) (OverviewRecord, error) {
	f.calls++
	f.owner = owner
	_, f.deadline = ctx.Deadline()
	f.bindings = append([]string(nil), bindings...)
	bindings[0] = "tampered"
	return f.record, f.err
}
func emptyOverview() OverviewRecord {
	return OverviewRecord{Overview: Overview{Source: OverviewSource, Scope: OverviewScope, ReadAt: time.Now().UTC()}}
}
func TestOverviewZeroScopeAuthorizationAndMissingPort(t *testing.T) {
	f := &overviewReaderFake{record: emptyOverview()}
	s, _ := NewService(f, []string{bid})
	o, err := s.Overview(context.Background(), admin(), "")
	if err != nil || o.Manifests.Total != 0 || o.Jobs.Total != 0 || !f.deadline || f.bindings[0] != bid || s.bindings[0] != bid {
		t.Fatal(o, err)
	}
	f.record.OwnerUserID = aid
	if _, err = s.Overview(context.Background(), admin(), aid); err != nil || f.owner != aid {
		t.Fatal(err)
	}
	for _, p := range []auth.Principal{{}, {UserID: aid, Login: "ordinary", Role: auth.RoleUser}} {
		before := f.calls
		if _, err = s.Overview(context.Background(), p, ""); err == nil || f.calls != before {
			t.Fatal("unauthorized port call")
		}
	}
	if _, err = s.Overview(context.Background(), admin(), "bad"); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	legacy, _ := NewService(&readerFake{}, []string{bid})
	if _, err = legacy.Overview(context.Background(), admin(), ""); !errors.Is(err, ErrUnavailable) {
		t.Fatal("missing port is not zero", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	before := f.calls
	if _, err = s.Overview(ctx, admin(), ""); !errors.Is(err, context.Canceled) || f.calls != before {
		t.Fatal(err)
	}
}
func TestOverviewRejectsUnknownInconsistentOverflowAndFailure(t *testing.T) {
	for name, change := range map[string]func(*OverviewRecord){
		"invalid pair":      func(r *OverviewRecord) { r.InvalidRecords = 1 },
		"negative evidence": func(r *OverviewRecord) { r.InvalidRecords = -1 },
		"unknown scope":     func(r *OverviewRecord) { r.Scope = "ALL_JOBS" },
		"unknown source":    func(r *OverviewRecord) { r.Source = "115" },
		"wrong owner":       func(r *OverviewRecord) { r.OwnerUserID = aid },
		"missing timestamp": func(r *OverviewRecord) { r.ReadAt = time.Time{} },
		"negative":          func(r *OverviewRecord) { r.Manifests.Pending = -1 },
		"unknown manifest":  func(r *OverviewRecord) { r.Manifests.Total = 1 },
		"unknown job":       func(r *OverviewRecord) { r.Jobs.Total = 1 },
		"incomplete":        func(r *OverviewRecord) { r.Manifests.Incomplete = 1 },
		"missing job":       func(r *OverviewRecord) { r.Manifests.Active = 1; r.Manifests.Total = 1; r.Manifests.Incomplete = 1 },
		"wrong terminal": func(r *OverviewRecord) {
			r.Manifests.Total = 1
			r.Manifests.Failed = 1
			r.Jobs.Total = 1
			r.Jobs.Succeeded = 1
		},
		"queued late stage": func(r *OverviewRecord) {
			r.Manifests.Total = 1
			r.Manifests.AwaitingCanonical = 1
			r.Manifests.Incomplete = 1
			r.Jobs.Total = 1
			r.Jobs.Queued = 1
		},
		"int64 overflow": func(r *OverviewRecord) { r.Manifests.Pending = math.MaxInt64; r.Manifests.Active = 1 },
		"json precision": func(r *OverviewRecord) {
			r.Manifests.Pending = maxOverviewCount + 1
			r.Manifests.Total = maxOverviewCount + 1
			r.Manifests.Incomplete = maxOverviewCount + 1
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := emptyOverview()
			change(&r)
			f := &overviewReaderFake{record: r}
			s, _ := NewService(f, []string{bid})
			if _, err := s.Overview(context.Background(), admin(), ""); !errors.Is(err, ErrUnavailable) {
				t.Fatal(err)
			}
		})
	}
	f := &overviewReaderFake{record: emptyOverview(), readerFake: readerFake{err: errors.New("Cookie=SECRET")}}
	s, _ := NewService(f, []string{bid})
	if _, err := s.Overview(context.Background(), admin(), ""); err != ErrUnavailable {
		t.Fatal("raw error leaked", err)
	}
	f.err = context.DeadlineExceeded
	if _, err := s.Overview(context.Background(), admin(), ""); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}
func TestOverviewMixedStatesAndSafeIntegerBoundary(t *testing.T) {
	r := emptyOverview()
	r.Manifests = ManifestCounts{Total: 10, Pending: 1, Active: 3, AwaitingVisibility: 1, AwaitingCanonical: 1, Ready: 1, Failed: 1, Canceled: 1, RecoveryRequired: 1, Incomplete: 6}
	r.Jobs = JobCounts{Total: 9, Queued: 1, Running: 2, RetryWait: 2, Succeeded: 1, Failed: 1, Canceled: 1, RecoveryRequired: 1}
	f := &overviewReaderFake{record: r}
	s, _ := NewService(f, []string{bid})
	if _, err := s.Overview(context.Background(), admin(), ""); err != nil {
		t.Fatal(err)
	}
	f.record = emptyOverview()
	f.record.Manifests = ManifestCounts{Total: maxOverviewCount, Pending: maxOverviewCount, Incomplete: maxOverviewCount}
	if _, err := s.Overview(context.Background(), admin(), ""); err != nil {
		t.Fatal(err)
	}
}

func TestOverviewCanceledWithoutJobKeepsSeparateDenominators(t *testing.T) {
	r := emptyOverview()
	r.Manifests = ManifestCounts{Total: 4, Pending: 1, Active: 1, Canceled: 2, CanceledWithoutJob: 1, Incomplete: 2}
	r.Jobs = JobCounts{Total: 2, Queued: 1, Canceled: 1}
	if !validOverviewCounts(r.Overview) {
		t.Fatal("mixed pending/canceled intents and real Jobs refused")
	}
	for _, change := range []func(*Overview){
		func(o *Overview) { o.Manifests.CanceledWithoutJob = -1 },
		func(o *Overview) { o.Manifests.CanceledWithoutJob = 3 },
		func(o *Overview) { o.Manifests.CanceledWithoutJob = 0 },
		func(o *Overview) { o.Jobs.Canceled = 2; o.Jobs.Total = 3 },
	} {
		o := r.Overview
		change(&o)
		if validOverviewCounts(o) {
			t.Fatal("inconsistent no-Job count accepted", o)
		}
	}
}

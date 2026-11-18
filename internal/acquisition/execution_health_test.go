package acquisition_test

import (
	"context"
	"errors"
	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/providers/contracts"
	"github.com/nathanxiangang-web/panta/internal/sessionhealth"
	"testing"
)

type healthProvider struct {
	*scriptedProvider
	status sessionhealth.Status
	checks int
}

func (p *healthProvider) CheckSession(context.Context) sessionhealth.Status {
	p.checks++
	return p.status
}
func TestExecutionPrestartHealthDoesNotPostOrReplayReservation(t *testing.T) {
	for _, tc := range []struct {
		status sessionhealth.Status
		want   error
	}{
		{sessionhealth.SessionLoginRequired, acquisition.ErrProviderLoginBeforeStart},
		{sessionhealth.SessionUnknown, acquisition.ErrProviderUnknownBeforeStart},
	} {
		f := newExecutionFixture(t)
		p := &healthProvider{scriptedProvider: f.provider, status: tc.status}
		f.sessions.bindings[newSessionKey(executionProviderID, string(executionConnID))] = contracts.DownloaderBinding{Descriptor: p.Descriptor(), Downloader: p}
		_, err := f.service.Execute(context.Background(), f.request)
		if !errors.Is(err, tc.want) || errors.Is(err, acquisition.ErrExecutionSideEffectUncertain) {
			t.Fatal(err)
		}
		start, poll, _ := p.counts()
		if start != 0 || poll != 0 || p.checks != 1 {
			t.Fatal(start, poll, p.checks)
		}
		if f.durableTask(t).ReferenceKnown() {
			t.Fatal("fabricated reference")
		}
		// A later execution/restart still cannot reuse an existing reservation.
		p.status = sessionhealth.SessionAuthenticated
		_, err = f.service.Execute(context.Background(), f.request)
		if !errors.Is(err, acquisition.ErrExecutionSideEffectUncertain) || p.checks != 1 {
			t.Fatal(err, p.checks)
		}
	}
}
func TestExecutionGoodHealthNeverMakesPostFailureSafe(t *testing.T) {
	f := newExecutionFixture(t)
	p := &healthProvider{scriptedProvider: f.provider, status: sessionhealth.SessionAuthenticated}
	p.startError = context.DeadlineExceeded
	f.sessions.bindings[newSessionKey(executionProviderID, string(executionConnID))] = contracts.DownloaderBinding{Descriptor: p.Descriptor(), Downloader: p}
	_, err := f.service.Execute(context.Background(), f.request)
	if !errors.Is(err, acquisition.ErrExecutionSideEffectUncertain) {
		t.Fatal(err)
	}
	starts, _, _ := p.counts()
	if starts != 1 {
		t.Fatal(starts)
	}
}

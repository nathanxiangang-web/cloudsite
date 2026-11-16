package sessionhealth

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type probeFunc func(context.Context) Status

func (f probeFunc) CheckSession(ctx context.Context) Status { return f(ctx) }

func TestSessionHealthSingleFlightCacheAndExpiry(t *testing.T) {
	var calls atomic.Int32
	entered, release := make(chan struct{}), make(chan struct{})
	h := New(probeFunc(func(ctx context.Context) Status {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
		return SessionAuthenticated
	}))
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if v := h.Read(context.Background()); v.Status != SessionAuthenticated || v.ValidUntil.Sub(v.CheckedAt) != MaxAge {
				t.Errorf("bad view: %v", v)
			}
		}()
	}
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if h.Read(ctx).Status != SessionUnknown {
		t.Fatal("canceled waiter")
	}
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal("fanout", calls.Load())
	}
	h.mu.Lock()
	h.view.ValidUntil = time.Now().Add(-time.Second)
	h.mu.Unlock()
	h.Read(context.Background())
	if calls.Load() != 2 {
		t.Fatal("expired cache not renewed")
	}
}
func TestSessionHealthFailureCacheAndUnsupportedFailClosed(t *testing.T) {
	for _, status := range []Status{SessionLoginRequired, SessionUnknown, "unexpected private body"} {
		calls := 0
		h := New(probeFunc(func(context.Context) Status { calls++; return status }))
		v := h.Read(context.Background())
		h.Read(context.Background())
		want := status
		if status != SessionLoginRequired {
			want = SessionUnknown
		}
		if calls != 1 || v.Status != want {
			t.Fatal(calls, v)
		}
	}
	if New(nil).Read(context.Background()).Status != SessionUnknown {
		t.Fatal("unsupported")
	}
	h := New(probeFunc(func(ctx context.Context) Status { <-ctx.Done(); return SessionAuthenticated }))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if h.Read(ctx).Status != SessionUnknown {
		t.Fatal("cancellation")
	}
}

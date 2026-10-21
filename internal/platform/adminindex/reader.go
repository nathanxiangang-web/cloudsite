// Package adminindex adapts only the existing Q9 port to the admin read domain.
package adminindex

import (
	"context"
	"errors"

	"github.com/nathanxiangang-web/panta/internal/adminobserve"
	"github.com/nathanxiangang-web/panta/internal/integrations/indexcore"
)

type Reader struct{ port indexcore.RootStatusReadPort }

func New(port indexcore.RootStatusReadPort) (*Reader, error) {
	if adminobserve.Missing(port) {
		return nil, adminobserve.ErrNotConnected
	}
	return &Reader{port: port}, nil
}
func (r *Reader) ReadRoot(ctx context.Context, id string) (adminobserve.RootState, error) {
	s, err := r.port.RootStatus(ctx, indexcore.RootStatusRequest{RootID: id, IncludeDeprecatedRoot: true, IncludeDeletedRoot: true})
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled):
			return adminobserve.RootState{}, context.Canceled
		case errors.Is(err, context.DeadlineExceeded):
			return adminobserve.RootState{}, context.DeadlineExceeded
		case errors.Is(err, indexcore.ErrNotFound):
			return adminobserve.RootState{}, adminobserve.ErrRootNotFound
		case errors.Is(err, indexcore.ErrNotReady):
			return adminobserve.RootState{}, adminobserve.ErrNotReady
		default:
			return adminobserve.RootState{}, adminobserve.ErrUnavailable
		}
	}
	return adminobserve.RootState{RootID: s.RootID, Lifecycle: string(s.LifecycleState), Generation: s.CurrentGeneration, LastAdmission: s.LastAppliedAdmissionSeq}, nil
}

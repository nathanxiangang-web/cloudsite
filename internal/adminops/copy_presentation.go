package adminops

import (
	"context"
	"github.com/nathanxiangang-web/panta/internal/adminread"
	"github.com/nathanxiangang-web/panta/internal/copypresentation"
	"time"
)

const SetCopyPresentation = "SET_COPY_PRESENTATION"

// Separate ports keep physical resources outside user/share/provider execution.
type CopyPresentationApplier interface {
	ApplyCopyPresentation(context.Context, Actor, Command, []string) (Result, error)
}
type CopyPresentationReader interface {
	ReadCopyPresentation(context.Context, Actor, string, string, []string) (copypresentation.Detail, error)
}
type CopyPresentationReads interface {
	CopyPresentation(context.Context, Actor, string, string) (copypresentation.Detail, error)
}

func (s *Service) CopyPresentation(ctx context.Context, a Actor, copyID, bindingID string) (copypresentation.Detail, error) {
	if s == nil {
		return copypresentation.Detail{}, ErrUnavailable
	}
	if e := authorize(a); e != nil {
		return copypresentation.Detail{}, e
	}
	if !adminread.ValidID(copyID) || !adminread.ValidID(bindingID) {
		return copypresentation.Detail{}, ErrInvalid
	}
	reader, ok := s.store.(CopyPresentationReader)
	if !ok {
		return copypresentation.Detail{}, ErrUnavailable
	}
	ctx, stop := context.WithTimeout(ctx, 5*time.Second)
	defer stop()
	d, e := reader.ReadCopyPresentation(ctx, a, copyID, bindingID, append([]string(nil), s.bindings...))
	return d, safe(e)
}

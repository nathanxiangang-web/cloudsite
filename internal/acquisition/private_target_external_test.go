package acquisition_test

import (
	"context"

	"github.com/nathanxiangang-web/panta/internal/acquisition"
)

type privateTargetReaderDouble struct {
	space acquisition.PrivateTargetSpace
	err   error
}

func (reader privateTargetReaderDouble) GetPrivateTargetSpace(context.Context, string) (acquisition.PrivateTargetSpace, error) {
	if reader.err != nil {
		return acquisition.PrivateTargetSpace{}, reader.err
	}
	return reader.space, nil
}

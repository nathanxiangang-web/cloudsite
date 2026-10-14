package main

import (
	"context"
	"errors"
	"testing"

	"github.com/nathanxiangang-web/panta/internal/platform/operator"
	"github.com/nathanxiangang-web/panta/internal/platform/productserver"
)

func TestProductModesRejectWorkerAndCredentialArguments(t *testing.T) {
	t.Setenv("PANTA_ACQUISITION_WORKER_ENABLED", "true")
	t.Setenv("PANTA_PRODUCT_CONFIG_FILE", "/missing/protected.json")
	for _, action := range []string{productserver.ServeAction, productserver.ProvisionAction} {
		if err := runWithContext(context.Background(), []string{action}); !errors.Is(err, productserver.ErrConfig) {
			t.Fatal("worker-enabled product mode admitted", err)
		}
		if err := runWithContext(context.Background(), []string{action, "--password=unsafe"}); !errors.Is(err, operator.ErrInvalidAction) {
			t.Fatal("credential arguments admitted")
		}
	}
	t.Setenv("PANTA_ACQUISITION_WORKER_ENABLED", "false")
	if err := runWithContext(context.Background(), []string{productserver.ProvisionAction}); !errors.Is(err, productserver.ErrConfig) {
		t.Fatal("unprotected config fallback", err)
	}
}

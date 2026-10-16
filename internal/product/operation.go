package product

import (
	"context"
	"github.com/nathanxiangang-web/panta/internal/auth"
)

// OperationReader reconciles a caller-owned key without source disclosure,
// activation, quota reservation or a replacement operation.
type OperationReader interface {
	Operation(context.Context, auth.Principal, string) (AdmissionDTO, error)
}

func ValidOperationKey(key string) bool { return operationKeyPattern.MatchString(key) }

package acquisition

import (
	"errors"
	"testing"
	"time"
)

func validResultObjectEvidence() ResultObjectEvidence {
	return ResultObjectEvidence{
		ManifestID: "manifest-id", ProviderConnectionID: "connection-id",
		ProviderObjectRef: "file-id", ProviderParentScope: "parent-id",
		ResultName: "result.bin", SizeBytes: 42, HashAlgorithm: "SHA1",
		ContentHash: "0123456789abcdef0123456789abcdef01234567", ObservedAt: time.Unix(100, 0),
	}
}

func validCopyOperation() CopyOperation {
	return CopyOperation{
		ID: "copy-id", ManifestID: "manifest-id", JobID: "job-id", DonorManifestID: "donor-id",
		ProviderConnectionID: "connection-id", RequestDigest: make([]byte, 32),
		DestinationBefore: []string{}, Status: CopyReserved,
		CreatedAt: time.Unix(100, 0), UpdatedAt: time.Unix(100, 0),
	}
}

func TestResultObjectEvidenceValidate(t *testing.T) {
	evidence := validResultObjectEvidence()
	if err := evidence.Validate(); err != nil {
		t.Fatal(err)
	}
	evidence.ContentHash = "invalid"
	if err := evidence.Validate(); !errors.Is(err, ErrInvalidResultEvidence) {
		t.Fatalf("error = %v, want ErrInvalidResultEvidence", err)
	}
}

func TestCopyOperationValidate(t *testing.T) {
	operation := validCopyOperation()
	if err := operation.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*CopyOperation){
		"short digest":   func(value *CopyOperation) { value.RequestDigest = make([]byte, 31) },
		"nil baseline":   func(value *CopyOperation) { value.DestinationBefore = nil },
		"invalid status": func(value *CopyOperation) { value.Status = "READY" },
		"invalid error kind": func(value *CopyOperation) {
			kind := ProviderErrorKind("403")
			value.LastErrorKind = &kind
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := operation
			mutate(&candidate)
			if err := candidate.Validate(); !errors.Is(err, ErrInvalidCopyOperation) {
				t.Fatalf("error = %v, want ErrInvalidCopyOperation", err)
			}
		})
	}
}

func TestCopyOperationConfirmedRequiresResultReference(t *testing.T) {
	operation := validCopyOperation()
	operation.Status = CopyProviderConfirmed
	if err := operation.Validate(); !errors.Is(err, ErrInvalidCopyOperation) {
		t.Fatalf("error = %v, want ErrInvalidCopyOperation", err)
	}
	result := "new-file-id"
	operation.ProviderResultRef = &result
	if err := operation.Validate(); err != nil {
		t.Fatal(err)
	}
}

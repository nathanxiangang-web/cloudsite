package contracts

import (
	"errors"
	"testing"
)

func TestValidateObjectEvidence(t *testing.T) {
	object := StorageObject{
		Reference: StorageObjectReference{Scope: "parent", Path: "file"},
		Name:      "result.bin", SizeBytes: 1, HashAlgorithm: ContentHashSHA1,
		ContentHash: "0123456789abcdef0123456789abcdef01234567",
	}
	if err := ValidateObjectEvidence(object); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*StorageObject){
		"blank scope":     func(value *StorageObject) { value.Reference.Scope = " " },
		"blank object":    func(value *StorageObject) { value.Reference.Path = "" },
		"directory":       func(value *StorageObject) { value.Directory = true },
		"path name":       func(value *StorageObject) { value.Name = "nested/result.bin" },
		"negative size":   func(value *StorageObject) { value.SizeBytes = -1 },
		"wrong algorithm": func(value *StorageObject) { value.HashAlgorithm = "SHA256" },
		"wrong hash":      func(value *StorageObject) { value.ContentHash = "xyz" },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := object
			mutate(&candidate)
			if err := ValidateObjectEvidence(candidate); !errors.Is(err, ErrInvalidStorageObject) {
				t.Fatalf("error = %v, want ErrInvalidStorageObject", err)
			}
		})
	}
}

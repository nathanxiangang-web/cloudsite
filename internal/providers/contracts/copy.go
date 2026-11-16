package contracts

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"
)

const ContentHashSHA1 = "SHA1"

var ErrInvalidStorageObject = errors.New("provider storage object evidence is invalid")

type ObjectCopyProvider interface {
	InspectObject(context.Context, StorageObjectReference) (StorageObject, error)
	ListObjects(context.Context, string) ([]StorageObject, error)
	CopyObject(context.Context, StorageObjectReference, string) error
}

type DirectoryMatch struct {
	DirectoryID string
	ParentID    string
	Name        string
	Found       bool
}

type DirectoryProvider interface {
	FindDirectory(context.Context, string, string) (DirectoryMatch, error)
	CreateDirectory(context.Context, string, string) (string, error)
}

func ValidateObjectReference(reference StorageObjectReference) error {
	if !validOpaqueObjectField(reference.Scope) || !validOpaqueObjectField(reference.Path) {
		return ErrInvalidStorageObject
	}
	return nil
}

func ValidateObjectEvidence(object StorageObject) error {
	if err := ValidateObjectReference(object.Reference); err != nil {
		return err
	}
	if object.Directory || ValidateDirectChildName(object.Name) != nil || object.SizeBytes < 0 {
		return ErrInvalidStorageObject
	}
	if object.HashAlgorithm != ContentHashSHA1 || len(object.ContentHash) != 40 {
		return ErrInvalidStorageObject
	}
	for _, value := range object.ContentHash {
		if (value < '0' || value > '9') && (value < 'a' || value > 'f') && (value < 'A' || value > 'F') {
			return ErrInvalidStorageObject
		}
	}
	return nil
}

func validOpaqueObjectField(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && utf8.ValidString(value) &&
		!strings.ContainsRune(value, '\x00') && utf8.RuneCountInString(value) <= 256
}

package userdirectory

import (
	"context"

	"github.com/nathanxiangang-web/panta/internal/product"
	"github.com/nathanxiangang-web/panta/internal/providers/contracts"
)

type Provider struct {
	Directory contracts.DirectoryProvider
}

func (provider Provider) FindDirectory(ctx context.Context, parentID, name string) (product.CloudDirectoryMatch, error) {
	match, err := provider.Directory.FindDirectory(ctx, parentID, name)
	if err != nil {
		return product.CloudDirectoryMatch{}, err
	}
	return product.CloudDirectoryMatch{DirectoryID: match.DirectoryID, ParentID: match.ParentID, Name: match.Name, Found: match.Found}, nil
}

func (provider Provider) CreateDirectory(ctx context.Context, parentID, name string) (string, error) {
	return provider.Directory.CreateDirectory(ctx, parentID, name)
}

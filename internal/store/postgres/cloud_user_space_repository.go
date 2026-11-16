package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/product"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

const cloudUserSpaceColumns = `
space_id::text, owner_user_id::text, binding_id::text, provider_connection_id::text,
indexcore_root_id, native_parent_id, native_dir_id, canonical_prefix, directory_name,
status, owner_state, provisioning_key, version, last_verified_at, created_at, updated_at`

type cloudUserSpaceDB interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type CloudUserSpaceRepository struct {
	db cloudUserSpaceDB
}

func NewCloudUserSpaceRepository(db cloudUserSpaceDB) (*CloudUserSpaceRepository, error) {
	if db == nil {
		return nil, errors.New("cloud user space database is required")
	}
	return &CloudUserSpaceRepository{db: db}, nil
}

func (repository *CloudUserSpaceRepository) CreateSpace(ctx context.Context, space product.CloudUserSpace) error {
	if err := space.Validate(); err != nil {
		return err
	}
	_, err := repository.db.Exec(ctx, `INSERT INTO cloud_user_spaces (
space_id, owner_user_id, binding_id, provider_connection_id, indexcore_root_id,
native_parent_id, native_dir_id, canonical_prefix, directory_name, status, owner_state,
provisioning_key, version, last_verified_at, created_at, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
		string(space.ID), space.OwnerUserID, string(space.BindingID), string(space.ProviderConnectionID),
		space.IndexCoreRootID, space.NativeParentID, space.NativeDirID, space.CanonicalPrefix,
		space.DirectoryName, string(space.Status), string(space.OwnerState), space.ProvisioningKey,
		space.Version, space.LastVerifiedAt, space.CreatedAt, space.UpdatedAt)
	return cloudUserSpaceError("create", err)
}

func (repository *CloudUserSpaceRepository) GetSpace(ctx context.Context, id product.UserSpaceID) (product.CloudUserSpace, error) {
	if id == "" {
		return product.CloudUserSpace{}, product.ErrInvalidUserSpace
	}
	space, err := scanCloudUserSpace(repository.db.QueryRow(ctx, `SELECT `+cloudUserSpaceColumns+` FROM cloud_user_spaces WHERE space_id=$1`, string(id)))
	if errors.Is(err, pgx.ErrNoRows) {
		return product.CloudUserSpace{}, product.ErrUserSpaceNotFound
	}
	if err != nil {
		return product.CloudUserSpace{}, cloudUserSpaceError("get", err)
	}
	return space, nil
}

func (repository *CloudUserSpaceRepository) GetSpaceByOwner(ctx context.Context, ownerUserID string, bindingID storage.BindingID) (product.CloudUserSpace, error) {
	if ownerUserID == "" || bindingID == "" {
		return product.CloudUserSpace{}, product.ErrInvalidUserSpace
	}
	space, err := scanCloudUserSpace(repository.db.QueryRow(ctx, `SELECT `+cloudUserSpaceColumns+` FROM cloud_user_spaces WHERE owner_user_id=$1 AND binding_id=$2`, ownerUserID, string(bindingID)))
	if errors.Is(err, pgx.ErrNoRows) {
		return product.CloudUserSpace{}, product.ErrUserSpaceNotFound
	}
	if err != nil {
		return product.CloudUserSpace{}, cloudUserSpaceError("get by owner", err)
	}
	return space, nil
}

func (repository *CloudUserSpaceRepository) GetPrivateTargetSpace(ctx context.Context, id string) (acquisition.PrivateTargetSpace, error) {
	space, err := repository.GetSpace(ctx, product.UserSpaceID(id))
	if err != nil {
		return acquisition.PrivateTargetSpace{}, err
	}
	target := acquisition.PrivateTargetSpace{
		ID: string(space.ID), OwnerUserID: space.OwnerUserID, BindingID: space.BindingID,
		ProviderConnectionID: space.ProviderConnectionID, IndexCoreRootID: space.IndexCoreRootID,
		CanonicalPrefix: space.CanonicalPrefix, Status: string(space.Status), OwnerState: string(space.OwnerState),
	}
	if space.NativeDirID != nil {
		target.NativeDirID = *space.NativeDirID
	}
	return target, nil
}

func (repository *CloudUserSpaceRepository) UpdateSpace(ctx context.Context, space product.CloudUserSpace, expectedVersion int64) error {
	if space.OwnerState != product.UserSpaceOwnerActive {
		return product.ErrUserSpaceConflict
	}
	if err := space.Validate(); err != nil || expectedVersion < 0 || space.Version != expectedVersion+1 {
		if err == nil {
			err = product.ErrInvalidUserSpace
		}
		return err
	}
	tag, err := repository.db.Exec(ctx, `UPDATE cloud_user_spaces SET
provider_connection_id=$2, indexcore_root_id=$3, native_parent_id=$4, native_dir_id=$5,
canonical_prefix=$6, directory_name=$7, status=$8,
version=$9, last_verified_at=$10, updated_at=$11
WHERE space_id=$1 AND version=$12 AND owner_state='ACTIVE'`,
		string(space.ID), string(space.ProviderConnectionID), space.IndexCoreRootID,
		space.NativeParentID, space.NativeDirID, space.CanonicalPrefix, space.DirectoryName,
		string(space.Status), space.Version, space.LastVerifiedAt, space.UpdatedAt, expectedVersion)
	if err != nil {
		return cloudUserSpaceError("update", err)
	}
	if tag.RowsAffected() != 1 {
		return product.ErrUserSpaceConflict
	}
	return nil
}

func scanCloudUserSpace(row pgx.Row) (product.CloudUserSpace, error) {
	var space product.CloudUserSpace
	var id, ownerID, bindingID, connectionID string
	var nativeDirID sql.NullString
	err := row.Scan(&id, &ownerID, &bindingID, &connectionID, &space.IndexCoreRootID,
		&space.NativeParentID, &nativeDirID, &space.CanonicalPrefix, &space.DirectoryName,
		&space.Status, &space.OwnerState, &space.ProvisioningKey, &space.Version,
		&space.LastVerifiedAt, &space.CreatedAt, &space.UpdatedAt)
	if err != nil {
		return product.CloudUserSpace{}, err
	}
	space.ID = product.UserSpaceID(id)
	space.OwnerUserID = ownerID
	space.BindingID = storage.BindingID(bindingID)
	space.ProviderConnectionID = storage.ConnectionID(connectionID)
	if nativeDirID.Valid {
		space.NativeDirID = &nativeDirID.String
	}
	return space, nil
}

func cloudUserSpaceError(operation string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23505":
			return fmt.Errorf("%w: %s", product.ErrUserSpaceConflict, postgresError.ConstraintName)
		case "23503", "23514", "22P02":
			return fmt.Errorf("%w: %s", product.ErrInvalidUserSpace, postgresError.ConstraintName)
		}
	}
	return fmt.Errorf("cloud user space %s: %w", operation, err)
}

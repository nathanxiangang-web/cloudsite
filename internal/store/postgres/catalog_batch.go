package postgres

import (
	"context"
	"database/sql"

	"github.com/nathanxiangang-web/panta/internal/catalog"
)

func (r *CatalogRepository) GetCopiesByPhysicalIdentity(ctx context.Context, root string, ids []string) (map[string]catalog.Copy, error) {
	if root == "" || len(ids) > 50 {
		return nil, catalog.ErrInvalidClassification
	}
	result := map[string]catalog.Copy{}
	if len(ids) == 0 {
		return result, nil
	}
	rows, err := r.db.Query(ctx, `SELECT copy_id::text,variant_id::text,indexcore_root_id,indexcore_resource_id,storage_binding_id::text,availability,created_at,updated_at FROM copies WHERE indexcore_root_id=$1 AND indexcore_resource_id=ANY($2::text[])`, root, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c catalog.Copy
		var copyID, binding string
		var variant sql.NullString
		if err = rows.Scan(&copyID, &variant, &c.IndexCoreRootID, &c.IndexCoreResourceID, &binding, &c.Availability, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.ID = catalog.CopyID(copyID)
		c.StorageBindingID = catalog.StorageBindingID(binding)
		if variant.Valid {
			v := catalog.VariantID(variant.String)
			c.VariantID = &v
		}
		if _, exists := result[c.IndexCoreResourceID]; exists {
			return nil, catalog.ErrInvalidClassification
		}
		result[c.IndexCoreResourceID] = c
	}
	return result, rows.Err()
}

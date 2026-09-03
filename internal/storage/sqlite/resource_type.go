package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ngochc/dev-dash/internal/resource"
)

type ResourceTypeRepository struct {
	db *sql.DB
}

func NewResourceTypeRepository(db *sql.DB) *ResourceTypeRepository {
	return &ResourceTypeRepository{db: db}
}

func (r *ResourceTypeRepository) Create(ctx context.Context, item resource.ResourceType) error {
	result, err := r.db.ExecContext(ctx, `
		INSERT INTO resource_types (name, display_name, owner, description)
		VALUES (?, ?, NULLIF(?, ''), NULLIF(?, ''))
		ON CONFLICT(name) DO NOTHING
	`, item.Name, item.DisplayName, item.Owner, item.Description)
	if err != nil {
		return fmt.Errorf("insert resource type: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read inserted resource type count: %w", err)
	}
	if rowsAffected == 0 {
		return resource.ErrTypeExists
	}
	return nil
}

func (r *ResourceTypeRepository) List(ctx context.Context) ([]resource.ResourceType, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT name, display_name, COALESCE(owner, ''), COALESCE(description, ''), created_at
		FROM resource_types
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("query resource types: %w", err)
	}
	defer rows.Close()

	var items []resource.ResourceType
	for rows.Next() {
		var item resource.ResourceType
		if err := rows.Scan(&item.Name, &item.DisplayName, &item.Owner, &item.Description, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan resource type: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate resource types: %w", err)
	}
	return items, nil
}

func (r *ResourceTypeRepository) Get(ctx context.Context, name string) (resource.ResourceType, error) {
	var item resource.ResourceType
	err := r.db.QueryRowContext(ctx, `
		SELECT name, display_name, COALESCE(owner, ''), COALESCE(description, ''), created_at
		FROM resource_types
		WHERE name = ?
	`, name).Scan(&item.Name, &item.DisplayName, &item.Owner, &item.Description, &item.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return resource.ResourceType{}, resource.ErrTypeNotFound
	}
	if err != nil {
		return resource.ResourceType{}, fmt.Errorf("query resource type: %w", err)
	}
	return item, nil
}

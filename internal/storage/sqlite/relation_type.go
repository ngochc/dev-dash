package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ngochc/dev-dash/internal/graph"
)

type RelationTypeRepository struct {
	db *sql.DB
}

func NewRelationTypeRepository(db *sql.DB) *RelationTypeRepository {
	return &RelationTypeRepository{db: db}
}

func (r *RelationTypeRepository) Create(ctx context.Context, item graph.RelationType) error {
	result, err := r.db.ExecContext(ctx, `
		INSERT INTO relation_types (name, display_name, inverse_name, symmetric, owner, description)
		VALUES (?, ?, NULLIF(?, ''), ?, NULLIF(?, ''), NULLIF(?, ''))
		ON CONFLICT(name) DO NOTHING
	`, item.Name, item.DisplayName, item.InverseName, item.Symmetric, item.Owner, item.Description)
	if err != nil {
		return fmt.Errorf("insert relation type: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read inserted relation type count: %w", err)
	}
	if rowsAffected == 0 {
		return graph.ErrTypeExists
	}
	return nil
}

func (r *RelationTypeRepository) List(ctx context.Context) ([]graph.RelationType, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT name, COALESCE(display_name, ''), COALESCE(inverse_name, ''), symmetric,
		       COALESCE(owner, ''), COALESCE(description, ''), created_at
		FROM relation_types
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("query relation types: %w", err)
	}
	defer rows.Close()

	var items []graph.RelationType
	for rows.Next() {
		var item graph.RelationType
		if err := rows.Scan(&item.Name, &item.DisplayName, &item.InverseName, &item.Symmetric, &item.Owner, &item.Description, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan relation type: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate relation types: %w", err)
	}
	return items, nil
}

func (r *RelationTypeRepository) Get(ctx context.Context, name string) (graph.RelationType, error) {
	var item graph.RelationType
	err := r.db.QueryRowContext(ctx, `
		SELECT name, COALESCE(display_name, ''), COALESCE(inverse_name, ''), symmetric,
		       COALESCE(owner, ''), COALESCE(description, ''), created_at
		FROM relation_types
		WHERE name = ?
	`, name).Scan(&item.Name, &item.DisplayName, &item.InverseName, &item.Symmetric, &item.Owner, &item.Description, &item.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return graph.RelationType{}, graph.ErrTypeNotFound
	}
	if err != nil {
		return graph.RelationType{}, fmt.Errorf("query relation type: %w", err)
	}
	return item, nil
}

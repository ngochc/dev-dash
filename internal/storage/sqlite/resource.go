package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ngochc/dev-dash/internal/resource"
)

type ResourceRepository struct {
	db *sql.DB
}

func NewResourceRepository(db *sql.DB) *ResourceRepository {
	return &ResourceRepository{db: db}
}

func (r *ResourceRepository) Create(ctx context.Context, item resource.Resource) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO resources (id, type, name, url)
		VALUES (?, ?, ?, NULLIF(?, ''))
	`, item.ID, item.Type, item.Name, item.URL)
	if err != nil {
		return fmt.Errorf("insert resource: %w", err)
	}
	return nil
}

func (r *ResourceRepository) List(ctx context.Context) ([]resource.Resource, error) {
	rows, err := r.db.QueryContext(ctx, resourceSelect+` ORDER BY name, id`)
	if err != nil {
		return nil, fmt.Errorf("query resources: %w", err)
	}
	defer rows.Close()

	var items []resource.Resource
	for rows.Next() {
		row := newResourceRow()
		if err := rows.Scan(row.destinations()...); err != nil {
			return nil, fmt.Errorf("scan resource: %w", err)
		}
		items = append(items, row.value())
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate resources: %w", err)
	}
	return items, nil
}

func (r *ResourceRepository) Get(ctx context.Context, id string) (resource.Resource, error) {
	row := newResourceRow()
	err := r.db.QueryRowContext(ctx, resourceSelect+` WHERE id = ?`, id).Scan(row.destinations()...)
	if errors.Is(err, sql.ErrNoRows) {
		return resource.Resource{}, resource.ErrNotFound
	}
	if err != nil {
		return resource.Resource{}, fmt.Errorf("query resource: %w", err)
	}
	return row.value(), nil
}

func (r *ResourceRepository) Update(ctx context.Context, item resource.Resource) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE resources
		SET type = ?, name = ?, url = NULLIF(?, ''), updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, item.Type, item.Name, item.URL, item.ID)
	if err != nil {
		return fmt.Errorf("update resource: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read updated resource count: %w", err)
	}
	if rowsAffected == 0 {
		return resource.ErrNotFound
	}
	return nil
}

func (r *ResourceRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM resources WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete resource: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted resource count: %w", err)
	}
	if rowsAffected == 0 {
		return resource.ErrNotFound
	}
	return nil
}

const resourceSelect = `
	SELECT id, type, integration_id, provider_id, external_key, name, url, metadata,
	       created_at, updated_at, last_seen_at
	FROM resources`

type resourceRow struct {
	item          resource.Resource
	integrationID sql.NullString
	providerID    sql.NullString
	externalKey   sql.NullString
	name          sql.NullString
	url           sql.NullString
	metadata      sql.NullString
	lastSeenAt    sql.NullTime
}

func newResourceRow() *resourceRow {
	return &resourceRow{}
}

func (r *resourceRow) destinations() []any {
	return []any{
		&r.item.ID,
		&r.item.Type,
		&r.integrationID,
		&r.providerID,
		&r.externalKey,
		&r.name,
		&r.url,
		&r.metadata,
		&r.item.CreatedAt,
		&r.item.UpdatedAt,
		&r.lastSeenAt,
	}
}

func (r *resourceRow) value() resource.Resource {
	r.item.IntegrationID = r.integrationID.String
	r.item.ProviderID = r.providerID.String
	r.item.ExternalKey = r.externalKey.String
	r.item.Name = r.name.String
	r.item.URL = r.url.String
	r.item.Metadata = r.metadata.String
	if r.lastSeenAt.Valid {
		lastSeenAt := r.lastSeenAt.Time
		r.item.LastSeenAt = &lastSeenAt
	}
	return r.item
}

package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ngochc/dev-dash/internal/workspace"
)

type WorkspaceResourceRepository struct {
	db *sql.DB
}

func NewWorkspaceResourceRepository(db *sql.DB) *WorkspaceResourceRepository {
	return &WorkspaceResourceRepository{db: db}
}

func (r *WorkspaceResourceRepository) Add(ctx context.Context, item workspace.ResourceMembership) error {
	result, err := r.db.ExecContext(ctx, `
		INSERT INTO workspace_resources (workspace_id, resource_id, role)
		VALUES (?, ?, NULLIF(?, ''))
		ON CONFLICT(workspace_id, resource_id) DO NOTHING
	`, item.WorkspaceID, item.ResourceID, item.Role)
	if err != nil {
		return fmt.Errorf("insert workspace resource: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read inserted workspace resource count: %w", err)
	}
	if rowsAffected == 0 {
		return workspace.ErrMembershipExists
	}
	return nil
}

func (r *WorkspaceResourceRepository) ListByWorkspace(ctx context.Context, workspaceID string) ([]workspace.ResourceMembership, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT wr.workspace_id, wr.resource_id, COALESCE(wr.role, ''), wr.created_at,
		       r.id, r.type, r.integration_id, r.provider_id, r.external_key, r.name, r.url,
		       r.metadata, r.created_at, r.updated_at, r.last_seen_at
		FROM workspace_resources AS wr
		JOIN resources AS r ON r.id = wr.resource_id
		WHERE wr.workspace_id = ?
		ORDER BY r.name, r.id
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("query workspace resources: %w", err)
	}
	defer rows.Close()

	var items []workspace.ResourceMembership
	for rows.Next() {
		var item workspace.ResourceMembership
		resourceRow := newResourceRow()
		destinations := []any{&item.WorkspaceID, &item.ResourceID, &item.Role, &item.CreatedAt}
		destinations = append(destinations, resourceRow.destinations()...)
		if err := rows.Scan(destinations...); err != nil {
			return nil, fmt.Errorf("scan workspace resource: %w", err)
		}
		item.Resource = resourceRow.value()
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspace resources: %w", err)
	}
	return items, nil
}

func (r *WorkspaceResourceRepository) Remove(ctx context.Context, workspaceID, resourceID string) error {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM workspace_resources
		WHERE workspace_id = ? AND resource_id = ?
	`, workspaceID, resourceID)
	if err != nil {
		return fmt.Errorf("delete workspace resource: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted workspace resource count: %w", err)
	}
	if rowsAffected == 0 {
		return workspace.ErrMembershipNotFound
	}
	return nil
}

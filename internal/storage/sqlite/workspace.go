package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ngochc/dev-dash/internal/workspace"
)

// WorkspaceRepository persists workspaces in SQLite.
type WorkspaceRepository struct {
	db *sql.DB
}

func NewWorkspaceRepository(db *sql.DB) *WorkspaceRepository {
	return &WorkspaceRepository{db: db}
}

func (r *WorkspaceRepository) Create(ctx context.Context, item workspace.Workspace) error {
	result, err := r.db.ExecContext(ctx, `
		INSERT INTO workspaces (id, name, local_path)
		VALUES (?, ?, ?)
		ON CONFLICT(name) DO NOTHING
	`, item.ID, item.Name, item.LocalPath)
	if err != nil {
		return fmt.Errorf("insert workspace: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read inserted workspace count: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("%w: %q", workspace.ErrNameExists, item.Name)
	}
	return nil
}

func (r *WorkspaceRepository) List(ctx context.Context) ([]workspace.Workspace, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, COALESCE(local_path, '')
		FROM workspaces
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("query workspaces: %w", err)
	}
	defer rows.Close()

	var items []workspace.Workspace
	for rows.Next() {
		var item workspace.Workspace
		if err := rows.Scan(&item.ID, &item.Name, &item.LocalPath); err != nil {
			return nil, fmt.Errorf("scan workspace: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspaces: %w", err)
	}
	return items, nil
}

func (r *WorkspaceRepository) GetByID(ctx context.Context, id string) (workspace.Workspace, error) {
	return r.get(ctx, "id", id)
}

func (r *WorkspaceRepository) GetByName(ctx context.Context, name string) (workspace.Workspace, error) {
	return r.get(ctx, "name", name)
}

func (r *WorkspaceRepository) get(ctx context.Context, column, value string) (workspace.Workspace, error) {
	query := fmt.Sprintf(`
		SELECT id, name, COALESCE(local_path, '')
		FROM workspaces
		WHERE %s = ?
	`, column)

	var item workspace.Workspace
	if err := r.db.QueryRowContext(ctx, query, value).Scan(&item.ID, &item.Name, &item.LocalPath); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return workspace.Workspace{}, workspace.ErrNotFound
		}
		return workspace.Workspace{}, fmt.Errorf("query workspace by %s: %w", column, err)
	}
	return item, nil
}

func (r *WorkspaceRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM workspaces WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete workspace: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted workspace count: %w", err)
	}
	if rowsAffected == 0 {
		return workspace.ErrNotFound
	}
	return nil
}

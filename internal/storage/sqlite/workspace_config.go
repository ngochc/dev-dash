package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ngochc/dev-dash/internal/workspace"
)

const upsertWorkspaceConfig = `
	INSERT INTO workspace_config (
		workspace_id, namespace, key, value, created_at, updated_at
	)
	VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	ON CONFLICT(workspace_id, namespace, key) DO UPDATE SET
		value = excluded.value,
		updated_at = CURRENT_TIMESTAMP
`

// WorkspaceConfigRepository persists workspace configuration in SQLite.
type WorkspaceConfigRepository struct {
	db *sql.DB
}

func NewWorkspaceConfigRepository(db *sql.DB) *WorkspaceConfigRepository {
	return &WorkspaceConfigRepository{db: db}
}

func (r *WorkspaceConfigRepository) Set(ctx context.Context, workspaceID string, entry workspace.ConfigEntry) error {
	if _, err := r.db.ExecContext(ctx, upsertWorkspaceConfig, workspaceID, entry.Namespace, entry.Key, entry.Value); err != nil {
		return fmt.Errorf("upsert workspace config %q: %w", entry.FullKey(), err)
	}
	return nil
}

func (r *WorkspaceConfigRepository) Get(ctx context.Context, workspaceID, namespace, key string) (workspace.ConfigEntry, error) {
	var entry workspace.ConfigEntry
	if err := r.db.QueryRowContext(ctx, `
		SELECT namespace, key, value
		FROM workspace_config
		WHERE workspace_id = ? AND namespace = ? AND key = ?
	`, workspaceID, namespace, key).Scan(&entry.Namespace, &entry.Key, &entry.Value); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return workspace.ConfigEntry{}, workspace.ErrConfigNotFound
		}
		return workspace.ConfigEntry{}, fmt.Errorf("query workspace config %q: %w", namespace+"."+key, err)
	}
	return entry, nil
}

func (r *WorkspaceConfigRepository) List(ctx context.Context, workspaceID string) ([]workspace.ConfigEntry, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT namespace, key, value
		FROM workspace_config
		WHERE workspace_id = ?
		ORDER BY namespace, key
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("query workspace config: %w", err)
	}
	defer rows.Close()

	entries, err := scanWorkspaceConfigRows(rows)
	if err != nil {
		return nil, err
	}
	return entries, nil
}

func (r *WorkspaceConfigRepository) ListNamespace(ctx context.Context, workspaceID, namespace string) ([]workspace.ConfigEntry, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT namespace, key, value
		FROM workspace_config
		WHERE workspace_id = ? AND namespace = ?
		ORDER BY key
	`, workspaceID, namespace)
	if err != nil {
		return nil, fmt.Errorf("query workspace config namespace %q: %w", namespace, err)
	}
	defer rows.Close()

	entries, err := scanWorkspaceConfigRows(rows)
	if err != nil {
		return nil, err
	}
	return entries, nil
}

func (r *WorkspaceConfigRepository) Unset(ctx context.Context, workspaceID, namespace, key string) error {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM workspace_config
		WHERE workspace_id = ? AND namespace = ? AND key = ?
	`, workspaceID, namespace, key)
	if err != nil {
		return fmt.Errorf("delete workspace config %q: %w", namespace+"."+key, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted workspace config count: %w", err)
	}
	if rowsAffected == 0 {
		return workspace.ErrConfigNotFound
	}
	return nil
}

func (r *WorkspaceConfigRepository) ReplaceAll(ctx context.Context, workspaceID string, entries []workspace.ConfigEntry) error {
	return r.replace(ctx, workspaceID, entries, false)
}

func (r *WorkspaceConfigRepository) ReplaceUser(ctx context.Context, workspaceID string, entries []workspace.ConfigEntry) error {
	return r.replace(ctx, workspaceID, entries, true)
}

func (r *WorkspaceConfigRepository) replace(ctx context.Context, workspaceID string, entries []workspace.ConfigEntry, userOnly bool) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin workspace config replacement: %w", err)
	}
	defer tx.Rollback()

	existing, err := listWorkspaceConfigKeys(ctx, tx, workspaceID, userOnly)
	if err != nil {
		return err
	}
	desired := make(map[workspaceConfigKey]struct{}, len(entries))
	for _, entry := range entries {
		if _, err := tx.ExecContext(ctx, upsertWorkspaceConfig, workspaceID, entry.Namespace, entry.Key, entry.Value); err != nil {
			return fmt.Errorf("upsert workspace config %q: %w", entry.FullKey(), err)
		}
		desired[workspaceConfigKey{namespace: entry.Namespace, key: entry.Key}] = struct{}{}
	}
	for key := range existing {
		if _, keep := desired[key]; keep {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM workspace_config
			WHERE workspace_id = ? AND namespace = ? AND key = ?
		`, workspaceID, key.namespace, key.key); err != nil {
			return fmt.Errorf("delete stale workspace config %q: %w", key.namespace+"."+key.key, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit workspace config replacement: %w", err)
	}
	return nil
}

func scanWorkspaceConfigRows(rows *sql.Rows) ([]workspace.ConfigEntry, error) {
	var entries []workspace.ConfigEntry
	for rows.Next() {
		var entry workspace.ConfigEntry
		if err := rows.Scan(&entry.Namespace, &entry.Key, &entry.Value); err != nil {
			return nil, fmt.Errorf("scan workspace config: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspace config: %w", err)
	}
	return entries, nil
}

type workspaceConfigKey struct {
	namespace string
	key       string
}

func listWorkspaceConfigKeys(ctx context.Context, tx *sql.Tx, workspaceID string, userOnly bool) (map[workspaceConfigKey]struct{}, error) {
	query := `
		SELECT namespace, key
		FROM workspace_config
		WHERE workspace_id = ?
	`
	if userOnly {
		query += ` AND substr(namespace, 1, 1) <> '_'`
	}
	rows, err := tx.QueryContext(ctx, query, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("query existing workspace config: %w", err)
	}

	keys := make(map[workspaceConfigKey]struct{})
	for rows.Next() {
		var key workspaceConfigKey
		if err := rows.Scan(&key.namespace, &key.key); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan existing workspace config: %w", err)
		}
		keys[key] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate existing workspace config: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close existing workspace config rows: %w", err)
	}
	return keys, nil
}

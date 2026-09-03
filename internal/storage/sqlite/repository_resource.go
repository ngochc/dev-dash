package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/ngochc/dev-dash/internal/repository"
)

type RepositoryResourceStore struct {
	db *sql.DB
}

func NewRepositoryResourceStore(db *sql.DB) *RepositoryResourceStore {
	return &RepositoryResourceStore{db: db}
}

func (s *RepositoryResourceStore) UpsertDiscovered(ctx context.Context, source repository.Source, workspaceID string, remotes []repository.Remote) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin repository refresh: %w", err)
	}
	defer tx.Rollback()

	var integrationID string
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO integrations (id, provider, name, base_url)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(provider, name) DO UPDATE SET
			base_url = excluded.base_url,
			updated_at = CURRENT_TIMESTAMP
		RETURNING id
	`, uuid.NewString(), source.Provider, source.Name, source.BaseURL).Scan(&integrationID); err != nil {
		return fmt.Errorf("upsert repository integration: %w", err)
	}

	for _, remote := range remotes {
		resourceID, err := findRepositoryResource(ctx, tx, integrationID, remote)
		if err != nil {
			return err
		}
		if resourceID == "" {
			resourceID = uuid.NewString()
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO resources (
					id, type, integration_id, provider_id, external_key, name, url, last_seen_at
				)
				VALUES (?, 'repository', ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
			`, resourceID, integrationID, remote.ProviderID, remote.ExternalKey, remote.Name, remote.URL); err != nil {
				return fmt.Errorf("insert repository resource %q: %w", remote.ExternalKey, err)
			}
		} else if _, err := tx.ExecContext(ctx, `
			UPDATE resources
			SET provider_id = ?, external_key = ?, name = ?, url = ?,
				updated_at = CURRENT_TIMESTAMP, last_seen_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, remote.ProviderID, remote.ExternalKey, remote.Name, remote.URL, resourceID); err != nil {
			return fmt.Errorf("update repository resource %q: %w", remote.ExternalKey, err)
		}

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO workspace_resources (workspace_id, resource_id)
			VALUES (?, ?)
			ON CONFLICT(workspace_id, resource_id) DO NOTHING
		`, workspaceID, resourceID); err != nil {
			return fmt.Errorf("associate repository %q with workspace: %w", remote.ExternalKey, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit repository refresh: %w", err)
	}
	return nil
}

func (s *RepositoryResourceStore) ListByWorkspace(ctx context.Context, workspaceID string) ([]repository.Repository, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.id, COALESCE(r.provider_id, ''), COALESCE(r.external_key, ''),
		       COALESCE(r.name, ''), COALESCE(r.url, ''),
		       COALESCE((
			   SELECT rl.path
			   FROM resource_locations AS rl
			   WHERE rl.resource_id = r.id
			     AND rl.workspace_id = wr.workspace_id
			     AND rl.location_type = 'local_checkout'
			   ORDER BY rl.updated_at DESC, rl.id
			   LIMIT 1
		       ), '')
		FROM workspace_resources AS wr
		JOIN resources AS r ON r.id = wr.resource_id
		WHERE wr.workspace_id = ? AND r.type = 'repository'
		ORDER BY r.external_key, r.id
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("query workspace repositories: %w", err)
	}
	defer rows.Close()

	var items []repository.Repository
	for rows.Next() {
		var item repository.Repository
		if err := rows.Scan(
			&item.ResourceID,
			&item.ProviderID,
			&item.ExternalKey,
			&item.Name,
			&item.URL,
			&item.CheckoutPath,
		); err != nil {
			return nil, fmt.Errorf("scan workspace repository: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspace repositories: %w", err)
	}
	return items, nil
}

func (s *RepositoryResourceStore) SetCheckout(ctx context.Context, workspaceID, resourceID, path string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin checkout registration: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM resource_locations
		WHERE resource_id = ? AND workspace_id = ? AND location_type = 'local_checkout'
	`, resourceID, workspaceID); err != nil {
		return fmt.Errorf("remove previous checkout location: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO resource_locations (
			id, resource_id, workspace_id, location_type, path
		)
		VALUES (?, ?, ?, 'local_checkout', ?)
	`, uuid.NewString(), resourceID, workspaceID, path); err != nil {
		return fmt.Errorf("register checkout location %q: %w", path, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit checkout registration: %w", err)
	}
	return nil
}

func findRepositoryResource(ctx context.Context, tx *sql.Tx, integrationID string, remote repository.Remote) (string, error) {
	var resourceID string
	err := tx.QueryRowContext(ctx, `
		SELECT id
		FROM resources
		WHERE integration_id = ?
		  AND type = 'repository'
		  AND (provider_id = ? OR (provider_id IS NULL AND external_key = ?))
		ORDER BY CASE WHEN provider_id = ? THEN 0 ELSE 1 END
		LIMIT 1
	`, integrationID, remote.ProviderID, remote.ExternalKey, remote.ProviderID).Scan(&resourceID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("query repository resource %q: %w", remote.ExternalKey, err)
	}
	return resourceID, nil
}

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/ngochc/dev-dash/internal/resource"
)

// ResourceSyncStore persists discovered resources and their workspace locations.
type ResourceSyncStore struct {
	db *sql.DB
}

func NewResourceSyncStore(db *sql.DB) *ResourceSyncStore {
	return &ResourceSyncStore{db: db}
}

func (s *ResourceSyncStore) UpsertDiscovered(ctx context.Context, source resource.Source, workspaceID, resourceType string, discovered []resource.Discovered) error {
	for _, item := range discovered {
		if strings.TrimSpace(item.ProviderID) == "" {
			return fmt.Errorf("%s resource %q provider ID is required", resourceType, item.ExternalKey)
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin %s: %w", refreshOperation(resourceType), err)
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
		return fmt.Errorf("upsert %s integration: %w", resourceNoun(resourceType), err)
	}

	for _, item := range discovered {
		resourceID, err := findDiscoveredResource(ctx, tx, integrationID, resourceType, item)
		if err != nil {
			return err
		}
		if resourceID == "" {
			resourceID = uuid.NewString()
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO resources (
					id, type, integration_id, provider_id, external_key, name, url, metadata, last_seen_at
				)
				VALUES (?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), CURRENT_TIMESTAMP)
			`, resourceID, resourceType, integrationID, item.ProviderID, item.ExternalKey, item.Name, item.URL, item.Metadata); err != nil {
				return fmt.Errorf("insert %s resource %q: %w", resourceNoun(resourceType), item.ExternalKey, err)
			}
		} else if _, err := tx.ExecContext(ctx, `
			UPDATE resources
			SET provider_id = ?, external_key = ?, name = ?, url = ?, metadata = NULLIF(?, ''),
				updated_at = CURRENT_TIMESTAMP, last_seen_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, item.ProviderID, item.ExternalKey, item.Name, item.URL, item.Metadata, resourceID); err != nil {
			return fmt.Errorf("update %s resource %q: %w", resourceNoun(resourceType), item.ExternalKey, err)
		}

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO workspace_resources (workspace_id, resource_id)
			VALUES (?, ?)
			ON CONFLICT(workspace_id, resource_id) DO NOTHING
		`, workspaceID, resourceID); err != nil {
			return fmt.Errorf("associate %s %q with workspace: %w", resourceNoun(resourceType), item.ExternalKey, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit %s: %w", refreshOperation(resourceType), err)
	}
	return nil
}

func (s *ResourceSyncStore) ListByWorkspace(ctx context.Context, workspaceID, resourceType, locationType string) ([]resource.Located, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.id, r.type, COALESCE(r.integration_id, ''), COALESCE(r.provider_id, ''),
		       COALESCE(r.external_key, ''), COALESCE(r.name, ''), COALESCE(r.url, ''),
		       COALESCE(r.metadata, ''), r.created_at, r.updated_at, r.last_seen_at,
		       COALESCE((
			   SELECT rl.path
			   FROM resource_locations AS rl
			   WHERE rl.resource_id = r.id
			     AND rl.workspace_id = wr.workspace_id
			     AND rl.location_type = ?
			   ORDER BY rl.updated_at DESC, rl.id
			   LIMIT 1
		       ), '')
		FROM workspace_resources AS wr
		JOIN resources AS r ON r.id = wr.resource_id
		WHERE wr.workspace_id = ? AND r.type = ?
		ORDER BY r.external_key, r.id
	`, locationType, workspaceID, resourceType)
	if err != nil {
		return nil, fmt.Errorf("query workspace %s: %w", resourcePlural(resourceType), err)
	}
	defer rows.Close()

	var items []resource.Located
	for rows.Next() {
		row := newResourceRow()
		var locationPath string
		destinations := append(row.destinations(), &locationPath)
		if err := rows.Scan(destinations...); err != nil {
			return nil, fmt.Errorf("scan workspace %s: %w", resourceNoun(resourceType), err)
		}
		items = append(items, resource.Located{Resource: row.value(), LocationPath: locationPath})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspace %s: %w", resourcePlural(resourceType), err)
	}
	return items, nil
}

func (s *ResourceSyncStore) SetLocation(ctx context.Context, workspaceID, resourceID, locationType, path string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin %s registration: %w", locationNoun(locationType), err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM resource_locations
		WHERE resource_id = ? AND workspace_id = ? AND location_type = ?
	`, resourceID, workspaceID, locationType); err != nil {
		return fmt.Errorf("remove previous %s location: %w", locationNoun(locationType), err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO resource_locations (
			id, resource_id, workspace_id, location_type, path
		)
		VALUES (?, ?, ?, ?, ?)
	`, uuid.NewString(), resourceID, workspaceID, locationType, path); err != nil {
		return fmt.Errorf("register %s location %q: %w", locationNoun(locationType), path, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit %s registration: %w", locationNoun(locationType), err)
	}
	return nil
}

func findDiscoveredResource(ctx context.Context, tx *sql.Tx, integrationID, resourceType string, item resource.Discovered) (string, error) {
	var resourceID string
	err := tx.QueryRowContext(ctx, `
		SELECT id
		FROM resources
		WHERE integration_id = ?
		  AND type = ?
		  AND (provider_id = ? OR (provider_id IS NULL AND external_key = ?))
		ORDER BY CASE WHEN provider_id = ? THEN 0 ELSE 1 END
		LIMIT 1
	`, integrationID, resourceType, item.ProviderID, item.ExternalKey, item.ProviderID).Scan(&resourceID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("query %s resource %q: %w", resourceNoun(resourceType), item.ExternalKey, err)
	}
	return resourceID, nil
}

func resourceNoun(resourceType string) string {
	if resourceType == "repository" {
		return "repository"
	}
	return "resource"
}

func resourcePlural(resourceType string) string {
	if resourceType == "repository" {
		return "repositories"
	}
	return "resources"
}

func refreshOperation(resourceType string) string {
	if resourceType == "repository" {
		return "repository refresh"
	}
	return "resource refresh"
}

func locationNoun(locationType string) string {
	if locationType == "local_checkout" {
		return "checkout"
	}
	return "resource location"
}

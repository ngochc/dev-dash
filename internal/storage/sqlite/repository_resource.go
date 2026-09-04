package sqlite

import (
	"context"
	"database/sql"

	"github.com/ngochc/dev-dash/internal/repository"
	"github.com/ngochc/dev-dash/internal/resource"
)

const (
	repositoryResourceType = "repository"
	checkoutLocationType   = "local_checkout"
)

// RepositoryResourceStore adapts the generic resource sync store to repositories.
type RepositoryResourceStore struct {
	store *ResourceSyncStore
}

func NewRepositoryResourceStore(db *sql.DB) *RepositoryResourceStore {
	return &RepositoryResourceStore{store: NewResourceSyncStore(db)}
}

func (s *RepositoryResourceStore) UpsertDiscovered(ctx context.Context, source repository.Source, workspaceID string, remotes []repository.Remote) error {
	discovered := make([]resource.Discovered, len(remotes))
	for i, remote := range remotes {
		discovered[i] = resource.Discovered{
			ProviderID:  remote.ProviderID,
			ExternalKey: remote.ExternalKey,
			Name:        remote.Name,
			URL:         remote.URL,
		}
	}
	return s.store.UpsertDiscovered(ctx, resource.Source{
		Provider: source.Provider,
		Name:     source.Name,
		BaseURL:  source.BaseURL,
	}, workspaceID, repositoryResourceType, discovered)
}

func (s *RepositoryResourceStore) ListByWorkspace(ctx context.Context, workspaceID string) ([]repository.Repository, error) {
	located, err := s.store.ListByWorkspace(ctx, workspaceID, repositoryResourceType, checkoutLocationType)
	if err != nil {
		return nil, err
	}
	items := make([]repository.Repository, len(located))
	for i, item := range located {
		items[i] = repository.Repository{
			ResourceID:   item.Resource.ID,
			ProviderID:   item.Resource.ProviderID,
			ExternalKey:  item.Resource.ExternalKey,
			Name:         item.Resource.Name,
			URL:          item.Resource.URL,
			CheckoutPath: item.LocationPath,
		}
	}
	return items, nil
}

func (s *RepositoryResourceStore) SetCheckout(ctx context.Context, workspaceID, resourceID, path string) error {
	return s.store.SetLocation(ctx, workspaceID, resourceID, checkoutLocationType, path)
}

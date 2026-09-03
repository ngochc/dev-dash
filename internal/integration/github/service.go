package github

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	repositorydomain "github.com/ngochc/dev-dash/internal/repository"
	"github.com/ngochc/dev-dash/internal/workspace"
)

var ErrCloneFailed = errors.New("repository clone failed")

type RepositoryClient interface {
	Validate(context.Context, Config) error
	Discover(context.Context, Config) ([]Repository, error)
	Clone(context.Context, Config, string, string) error
}

type Service struct {
	workspaceService *workspace.Service
	configService    *workspace.ConfigService
	store            repositorydomain.Store
	client           RepositoryClient
	inspector        repositorydomain.CheckoutInspector
	directories      repositorydomain.DirectoryManager
}

func NewService(
	workspaceRepository workspace.Repository,
	configRepository workspace.ConfigRepository,
	store repositorydomain.Store,
	client RepositoryClient,
	inspector repositorydomain.CheckoutInspector,
	directories repositorydomain.DirectoryManager,
) *Service {
	return &Service{
		workspaceService: workspace.NewService(workspaceRepository),
		configService:    workspace.NewConfigService(workspaceRepository, configRepository),
		store:            store,
		client:           client,
		inspector:        inspector,
		directories:      directories,
	}
}

func (s *Service) Check(ctx context.Context, workspaceIdentifier string) (workspace.Workspace, Config, error) {
	item, config, err := s.prepare(ctx, workspaceIdentifier)
	if err != nil {
		return workspace.Workspace{}, Config{}, err
	}
	if err := s.client.Validate(ctx, config); err != nil {
		return workspace.Workspace{}, Config{}, err
	}
	return item, config, nil
}

func (s *Service) Refresh(ctx context.Context, workspaceIdentifier string) (workspace.Workspace, int, error) {
	item, _, count, err := s.refresh(ctx, workspaceIdentifier)
	return item, count, err
}

func (s *Service) refresh(ctx context.Context, workspaceIdentifier string) (workspace.Workspace, Config, int, error) {
	item, config, err := s.Check(ctx, workspaceIdentifier)
	if err != nil {
		return workspace.Workspace{}, Config{}, 0, err
	}
	remoteRepositories, err := s.client.Discover(ctx, config)
	if err != nil {
		return workspace.Workspace{}, Config{}, 0, err
	}
	remotes := make([]repositorydomain.Remote, len(remoteRepositories))
	for i, remote := range remoteRepositories {
		remotes[i] = repositorydomain.Remote{
			ProviderID:  remote.ID,
			ExternalKey: remote.NameWithOwner,
			Name:        remote.Name,
			URL:         remote.URL,
		}
	}
	source := repositorydomain.Source{Provider: "github", Name: config.Host, BaseURL: config.BaseURL}
	if err := s.store.UpsertDiscovered(ctx, source, item.ID, remotes); err != nil {
		return workspace.Workspace{}, Config{}, 0, fmt.Errorf("store discovered repositories: %w", err)
	}
	return item, config, len(remotes), nil
}

func (s *Service) List(ctx context.Context, workspaceIdentifier string) (workspace.Workspace, []repositorydomain.Listed, error) {
	item, err := s.workspaceService.Get(ctx, workspaceIdentifier)
	if err != nil {
		return workspace.Workspace{}, nil, err
	}
	items, err := s.store.ListByWorkspace(ctx, item.ID)
	if err != nil {
		return workspace.Workspace{}, nil, fmt.Errorf("list workspace repositories: %w", err)
	}
	listed, err := repositorydomain.List(ctx, items, s.inspector)
	if err != nil {
		return workspace.Workspace{}, nil, err
	}
	return item, listed, nil
}

func (s *Service) Clone(ctx context.Context, workspaceIdentifier string, selectors []string, all bool) (workspace.Workspace, []repositorydomain.CloneResult, error) {
	item, config, _, err := s.refresh(ctx, workspaceIdentifier)
	if err != nil {
		return workspace.Workspace{}, nil, err
	}
	return s.cloneKnown(ctx, item, config, selectors, all)
}

func (s *Service) CloneKnown(ctx context.Context, workspaceIdentifier string, selectors []string) (workspace.Workspace, []repositorydomain.CloneResult, error) {
	item, config, err := s.Check(ctx, workspaceIdentifier)
	if err != nil {
		return workspace.Workspace{}, nil, err
	}
	return s.cloneKnown(ctx, item, config, selectors, false)
}

func (s *Service) cloneKnown(ctx context.Context, item workspace.Workspace, config Config, selectors []string, all bool) (workspace.Workspace, []repositorydomain.CloneResult, error) {
	items, err := s.store.ListByWorkspace(ctx, item.ID)
	if err != nil {
		return workspace.Workspace{}, nil, fmt.Errorf("list workspace repositories: %w", err)
	}
	selected, err := repositorydomain.Select(items, selectors, all)
	if err != nil {
		return workspace.Workspace{}, nil, err
	}

	results := make([]repositorydomain.CloneResult, 0, len(selected))
	failures := 0
	for _, repositoryItem := range selected {
		result := s.cloneOne(ctx, item, config, repositoryItem)
		if result.Error != nil {
			failures++
		}
		results = append(results, result)
	}
	if failures > 0 {
		return item, results, cloneFailureError{count: failures}
	}
	return item, results, nil
}

func (s *Service) prepare(ctx context.Context, workspaceIdentifier string) (workspace.Workspace, Config, error) {
	item, err := s.workspaceService.Get(ctx, workspaceIdentifier)
	if err != nil {
		return workspace.Workspace{}, Config{}, err
	}
	values, err := s.configService.Namespace(ctx, item.ID, "github")
	if err != nil {
		return workspace.Workspace{}, Config{}, err
	}
	config, err := ResolveConfig(item.Name, values)
	if err != nil {
		return workspace.Workspace{}, Config{}, err
	}
	return item, config, nil
}

func (s *Service) cloneOne(ctx context.Context, workspaceItem workspace.Workspace, config Config, item repositorydomain.Repository) repositorydomain.CloneResult {
	result := repositorydomain.CloneResult{Repository: item.ExternalKey}
	state, err := repositorydomain.DeriveState(ctx, item, s.inspector)
	if err != nil {
		result.Error = err
		return result
	}
	switch state {
	case repositorydomain.StateCloned:
		result.Status = "already cloned"
		return result
	case repositorydomain.StateInvalid:
		result.Error = errors.New("destination exists but is not the expected repository")
		return result
	}

	destination := item.CheckoutPath
	status := "restored"
	if state == repositorydomain.StateNotCloned {
		destination = filepath.Join(workspaceItem.LocalPath, "repos", item.Name)
		status = "cloned"
		inspection, err := s.inspector.Inspect(ctx, destination, item.URL)
		if err != nil {
			result.Error = err
			return result
		}
		if inspection.Exists {
			if !inspection.Valid {
				result.Error = errors.New("destination exists but is not the expected repository")
				return result
			}
			if err := s.store.SetCheckout(ctx, workspaceItem.ID, item.ResourceID, destination); err != nil {
				result.Error = fmt.Errorf("register existing checkout: %w", err)
				return result
			}
			result.Status = "already cloned"
			return result
		}
	}

	if err := s.directories.Ensure(filepath.Dir(destination)); err != nil {
		result.Error = err
		return result
	}
	if err := s.client.Clone(ctx, config, item.ExternalKey, destination); err != nil {
		result.Error = err
		if inspection, inspectErr := s.inspector.Inspect(ctx, destination, item.URL); inspectErr == nil && inspection.Exists {
			result.Error = fmt.Errorf("%w; partial destination remains at %s", err, destination)
		}
		return result
	}
	inspection, err := s.inspector.Inspect(ctx, destination, item.URL)
	if err != nil {
		result.Error = err
		return result
	}
	if !inspection.Exists || !inspection.Valid {
		result.Error = errors.New("clone completed but destination is not the expected repository")
		return result
	}
	if err := s.store.SetCheckout(ctx, workspaceItem.ID, item.ResourceID, destination); err != nil {
		result.Error = fmt.Errorf("register checkout: %w", err)
		return result
	}
	result.Status = status
	return result
}

type cloneFailureError struct {
	count int
}

func (e cloneFailureError) Error() string {
	return fmt.Sprintf("repository clone failed for %d repository(s)", e.count)
}

func (e cloneFailureError) Unwrap() error { return ErrCloneFailed }

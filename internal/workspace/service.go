package workspace

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/ngochc/dev-dash/internal/platform"
)

// Service provides workspace operations.
type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) Add(ctx context.Context, name, path string) (Workspace, error) {
	if strings.TrimSpace(name) == "" {
		return Workspace{}, errors.New("workspace name is required")
	}

	localPath, err := platform.ResolveWorkspaceDirectory(name, path)
	if err != nil {
		return Workspace{}, fmt.Errorf("resolve workspace path: %w", err)
	}

	item := Workspace{
		ID:        uuid.NewString(),
		Name:      name,
		LocalPath: localPath,
	}
	if err := s.repository.Create(ctx, item); err != nil {
		if errors.Is(err, ErrNameExists) {
			return Workspace{}, fmt.Errorf("%w: %q", ErrNameExists, name)
		}
		return Workspace{}, fmt.Errorf("create workspace: %w", err)
	}

	return item, nil
}

func (s *Service) List(ctx context.Context) ([]Workspace, error) {
	items, err := s.repository.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	return items, nil
}

func (s *Service) Get(ctx context.Context, identifier string) (Workspace, error) {
	item, err := s.repository.GetByID(ctx, identifier)
	if err == nil {
		return item, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Workspace{}, fmt.Errorf("get workspace by ID: %w", err)
	}

	item, err = s.repository.GetByName(ctx, identifier)
	if err == nil {
		return item, nil
	}
	if errors.Is(err, ErrNotFound) {
		return Workspace{}, fmt.Errorf("%w: %q", ErrNotFound, identifier)
	}
	return Workspace{}, fmt.Errorf("get workspace by name: %w", err)
}

func (s *Service) Remove(ctx context.Context, identifier string) (Workspace, error) {
	item, err := s.Get(ctx, identifier)
	if err != nil {
		return Workspace{}, err
	}

	if err := s.repository.Delete(ctx, item.ID); err != nil {
		return Workspace{}, fmt.Errorf("delete workspace: %w", err)
	}
	return item, nil
}

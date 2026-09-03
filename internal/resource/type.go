package resource

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var (
	ErrTypeNotFound = errors.New("resource type not found")
	ErrTypeExists   = errors.New("resource type already exists")
)

type ResourceType struct {
	Name        string
	DisplayName string
	Owner       string
	Description string
	CreatedAt   time.Time
}

type TypeRepository interface {
	Create(context.Context, ResourceType) error
	List(context.Context) ([]ResourceType, error)
	Get(context.Context, string) (ResourceType, error)
}

type TypeService struct {
	repository TypeRepository
}

func NewTypeService(repository TypeRepository) *TypeService {
	return &TypeService{repository: repository}
}

func (s *TypeService) Register(ctx context.Context, name, displayName, owner, description string) (ResourceType, error) {
	if strings.TrimSpace(name) == "" {
		return ResourceType{}, errors.New("resource type name is required")
	}
	if strings.TrimSpace(displayName) == "" {
		return ResourceType{}, errors.New("resource type display name is required")
	}

	item := ResourceType{
		Name:        name,
		DisplayName: displayName,
		Owner:       owner,
		Description: description,
	}
	if err := s.repository.Create(ctx, item); err != nil {
		if errors.Is(err, ErrTypeExists) {
			return ResourceType{}, typeContextError{message: fmt.Sprintf("resource type %q already exists", name), cause: ErrTypeExists}
		}
		return ResourceType{}, fmt.Errorf("create resource type: %w", err)
	}

	return s.Get(ctx, name)
}

func (s *TypeService) List(ctx context.Context) ([]ResourceType, error) {
	items, err := s.repository.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list resource types: %w", err)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	return items, nil
}

func (s *TypeService) Get(ctx context.Context, name string) (ResourceType, error) {
	item, err := s.repository.Get(ctx, name)
	if err == nil {
		return item, nil
	}
	if errors.Is(err, ErrTypeNotFound) {
		return ResourceType{}, typeContextError{message: fmt.Sprintf("resource type %q not found", name), cause: ErrTypeNotFound}
	}
	return ResourceType{}, fmt.Errorf("get resource type: %w", err)
}

type typeContextError struct {
	message string
	cause   error
}

func (e typeContextError) Error() string { return e.message }
func (e typeContextError) Unwrap() error { return e.cause }

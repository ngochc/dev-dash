package resource

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrNotFound = errors.New("resource not found")

type Resource struct {
	ID            string
	Type          string
	IntegrationID string
	ProviderID    string
	ExternalKey   string
	Name          string
	URL           string
	Metadata      string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	LastSeenAt    *time.Time
}

type Repository interface {
	Create(context.Context, Resource) error
	List(context.Context) ([]Resource, error)
	Get(context.Context, string) (Resource, error)
	Update(context.Context, Resource) error
	Delete(context.Context, string) error
}

type Service struct {
	repository     Repository
	typeRepository TypeRepository
}

func NewService(repository Repository, typeRepository TypeRepository) *Service {
	return &Service{repository: repository, typeRepository: typeRepository}
}

func (s *Service) Add(ctx context.Context, resourceType, name, url string) (Resource, error) {
	if err := validateFields("", resourceType, name, false); err != nil {
		return Resource{}, err
	}
	if err := s.requireType(ctx, resourceType); err != nil {
		return Resource{}, err
	}

	item := Resource{ID: uuid.NewString(), Type: resourceType, Name: name, URL: url}
	if err := s.repository.Create(ctx, item); err != nil {
		return Resource{}, fmt.Errorf("create resource: %w", err)
	}
	return s.Get(ctx, item.ID)
}

func (s *Service) List(ctx context.Context) ([]Resource, error) {
	items, err := s.repository.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list resources: %w", err)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Name == items[j].Name {
			return items[i].ID < items[j].ID
		}
		return items[i].Name < items[j].Name
	})
	return items, nil
}

func (s *Service) Get(ctx context.Context, id string) (Resource, error) {
	if strings.TrimSpace(id) == "" {
		return Resource{}, errors.New("resource ID is required")
	}
	item, err := s.repository.Get(ctx, id)
	if err == nil {
		return item, nil
	}
	if errors.Is(err, ErrNotFound) {
		return Resource{}, resourceContextError{message: fmt.Sprintf("resource %q not found", id), cause: ErrNotFound}
	}
	return Resource{}, fmt.Errorf("get resource: %w", err)
}

func (s *Service) Update(ctx context.Context, id, resourceType, name, url string) (Resource, error) {
	if err := validateFields(id, resourceType, name, true); err != nil {
		return Resource{}, err
	}
	if err := s.requireType(ctx, resourceType); err != nil {
		return Resource{}, err
	}
	item, err := s.Get(ctx, id)
	if err != nil {
		return Resource{}, err
	}
	item.Type = resourceType
	item.Name = name
	item.URL = url
	if err := s.repository.Update(ctx, item); err != nil {
		if errors.Is(err, ErrNotFound) {
			return Resource{}, resourceContextError{message: fmt.Sprintf("resource %q not found", id), cause: ErrNotFound}
		}
		return Resource{}, fmt.Errorf("update resource: %w", err)
	}
	return s.Get(ctx, id)
}

func (s *Service) Remove(ctx context.Context, id string) (Resource, error) {
	item, err := s.Get(ctx, id)
	if err != nil {
		return Resource{}, err
	}
	if err := s.repository.Delete(ctx, id); err != nil {
		if errors.Is(err, ErrNotFound) {
			return Resource{}, resourceContextError{message: fmt.Sprintf("resource %q not found", id), cause: ErrNotFound}
		}
		return Resource{}, fmt.Errorf("delete resource: %w", err)
	}
	return item, nil
}

func (s *Service) requireType(ctx context.Context, name string) error {
	_, err := s.typeRepository.Get(ctx, name)
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrTypeNotFound) {
		return typeContextError{message: fmt.Sprintf("resource type %q not found", name), cause: ErrTypeNotFound}
	}
	return fmt.Errorf("get resource type: %w", err)
}

func validateFields(id, resourceType, name string, requireID bool) error {
	if requireID && strings.TrimSpace(id) == "" {
		return errors.New("resource ID is required")
	}
	if strings.TrimSpace(resourceType) == "" {
		return errors.New("resource type is required")
	}
	if strings.TrimSpace(name) == "" {
		return errors.New("resource name is required")
	}
	return nil
}

type resourceContextError struct {
	message string
	cause   error
}

func (e resourceContextError) Error() string { return e.message }
func (e resourceContextError) Unwrap() error { return e.cause }

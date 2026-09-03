package graph

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var (
	ErrTypeNotFound = errors.New("relation type not found")
	ErrTypeExists   = errors.New("relation type already exists")
)

type RelationType struct {
	Name        string
	DisplayName string
	InverseName string
	Symmetric   bool
	Owner       string
	Description string
	CreatedAt   time.Time
}

type TypeRepository interface {
	Create(context.Context, RelationType) error
	List(context.Context) ([]RelationType, error)
	Get(context.Context, string) (RelationType, error)
}

type TypeService struct {
	repository TypeRepository
}

func NewTypeService(repository TypeRepository) *TypeService {
	return &TypeService{repository: repository}
}

func (s *TypeService) Register(ctx context.Context, name, displayName, inverseName string, symmetric bool, owner, description string) (RelationType, error) {
	if strings.TrimSpace(name) == "" {
		return RelationType{}, errors.New("relation type name is required")
	}
	if strings.TrimSpace(displayName) == "" {
		return RelationType{}, errors.New("relation type display name is required")
	}
	if symmetric {
		if inverseName == "" {
			inverseName = name
		} else if inverseName != name {
			return RelationType{}, errors.New("symmetric relation type inverse must match its name")
		}
	}

	item := RelationType{
		Name:        name,
		DisplayName: displayName,
		InverseName: inverseName,
		Symmetric:   symmetric,
		Owner:       owner,
		Description: description,
	}
	if err := s.repository.Create(ctx, item); err != nil {
		if errors.Is(err, ErrTypeExists) {
			return RelationType{}, typeContextError{message: fmt.Sprintf("relation type %q already exists", name), cause: ErrTypeExists}
		}
		return RelationType{}, fmt.Errorf("create relation type: %w", err)
	}

	return s.Get(ctx, name)
}

func (s *TypeService) List(ctx context.Context) ([]RelationType, error) {
	items, err := s.repository.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list relation types: %w", err)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	return items, nil
}

func (s *TypeService) Get(ctx context.Context, name string) (RelationType, error) {
	item, err := s.repository.Get(ctx, name)
	if err == nil {
		return item, nil
	}
	if errors.Is(err, ErrTypeNotFound) {
		return RelationType{}, typeContextError{message: fmt.Sprintf("relation type %q not found", name), cause: ErrTypeNotFound}
	}
	return RelationType{}, fmt.Errorf("get relation type: %w", err)
}

type typeContextError struct {
	message string
	cause   error
}

func (e typeContextError) Error() string { return e.message }
func (e typeContextError) Unwrap() error { return e.cause }

package secret

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
)

var (
	ErrInvalidKey = errors.New("invalid secret key")
	keyPattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

// Service provides secret operations.
type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func ValidateKey(key string) error {
	if !keyPattern.MatchString(key) {
		return fmt.Errorf("%w: %q", ErrInvalidKey, key)
	}
	return nil
}

func (s *Service) Set(ctx context.Context, key, value string) error {
	if err := ValidateKey(key); err != nil {
		return err
	}
	if len(value) == 0 {
		return errors.New("secret value is required")
	}
	if err := s.repository.Set(ctx, key, value); err != nil {
		return fmt.Errorf("set secret %q: %w", key, err)
	}
	return nil
}

func (s *Service) Get(ctx context.Context, key string) (Secret, error) {
	if err := ValidateKey(key); err != nil {
		return Secret{}, err
	}
	item, err := s.repository.Get(ctx, key)
	if err == nil {
		return item, nil
	}
	if errors.Is(err, ErrNotFound) {
		return Secret{}, notFoundError{key: key}
	}
	return Secret{}, fmt.Errorf("get secret %q: %w", key, err)
}

func (s *Service) List(ctx context.Context) ([]Secret, error) {
	items, err := s.repository.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list secrets: %w", err)
	}
	for i := range items {
		items[i].Value = ""
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Key < items[j].Key
	})
	return items, nil
}

func (s *Service) Delete(ctx context.Context, key string) error {
	if err := ValidateKey(key); err != nil {
		return err
	}
	if err := s.repository.Delete(ctx, key); err != nil {
		if errors.Is(err, ErrNotFound) {
			return notFoundError{key: key}
		}
		return fmt.Errorf("delete secret %q: %w", key, err)
	}
	return nil
}

type notFoundError struct {
	key string
}

func (e notFoundError) Error() string {
	return fmt.Sprintf("secret %q not found", e.key)
}

func (e notFoundError) Unwrap() error {
	return ErrNotFound
}

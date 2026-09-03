package secret

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestValidateKey(t *testing.T) {
	valid := []string{
		"a",
		"Z9",
		"github.pat",
		"API_KEY",
		"service-token",
		"9starts-with-digit",
	}
	for _, key := range valid {
		t.Run("valid "+key, func(t *testing.T) {
			if err := ValidateKey(key); err != nil {
				t.Errorf("ValidateKey(%q) error = %v", key, err)
			}
		})
	}

	invalid := []string{
		"",
		".leading-dot",
		"_leading-underscore",
		"-leading-hyphen",
		"contains space",
		"contains/slash",
		"éclair",
		"line\nbreak",
	}
	for _, key := range invalid {
		t.Run("invalid "+key, func(t *testing.T) {
			if err := ValidateKey(key); !errors.Is(err, ErrInvalidKey) {
				t.Errorf("ValidateKey(%q) error = %v, want ErrInvalidKey", key, err)
			}
		})
	}
}

func TestInvalidKeyOperationsDoNotCallRepository(t *testing.T) {
	invalid := []string{"", ".leading", "contains space", "contains/slash"}
	for _, key := range invalid {
		t.Run(key, func(t *testing.T) {
			repository := newFakeRepository()
			service := NewService(repository)

			if err := service.Set(context.Background(), key, "test-sensitive-value"); !errors.Is(err, ErrInvalidKey) {
				t.Errorf("Set() error = %v, want ErrInvalidKey", err)
			}
			if _, err := service.Get(context.Background(), key); !errors.Is(err, ErrInvalidKey) {
				t.Errorf("Get() error = %v, want ErrInvalidKey", err)
			}
			if err := service.Delete(context.Background(), key); !errors.Is(err, ErrInvalidKey) {
				t.Errorf("Delete() error = %v, want ErrInvalidKey", err)
			}
			if repository.calls != 0 {
				t.Errorf("repository calls = %d, want 0", repository.calls)
			}
		})
	}
}

func TestServiceSetAndReplace(t *testing.T) {
	repository := newFakeRepository()
	service := NewService(repository)
	ctx := context.Background()

	if err := service.Set(ctx, "github.pat", "initial-test-value"); err != nil {
		t.Fatalf("Set(initial) error = %v", err)
	}
	if err := service.Set(ctx, "github.pat", "replacement-test-value"); err != nil {
		t.Fatalf("Set(replacement) error = %v", err)
	}
	if len(repository.items) != 1 {
		t.Fatalf("stored item count = %d, want 1", len(repository.items))
	}
	if repository.items[0].Value != "replacement-test-value" {
		t.Error("Set() did not store the replacement value")
	}
}

func TestServiceSetRejectsEmptyValue(t *testing.T) {
	repository := newFakeRepository()
	err := NewService(repository).Set(context.Background(), "github.pat", "")
	if err == nil || err.Error() != "secret value is required" {
		t.Fatalf("Set() error = %v, want required value error", err)
	}
	if repository.calls != 0 {
		t.Errorf("repository calls = %d, want 0", repository.calls)
	}
}

func TestServiceGet(t *testing.T) {
	repository := newFakeRepository(Secret{Key: "github.pat", Value: "test-sensitive-value"})
	item, err := NewService(repository).Get(context.Background(), "github.pat")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if item.Key != "github.pat" {
		t.Errorf("Get() key = %q, want github.pat", item.Key)
	}
	if item.Value != "test-sensitive-value" {
		t.Error("Get() returned an unexpected value")
	}
}

func TestServiceGetMissing(t *testing.T) {
	_, err := NewService(newFakeRepository()).Get(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() error = %v, want ErrNotFound", err)
	}
	if err.Error() != `secret "missing" not found` {
		t.Errorf("Get() error = %q, want keyed not-found error", err)
	}
}

func TestServiceListEmpty(t *testing.T) {
	items, err := NewService(newFakeRepository()).List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("List() returned %d secrets, want 0", len(items))
	}
}

func TestServiceListSortsAndClearsValues(t *testing.T) {
	repository := newFakeRepository(
		Secret{Key: "zeta", Value: "zeta-test-value"},
		Secret{Key: "alpha", Value: "alpha-test-value"},
	)
	items, err := NewService(repository).List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	gotKeys := []string{items[0].Key, items[1].Key}
	if !reflect.DeepEqual(gotKeys, []string{"alpha", "zeta"}) {
		t.Errorf("List() keys = %v, want alphabetical order", gotKeys)
	}
	for _, item := range items {
		if item.Value != "" {
			t.Error("List() unexpectedly returned a secret value")
		}
	}
}

func TestServiceDelete(t *testing.T) {
	repository := newFakeRepository(Secret{Key: "github.pat", Value: "test-sensitive-value"})
	service := NewService(repository)
	if err := service.Delete(context.Background(), "github.pat"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if len(repository.items) != 0 {
		t.Errorf("Delete() left %d secrets, want 0", len(repository.items))
	}

	err := service.Delete(context.Background(), "github.pat")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("second Delete() error = %v, want ErrNotFound", err)
	}
	if err.Error() != `secret "github.pat" not found` {
		t.Errorf("second Delete() error = %q, want keyed not-found error", err)
	}
}

func TestServiceWrapsRepositoryErrors(t *testing.T) {
	repositoryError := errors.New("repository failed")
	tests := []struct {
		name string
		run  func(*Service) error
	}{
		{name: "set", run: func(service *Service) error {
			return service.Set(context.Background(), "github.pat", "test-sensitive-value")
		}},
		{name: "get", run: func(service *Service) error {
			_, err := service.Get(context.Background(), "github.pat")
			return err
		}},
		{name: "list", run: func(service *Service) error {
			_, err := service.List(context.Background())
			return err
		}},
		{name: "delete", run: func(service *Service) error {
			return service.Delete(context.Background(), "github.pat")
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := newFakeRepository()
			repository.setErr = repositoryError
			repository.getErr = repositoryError
			repository.listErr = repositoryError
			repository.deleteErr = repositoryError
			err := test.run(NewService(repository))
			if !errors.Is(err, repositoryError) {
				t.Fatalf("service error = %v, want repository error", err)
			}
			if strings.Contains(err.Error(), "test-sensitive-value") {
				t.Error("service error disclosed the attempted secret value")
			}
		})
	}
}

type fakeRepository struct {
	items     []Secret
	calls     int
	setErr    error
	getErr    error
	listErr   error
	deleteErr error
}

func newFakeRepository(items ...Secret) *fakeRepository {
	return &fakeRepository{items: append([]Secret(nil), items...)}
}

func (r *fakeRepository) Set(_ context.Context, key, value string) error {
	r.calls++
	if r.setErr != nil {
		return r.setErr
	}
	for index := range r.items {
		if r.items[index].Key == key {
			r.items[index].Value = value
			return nil
		}
	}
	r.items = append(r.items, Secret{Key: key, Value: value})
	return nil
}

func (r *fakeRepository) Get(_ context.Context, key string) (Secret, error) {
	r.calls++
	if r.getErr != nil {
		return Secret{}, r.getErr
	}
	for _, item := range r.items {
		if item.Key == key {
			return item, nil
		}
	}
	return Secret{}, ErrNotFound
}

func (r *fakeRepository) List(context.Context) ([]Secret, error) {
	r.calls++
	if r.listErr != nil {
		return nil, r.listErr
	}
	return append([]Secret(nil), r.items...), nil
}

func (r *fakeRepository) Delete(_ context.Context, key string) error {
	r.calls++
	if r.deleteErr != nil {
		return r.deleteErr
	}
	for index, item := range r.items {
		if item.Key == key {
			r.items = append(r.items[:index], r.items[index+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

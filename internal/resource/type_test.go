package resource

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestTypeServiceRegister(t *testing.T) {
	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	repository := &fakeTypeRepository{items: map[string]ResourceType{}}
	repository.afterCreate = func(item ResourceType) ResourceType {
		item.CreatedAt = createdAt
		return item
	}

	item, err := NewTypeService(repository).Register(context.Background(), " service ", " Service ", " owner ", " description ")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if item.Name != " service " || item.DisplayName != " Service " || item.Owner != " owner " || item.Description != " description " {
		t.Errorf("Register() = %#v, want caller text preserved", item)
	}
	if !item.CreatedAt.Equal(createdAt) {
		t.Errorf("Register() CreatedAt = %v, want %v", item.CreatedAt, createdAt)
	}
	if repository.getCalls != 1 {
		t.Errorf("Register() Get calls = %d, want 1", repository.getCalls)
	}
}

func TestTypeServiceValidationPrecedesRepository(t *testing.T) {
	tests := []struct {
		name        string
		typeName    string
		displayName string
		want        string
	}{
		{name: "name", typeName: " ", displayName: "display", want: "resource type name is required"},
		{name: "display name", typeName: "name", displayName: " ", want: "resource type display name is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &fakeTypeRepository{items: map[string]ResourceType{}}
			_, err := NewTypeService(repository).Register(context.Background(), test.typeName, test.displayName, "", "")
			if err == nil || err.Error() != test.want {
				t.Fatalf("Register() error = %v, want %q", err, test.want)
			}
			if repository.createCalls != 0 {
				t.Errorf("Register() Create calls = %d, want 0", repository.createCalls)
			}
		})
	}
}

func TestTypeServiceDuplicateAndMissingErrors(t *testing.T) {
	repository := &fakeTypeRepository{items: map[string]ResourceType{"repository": {Name: "repository"}}}
	service := NewTypeService(repository)

	_, err := service.Register(context.Background(), "repository", "Repository", "", "")
	if err == nil || err.Error() != `resource type "repository" already exists` || !errors.Is(err, ErrTypeExists) {
		t.Fatalf("duplicate Register() error = %v, want exact ErrTypeExists", err)
	}
	_, err = service.Get(context.Background(), "missing")
	if err == nil || err.Error() != `resource type "missing" not found` || !errors.Is(err, ErrTypeNotFound) {
		t.Fatalf("missing Get() error = %v, want exact ErrTypeNotFound", err)
	}
}

func TestTypeServiceListSortsByName(t *testing.T) {
	repository := &fakeTypeRepository{list: []ResourceType{{Name: "zeta"}, {Name: "alpha"}}}
	items, err := NewTypeService(repository).List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	got := []string{items[0].Name, items[1].Name}
	if want := []string{"alpha", "zeta"}; !reflect.DeepEqual(got, want) {
		t.Errorf("List() names = %v, want %v", got, want)
	}
}

type fakeTypeRepository struct {
	items       map[string]ResourceType
	list        []ResourceType
	afterCreate func(ResourceType) ResourceType
	createCalls int
	getCalls    int
}

func (r *fakeTypeRepository) Create(_ context.Context, item ResourceType) error {
	r.createCalls++
	if _, exists := r.items[item.Name]; exists {
		return ErrTypeExists
	}
	if r.afterCreate != nil {
		item = r.afterCreate(item)
	}
	r.items[item.Name] = item
	return nil
}

func (r *fakeTypeRepository) List(context.Context) ([]ResourceType, error) {
	return append([]ResourceType(nil), r.list...), nil
}

func (r *fakeTypeRepository) Get(_ context.Context, name string) (ResourceType, error) {
	r.getCalls++
	item, exists := r.items[name]
	if !exists {
		return ResourceType{}, ErrTypeNotFound
	}
	return item, nil
}

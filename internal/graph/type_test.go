package graph

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestTypeServiceRegisterSymmetricInverse(t *testing.T) {
	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	repository := &fakeTypeRepository{items: map[string]RelationType{}, createdAt: createdAt}
	item, err := NewTypeService(repository).Register(context.Background(), "related_to", "Related To", "", true, "core", "Symmetric relation")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if item.InverseName != item.Name {
		t.Errorf("Register() inverse = %q, want %q", item.InverseName, item.Name)
	}
	if !item.CreatedAt.Equal(createdAt) || repository.getCalls != 1 {
		t.Errorf("Register() = %#v, Get calls = %d; want stored timestamp and one fetch", item, repository.getCalls)
	}
}

func TestTypeServiceRegisterAllowsUnregisteredInverse(t *testing.T) {
	repository := &fakeTypeRepository{items: map[string]RelationType{}}
	item, err := NewTypeService(repository).Register(context.Background(), "supports", "Supports", "supported_by", false, "", "")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if item.InverseName != "supported_by" {
		t.Errorf("Register() inverse = %q, want supported_by", item.InverseName)
	}
}

func TestTypeServiceRegisterRejectsInvalidSymmetricInverse(t *testing.T) {
	repository := &fakeTypeRepository{items: map[string]RelationType{}}
	_, err := NewTypeService(repository).Register(context.Background(), "related_to", "Related To", "different", true, "", "")
	if err == nil || err.Error() != "symmetric relation type inverse must match its name" {
		t.Fatalf("Register() error = %v, want symmetric inverse error", err)
	}
	if repository.createCalls != 0 {
		t.Errorf("Register() Create calls = %d, want 0", repository.createCalls)
	}
}

func TestTypeServiceValidationDuplicateAndMissing(t *testing.T) {
	for _, test := range []struct {
		name, displayName, want string
	}{
		{name: " ", displayName: "display", want: "relation type name is required"},
		{name: "name", displayName: " ", want: "relation type display name is required"},
	} {
		repository := &fakeTypeRepository{items: map[string]RelationType{}}
		_, err := NewTypeService(repository).Register(context.Background(), test.name, test.displayName, "", false, "", "")
		if err == nil || err.Error() != test.want || repository.createCalls != 0 {
			t.Errorf("Register(%q, %q) error = %v, Create calls = %d; want %q and 0", test.name, test.displayName, err, repository.createCalls, test.want)
		}
	}

	repository := &fakeTypeRepository{items: map[string]RelationType{"supports": {Name: "supports"}}}
	service := NewTypeService(repository)
	_, err := service.Register(context.Background(), "supports", "Supports", "", false, "", "")
	if err == nil || err.Error() != `relation type "supports" already exists` || !errors.Is(err, ErrTypeExists) {
		t.Fatalf("duplicate Register() error = %v, want exact ErrTypeExists", err)
	}
	_, err = service.Get(context.Background(), "missing")
	if err == nil || err.Error() != `relation type "missing" not found` || !errors.Is(err, ErrTypeNotFound) {
		t.Fatalf("missing Get() error = %v, want exact ErrTypeNotFound", err)
	}
}

func TestTypeServiceListSortsByName(t *testing.T) {
	repository := &fakeTypeRepository{list: []RelationType{{Name: "zeta"}, {Name: "alpha"}}}
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
	items       map[string]RelationType
	list        []RelationType
	createdAt   time.Time
	createCalls int
	getCalls    int
}

func (r *fakeTypeRepository) Create(_ context.Context, item RelationType) error {
	r.createCalls++
	if _, exists := r.items[item.Name]; exists {
		return ErrTypeExists
	}
	item.CreatedAt = r.createdAt
	r.items[item.Name] = item
	return nil
}

func (r *fakeTypeRepository) List(context.Context) ([]RelationType, error) {
	return append([]RelationType(nil), r.list...), nil
}

func (r *fakeTypeRepository) Get(_ context.Context, name string) (RelationType, error) {
	r.getCalls++
	item, exists := r.items[name]
	if !exists {
		return RelationType{}, ErrTypeNotFound
	}
	return item, nil
}

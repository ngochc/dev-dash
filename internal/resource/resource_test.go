package resource

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestServiceAddGeneratesIDAndFetchesStoredResource(t *testing.T) {
	createdAt := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	repository := newFakeResourceRepository()
	repository.afterCreate = func(item Resource) Resource {
		item.CreatedAt = createdAt
		item.UpdatedAt = createdAt
		return item
	}
	types := &fakeTypeRepository{items: map[string]ResourceType{"service": {Name: "service"}}}

	item, err := NewService(repository, types).Add(context.Background(), "service", " api ", " https://example.test ")
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if item.ID == "" {
		t.Fatal("Add() ID is empty")
	}
	if item.Name != " api " || item.URL != " https://example.test " {
		t.Errorf("Add() = %#v, want caller text preserved", item)
	}
	if !item.CreatedAt.Equal(createdAt) || repository.getCalls != 1 {
		t.Errorf("Add() = %#v, Get calls = %d; want stored timestamps and one fetch", item, repository.getCalls)
	}
}

func TestServiceValidatesBeforeRepositoryCalls(t *testing.T) {
	tests := []struct {
		name string
		run  func(*Service) error
		want string
	}{
		{name: "add type", run: func(s *Service) error { _, err := s.Add(context.Background(), " ", "name", ""); return err }, want: "resource type is required"},
		{name: "add name", run: func(s *Service) error { _, err := s.Add(context.Background(), "service", " ", ""); return err }, want: "resource name is required"},
		{name: "get ID", run: func(s *Service) error { _, err := s.Get(context.Background(), " "); return err }, want: "resource ID is required"},
		{name: "update ID", run: func(s *Service) error {
			_, err := s.Update(context.Background(), " ", "service", "name", "")
			return err
		}, want: "resource ID is required"},
		{name: "update type", run: func(s *Service) error { _, err := s.Update(context.Background(), "id", " ", "name", ""); return err }, want: "resource type is required"},
		{name: "update name", run: func(s *Service) error { _, err := s.Update(context.Background(), "id", "service", " ", ""); return err }, want: "resource name is required"},
		{name: "remove ID", run: func(s *Service) error { _, err := s.Remove(context.Background(), " "); return err }, want: "resource ID is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := newFakeResourceRepository()
			types := &countingTypeRepository{items: map[string]ResourceType{"service": {Name: "service"}}}
			err := test.run(NewService(repository, types))
			if err == nil || err.Error() != test.want {
				t.Fatalf("operation error = %v, want %q", err, test.want)
			}
			if repository.totalCalls() != 0 || types.getCalls != 0 {
				t.Errorf("repository calls = %d, type calls = %d; want zero", repository.totalCalls(), types.getCalls)
			}
		})
	}
}

func TestServiceUnknownTypePrecedesResourceMutation(t *testing.T) {
	repository := newFakeResourceRepository(Resource{ID: "resource-1", Type: "service", Name: "api"})
	types := &countingTypeRepository{items: map[string]ResourceType{}}
	service := NewService(repository, types)

	for _, run := range []func() error{
		func() error { _, err := service.Add(context.Background(), "missing", "api", ""); return err },
		func() error {
			_, err := service.Update(context.Background(), "resource-1", "missing", "api", "")
			return err
		},
	} {
		err := run()
		if err == nil || err.Error() != `resource type "missing" not found` || !errors.Is(err, ErrTypeNotFound) {
			t.Fatalf("operation error = %v, want exact ErrTypeNotFound", err)
		}
	}
	if repository.createCalls != 0 || repository.updateCalls != 0 {
		t.Errorf("mutation calls = create %d, update %d; want zero", repository.createCalls, repository.updateCalls)
	}
}

func TestServiceUpdatePreservesProviderFields(t *testing.T) {
	lastSeenAt := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	original := Resource{
		ID: "resource-1", Type: "repository", IntegrationID: "github", ProviderID: "42",
		ExternalKey: "org/repo", Name: "old", URL: "old-url", Metadata: `{"key":"value"}`,
		CreatedAt: lastSeenAt, UpdatedAt: lastSeenAt, LastSeenAt: &lastSeenAt,
	}
	repository := newFakeResourceRepository(original)
	types := &countingTypeRepository{items: map[string]ResourceType{"service": {Name: "service"}}}

	got, err := NewService(repository, types).Update(context.Background(), original.ID, "service", "new", "new-url")
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	want := original
	want.Type, want.Name, want.URL = "service", "new", "new-url"
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Update() = %#v, want %#v", got, want)
	}
}

func TestServiceListSortsByNameThenID(t *testing.T) {
	repository := newFakeResourceRepository(
		Resource{ID: "2", Name: "beta"}, Resource{ID: "3", Name: "alpha"}, Resource{ID: "1", Name: "alpha"},
	)
	items, err := NewService(repository, &countingTypeRepository{}).List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	got := []string{items[0].ID, items[1].ID, items[2].ID}
	if want := []string{"1", "3", "2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("List() IDs = %v, want %v", got, want)
	}
}

func TestServiceMissingGetUpdateRemove(t *testing.T) {
	service := NewService(newFakeResourceRepository(), &countingTypeRepository{items: map[string]ResourceType{"service": {Name: "service"}}})
	for name, run := range map[string]func() error{
		"get": func() error { _, err := service.Get(context.Background(), "missing"); return err },
		"update": func() error {
			_, err := service.Update(context.Background(), "missing", "service", "name", "")
			return err
		},
		"remove": func() error { _, err := service.Remove(context.Background(), "missing"); return err },
	} {
		t.Run(name, func(t *testing.T) {
			err := run()
			if err == nil || err.Error() != `resource "missing" not found` || !errors.Is(err, ErrNotFound) {
				t.Fatalf("operation error = %v, want exact ErrNotFound", err)
			}
		})
	}
}

type fakeResourceRepository struct {
	items       map[string]Resource
	afterCreate func(Resource) Resource
	createCalls int
	listCalls   int
	getCalls    int
	updateCalls int
	deleteCalls int
}

func newFakeResourceRepository(items ...Resource) *fakeResourceRepository {
	r := &fakeResourceRepository{items: make(map[string]Resource)}
	for _, item := range items {
		r.items[item.ID] = item
	}
	return r
}

func (r *fakeResourceRepository) Create(_ context.Context, item Resource) error {
	r.createCalls++
	if r.afterCreate != nil {
		item = r.afterCreate(item)
	}
	r.items[item.ID] = item
	return nil
}
func (r *fakeResourceRepository) List(context.Context) ([]Resource, error) {
	r.listCalls++
	items := make([]Resource, 0, len(r.items))
	for _, item := range r.items {
		items = append(items, item)
	}
	return items, nil
}
func (r *fakeResourceRepository) Get(_ context.Context, id string) (Resource, error) {
	r.getCalls++
	item, ok := r.items[id]
	if !ok {
		return Resource{}, ErrNotFound
	}
	return item, nil
}
func (r *fakeResourceRepository) Update(_ context.Context, item Resource) error {
	r.updateCalls++
	if _, ok := r.items[item.ID]; !ok {
		return ErrNotFound
	}
	r.items[item.ID] = item
	return nil
}
func (r *fakeResourceRepository) Delete(_ context.Context, id string) error {
	r.deleteCalls++
	if _, ok := r.items[id]; !ok {
		return ErrNotFound
	}
	delete(r.items, id)
	return nil
}
func (r *fakeResourceRepository) totalCalls() int {
	return r.createCalls + r.listCalls + r.getCalls + r.updateCalls + r.deleteCalls
}

type countingTypeRepository struct {
	items    map[string]ResourceType
	getCalls int
}

func (r *countingTypeRepository) Create(context.Context, ResourceType) error   { return nil }
func (r *countingTypeRepository) List(context.Context) ([]ResourceType, error) { return nil, nil }
func (r *countingTypeRepository) Get(_ context.Context, name string) (ResourceType, error) {
	r.getCalls++
	item, ok := r.items[name]
	if !ok {
		return ResourceType{}, ErrTypeNotFound
	}
	return item, nil
}

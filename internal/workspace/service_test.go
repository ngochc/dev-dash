package workspace

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

func TestServiceAddWithExplicitPath(t *testing.T) {
	directory := t.TempDir()
	service := NewService(newFakeRepository())

	item, err := service.Add(context.Background(), "devdash", filepath.Join(directory, "."))
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if item.ID == "" {
		t.Fatal("Add() returned an empty ID")
	}
	if item.Name != "devdash" {
		t.Errorf("Add() name = %q, want %q", item.Name, "devdash")
	}
	if item.LocalPath != directory {
		t.Errorf("Add() path = %q, want %q", item.LocalPath, directory)
	}
}

func TestServiceAddUsesCurrentDirectory(t *testing.T) {
	directory := t.TempDir()
	t.Chdir(directory)
	service := NewService(newFakeRepository())

	item, err := service.Add(context.Background(), "devdash", "")
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if item.LocalPath != directory {
		t.Errorf("Add() path = %q, want %q", item.LocalPath, directory)
	}
}

func TestServiceAddRejectsDuplicateName(t *testing.T) {
	service := NewService(newFakeRepository())
	ctx := context.Background()

	if _, err := service.Add(ctx, "devdash", t.TempDir()); err != nil {
		t.Fatalf("first Add() error = %v", err)
	}
	if _, err := service.Add(ctx, "devdash", t.TempDir()); !errors.Is(err, ErrNameExists) {
		t.Fatalf("second Add() error = %v, want ErrNameExists", err)
	}
}

func TestServiceListEmpty(t *testing.T) {
	items, err := NewService(newFakeRepository()).List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("List() returned %d workspaces, want 0", len(items))
	}
}

func TestServiceListOrdersByName(t *testing.T) {
	repository := newFakeRepository(
		Workspace{ID: "2", Name: "frontend", LocalPath: "/frontend"},
		Workspace{ID: "1", Name: "devdash", LocalPath: "/devdash"},
	)

	items, err := NewService(repository).List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	got := []string{items[0].Name, items[1].Name}
	want := []string{"devdash", "frontend"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("List() names = %v, want %v", got, want)
	}
}

func TestServiceGetByIDBeforeName(t *testing.T) {
	repository := newFakeRepository(
		Workspace{ID: "target", Name: "by-id"},
		Workspace{ID: "other", Name: "target"},
	)

	item, err := NewService(repository).Get(context.Background(), "target")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if item.Name != "by-id" {
		t.Errorf("Get() name = %q, want ID match %q", item.Name, "by-id")
	}
}

func TestServiceGetByName(t *testing.T) {
	repository := newFakeRepository(Workspace{ID: "workspace-id", Name: "devdash"})

	item, err := NewService(repository).Get(context.Background(), "devdash")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if item.ID != "workspace-id" {
		t.Errorf("Get() ID = %q, want %q", item.ID, "workspace-id")
	}
}

func TestServiceGetMissing(t *testing.T) {
	_, err := NewService(newFakeRepository()).Get(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() error = %v, want ErrNotFound", err)
	}
}

func TestServiceRemoveByID(t *testing.T) {
	repository := newFakeRepository(Workspace{ID: "workspace-id", Name: "devdash"})

	item, err := NewService(repository).Remove(context.Background(), "workspace-id")
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if item.Name != "devdash" {
		t.Errorf("Remove() name = %q, want %q", item.Name, "devdash")
	}
	if len(repository.items) != 0 {
		t.Fatalf("Remove() left %d workspaces, want 0", len(repository.items))
	}
}

func TestServiceRemoveByName(t *testing.T) {
	repository := newFakeRepository(Workspace{ID: "workspace-id", Name: "devdash"})

	item, err := NewService(repository).Remove(context.Background(), "devdash")
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if item.ID != "workspace-id" {
		t.Errorf("Remove() ID = %q, want %q", item.ID, "workspace-id")
	}
	if len(repository.items) != 0 {
		t.Fatalf("Remove() left %d workspaces, want 0", len(repository.items))
	}
}

func TestServiceAddRejectsMissingDirectory(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")

	_, err := NewService(newFakeRepository()).Add(context.Background(), "devdash", missing)
	if err == nil {
		t.Fatal("Add() error = nil, want missing directory error")
	}
}

func TestServicePropagatesRepositoryErrors(t *testing.T) {
	repositoryError := errors.New("repository failed")

	t.Run("create", func(t *testing.T) {
		repository := newFakeRepository()
		repository.createErr = repositoryError
		_, err := NewService(repository).Add(context.Background(), "devdash", t.TempDir())
		if !errors.Is(err, repositoryError) {
			t.Fatalf("Add() error = %v, want repository error", err)
		}
	})

	t.Run("list", func(t *testing.T) {
		repository := newFakeRepository()
		repository.listErr = repositoryError
		_, err := NewService(repository).List(context.Background())
		if !errors.Is(err, repositoryError) {
			t.Fatalf("List() error = %v, want repository error", err)
		}
	})

	t.Run("get by ID", func(t *testing.T) {
		repository := newFakeRepository()
		repository.getByIDErr = repositoryError
		_, err := NewService(repository).Get(context.Background(), "devdash")
		if !errors.Is(err, repositoryError) {
			t.Fatalf("Get() error = %v, want repository error", err)
		}
	})

	t.Run("get by name", func(t *testing.T) {
		repository := newFakeRepository()
		repository.getByNameErr = repositoryError
		_, err := NewService(repository).Get(context.Background(), "devdash")
		if !errors.Is(err, repositoryError) {
			t.Fatalf("Get() error = %v, want repository error", err)
		}
	})

	t.Run("delete", func(t *testing.T) {
		repository := newFakeRepository(Workspace{ID: "workspace-id", Name: "devdash"})
		repository.deleteErr = repositoryError
		_, err := NewService(repository).Remove(context.Background(), "workspace-id")
		if !errors.Is(err, repositoryError) {
			t.Fatalf("Remove() error = %v, want repository error", err)
		}
	})
}

func TestServiceAddRejectsBlankName(t *testing.T) {
	_, err := NewService(newFakeRepository()).Add(context.Background(), " ", t.TempDir())
	if err == nil {
		t.Fatal("Add() error = nil, want required name error")
	}
}

type fakeRepository struct {
	items        []Workspace
	createErr    error
	listErr      error
	getByIDErr   error
	getByNameErr error
	deleteErr    error
}

func newFakeRepository(items ...Workspace) *fakeRepository {
	return &fakeRepository{items: append([]Workspace(nil), items...)}
}

func (r *fakeRepository) Create(_ context.Context, item Workspace) error {
	if r.createErr != nil {
		return r.createErr
	}
	for _, existing := range r.items {
		if existing.Name == item.Name {
			return ErrNameExists
		}
	}
	r.items = append(r.items, item)
	return nil
}

func (r *fakeRepository) List(context.Context) ([]Workspace, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	return append([]Workspace(nil), r.items...), nil
}

func (r *fakeRepository) GetByID(_ context.Context, id string) (Workspace, error) {
	if r.getByIDErr != nil {
		return Workspace{}, r.getByIDErr
	}
	for _, item := range r.items {
		if item.ID == id {
			return item, nil
		}
	}
	return Workspace{}, ErrNotFound
}

func (r *fakeRepository) GetByName(_ context.Context, name string) (Workspace, error) {
	if r.getByNameErr != nil {
		return Workspace{}, r.getByNameErr
	}
	for _, item := range r.items {
		if item.Name == name {
			return item, nil
		}
	}
	return Workspace{}, ErrNotFound
}

func (r *fakeRepository) Delete(_ context.Context, id string) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	for index, item := range r.items {
		if item.ID == id {
			r.items = append(r.items[:index], r.items[index+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

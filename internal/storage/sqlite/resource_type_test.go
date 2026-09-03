package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/ngochc/dev-dash/internal/resource"
)

func TestResourceTypeRepositoryLifecycle(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "devdash.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	repository := NewResourceTypeRepository(db)

	item := resource.ResourceType{Name: "service_component", DisplayName: "Service Component", Owner: "core", Description: "Deployable service unit"}
	if err := repository.Create(ctx, item); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := repository.Create(ctx, item); !errors.Is(err, resource.ErrTypeExists) {
		t.Fatalf("duplicate Create() error = %v, want ErrTypeExists", err)
	}
	got, err := repository.Get(ctx, item.Name)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Name != item.Name || got.DisplayName != item.DisplayName || got.Owner != item.Owner || got.Description != item.Description || got.CreatedAt.IsZero() {
		t.Errorf("Get() = %#v, want all stored columns and CreatedAt", got)
	}
	items, err := repository.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	for index := 1; index < len(items); index++ {
		if items[index-1].Name > items[index].Name {
			t.Fatalf("List() is not ordered at %q, %q", items[index-1].Name, items[index].Name)
		}
	}
	if _, err := repository.Get(ctx, "missing"); !errors.Is(err, resource.ErrTypeNotFound) {
		t.Errorf("missing Get() error = %v, want ErrTypeNotFound", err)
	}
}

func TestResourceTypeRepositoryNullableText(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "devdash.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	repository := NewResourceTypeRepository(db)
	if err := repository.Create(ctx, resource.ResourceType{Name: "minimal", DisplayName: "Minimal"}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	got, err := repository.Get(ctx, "minimal")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Owner != "" || got.Description != "" {
		t.Errorf("Get() nullable text = owner %q, description %q; want empty", got.Owner, got.Description)
	}
}

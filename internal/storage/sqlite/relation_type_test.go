package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/ngochc/dev-dash/internal/graph"
)

func TestRelationTypeRepositoryLifecycle(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "devdash.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	repository := NewRelationTypeRepository(db)

	item := graph.RelationType{Name: "supports", DisplayName: "Supports", InverseName: "supported_by", Owner: "core", Description: "Source supports target"}
	if err := repository.Create(ctx, item); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := repository.Create(ctx, item); !errors.Is(err, graph.ErrTypeExists) {
		t.Fatalf("duplicate Create() error = %v, want ErrTypeExists", err)
	}
	got, err := repository.Get(ctx, item.Name)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Name != item.Name || got.DisplayName != item.DisplayName || got.InverseName != item.InverseName || got.Symmetric || got.Owner != item.Owner || got.Description != item.Description || got.CreatedAt.IsZero() {
		t.Errorf("Get() = %#v, want all stored columns and CreatedAt", got)
	}

	symmetric := graph.RelationType{Name: "cooperates_with", DisplayName: "Cooperates With", InverseName: "cooperates_with", Symmetric: true}
	if err := repository.Create(ctx, symmetric); err != nil {
		t.Fatalf("Create(symmetric) error = %v", err)
	}
	symmetricGot, err := repository.Get(ctx, symmetric.Name)
	if err != nil {
		t.Fatalf("Get(symmetric) error = %v", err)
	}
	if !symmetricGot.Symmetric || symmetricGot.InverseName != symmetric.Name || symmetricGot.Owner != "" || symmetricGot.Description != "" {
		t.Errorf("Get(symmetric) = %#v, want symmetric row with nullable text", symmetricGot)
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
	if _, err := repository.Get(ctx, "missing"); !errors.Is(err, graph.ErrTypeNotFound) {
		t.Errorf("missing Get() error = %v, want ErrTypeNotFound", err)
	}
}

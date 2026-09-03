package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/ngochc/dev-dash/internal/workspace"
)

func TestWorkspaceRepositoryLifecycle(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "devdash.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	repository := NewWorkspaceRepository(db)
	frontend := workspace.Workspace{ID: "workspace-2", Name: "frontend", LocalPath: "/frontend"}
	devdash := workspace.Workspace{ID: "workspace-1", Name: "devdash", LocalPath: "/devdash"}
	if err := repository.Create(ctx, frontend); err != nil {
		t.Fatalf("Create(frontend) error = %v", err)
	}
	if err := repository.Create(ctx, devdash); err != nil {
		t.Fatalf("Create(devdash) error = %v", err)
	}
	if err := repository.Create(ctx, workspace.Workspace{ID: "other", Name: "devdash"}); !errors.Is(err, workspace.ErrNameExists) {
		t.Fatalf("duplicate Create() error = %v, want ErrNameExists", err)
	}

	items, err := repository.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("List() returned %d workspaces, want 2", len(items))
	}
	if items[0] != devdash || items[1] != frontend {
		t.Errorf("List() = %#v, want devdash then frontend", items)
	}

	byID, err := repository.GetByID(ctx, devdash.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if byID != devdash {
		t.Errorf("GetByID() = %#v, want %#v", byID, devdash)
	}

	byName, err := repository.GetByName(ctx, frontend.Name)
	if err != nil {
		t.Fatalf("GetByName() error = %v", err)
	}
	if byName != frontend {
		t.Errorf("GetByName() = %#v, want %#v", byName, frontend)
	}

	if _, err := db.ExecContext(ctx, `
		INSERT INTO resources (id, type, name) VALUES ('resource-1', 'repository', 'devdash');
		INSERT INTO workspace_resources (workspace_id, resource_id) VALUES ('workspace-1', 'resource-1');
	`); err != nil {
		t.Fatalf("create workspace membership: %v", err)
	}
	if err := repository.Delete(ctx, devdash.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	var membershipCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM workspace_resources WHERE workspace_id = ?", devdash.ID).Scan(&membershipCount); err != nil {
		t.Fatalf("query workspace membership: %v", err)
	}
	if membershipCount != 0 {
		t.Errorf("workspace membership count = %d, want 0", membershipCount)
	}
}

func TestWorkspaceRepositoryMissing(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "devdash.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	repository := NewWorkspaceRepository(db)
	if _, err := repository.GetByID(ctx, "missing"); !errors.Is(err, workspace.ErrNotFound) {
		t.Errorf("GetByID() error = %v, want ErrNotFound", err)
	}
	if _, err := repository.GetByName(ctx, "missing"); !errors.Is(err, workspace.ErrNotFound) {
		t.Errorf("GetByName() error = %v, want ErrNotFound", err)
	}
	if err := repository.Delete(ctx, "missing"); !errors.Is(err, workspace.ErrNotFound) {
		t.Errorf("Delete() error = %v, want ErrNotFound", err)
	}
}

package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/ngochc/dev-dash/internal/resource"
	"github.com/ngochc/dev-dash/internal/workspace"
)

func TestWorkspaceResourceRepositoryLifecycle(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "devdash.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	workspaceRepository := NewWorkspaceRepository(db)
	resourceRepository := NewResourceRepository(db)
	membershipRepository := NewWorkspaceResourceRepository(db)
	for _, item := range []workspace.Workspace{{ID: "workspace-1", Name: "one"}, {ID: "workspace-2", Name: "two"}} {
		if err := workspaceRepository.Create(ctx, item); err != nil {
			t.Fatalf("Create(workspace %s) error = %v", item.ID, err)
		}
	}
	for _, item := range []resource.Resource{
		{ID: "2", Type: "service", Name: "beta"},
		{ID: "3", Type: "service", Name: "alpha", URL: "https://example.test"},
		{ID: "1", Type: "service", Name: "alpha"},
	} {
		if err := resourceRepository.Create(ctx, item); err != nil {
			t.Fatalf("Create(resource %s) error = %v", item.ID, err)
		}
	}

	memberships := []workspace.ResourceMembership{
		{WorkspaceID: "workspace-1", ResourceID: "2", Role: "dependency"},
		{WorkspaceID: "workspace-1", ResourceID: "3", Role: ""},
		{WorkspaceID: "workspace-1", ResourceID: "1", Role: " custom "},
		{WorkspaceID: "workspace-2", ResourceID: "1", Role: "primary"},
	}
	for _, item := range memberships {
		if err := membershipRepository.Add(ctx, item); err != nil {
			t.Fatalf("Add(%s/%s) error = %v", item.WorkspaceID, item.ResourceID, err)
		}
	}
	if err := membershipRepository.Add(ctx, memberships[0]); !errors.Is(err, workspace.ErrMembershipExists) {
		t.Fatalf("duplicate Add() error = %v, want ErrMembershipExists", err)
	}

	var emptyRoleIsNull bool
	if err := db.QueryRowContext(ctx, `SELECT role IS NULL FROM workspace_resources WHERE workspace_id = 'workspace-1' AND resource_id = '3'`).Scan(&emptyRoleIsNull); err != nil {
		t.Fatalf("query empty role: %v", err)
	}
	if !emptyRoleIsNull {
		t.Error("empty role was not stored as NULL")
	}

	items, err := membershipRepository.ListByWorkspace(ctx, "workspace-1")
	if err != nil {
		t.Fatalf("ListByWorkspace() error = %v", err)
	}
	if len(items) != 3 || items[0].Resource.ID != "1" || items[1].Resource.ID != "3" || items[2].Resource.ID != "2" {
		t.Fatalf("ListByWorkspace() = %#v, want name then ID order", items)
	}
	if items[0].Role != " custom " || items[1].Role != "" || items[1].Resource.URL != "https://example.test" || items[0].CreatedAt.IsZero() {
		t.Errorf("ListByWorkspace() did not preserve joined values: %#v", items)
	}

	if err := membershipRepository.Remove(ctx, "workspace-1", "1"); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if _, err := resourceRepository.Get(ctx, "1"); err != nil {
		t.Errorf("resource after membership Remove() error = %v", err)
	}
	otherItems, err := membershipRepository.ListByWorkspace(ctx, "workspace-2")
	if err != nil || len(otherItems) != 1 || otherItems[0].ResourceID != "1" {
		t.Errorf("other workspace memberships = %#v, error = %v", otherItems, err)
	}
	if err := membershipRepository.Remove(ctx, "workspace-1", "1"); !errors.Is(err, workspace.ErrMembershipNotFound) {
		t.Errorf("missing Remove() error = %v, want ErrMembershipNotFound", err)
	}
}

func TestWorkspaceResourceRepositoryForeignKeysAndResourceCascade(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "devdash.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	workspaceRepository := NewWorkspaceRepository(db)
	resourceRepository := NewResourceRepository(db)
	membershipRepository := NewWorkspaceResourceRepository(db)
	if err := workspaceRepository.Create(ctx, workspace.Workspace{ID: "workspace", Name: "workspace"}); err != nil {
		t.Fatalf("Create(workspace) error = %v", err)
	}
	if err := resourceRepository.Create(ctx, resource.Resource{ID: "resource", Type: "service", Name: "resource"}); err != nil {
		t.Fatalf("Create(resource) error = %v", err)
	}

	if err := membershipRepository.Add(ctx, workspace.ResourceMembership{WorkspaceID: "missing", ResourceID: "resource"}); err == nil {
		t.Error("Add(unknown workspace) error = nil, want foreign-key error")
	}
	if err := membershipRepository.Add(ctx, workspace.ResourceMembership{WorkspaceID: "workspace", ResourceID: "missing"}); err == nil {
		t.Error("Add(unknown resource) error = nil, want foreign-key error")
	}
	if err := membershipRepository.Add(ctx, workspace.ResourceMembership{WorkspaceID: "workspace", ResourceID: "resource"}); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if err := resourceRepository.Delete(ctx, "resource"); err != nil {
		t.Fatalf("Delete(resource) error = %v", err)
	}
	items, err := membershipRepository.ListByWorkspace(ctx, "workspace")
	if err != nil {
		t.Fatalf("ListByWorkspace() error = %v", err)
	}
	if len(items) != 0 {
		t.Errorf("memberships after resource cascade = %#v, want empty", items)
	}
}

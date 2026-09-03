package sqlite

import (
	"context"
	"testing"

	"github.com/ngochc/dev-dash/internal/repository"
)

func TestRepositoryResourceStoreRefreshIsIdempotent(t *testing.T) {
	ctx := context.Background()
	db := openWorkspaceConfigTestDB(t)
	createWorkspaceConfigTestWorkspace(t, db, "workspace-1", "one")
	createWorkspaceConfigTestWorkspace(t, db, "workspace-2", "two")
	store := NewRepositoryResourceStore(db)
	source := repository.Source{Provider: "github", Name: "github.com", BaseURL: "https://github.com"}
	remote := repository.Remote{
		ProviderID:  "R_1",
		ExternalKey: "example/old-name",
		Name:        "old-name",
		URL:         "https://github.com/example/old-name",
	}
	if err := store.UpsertDiscovered(ctx, source, "workspace-1", []repository.Remote{remote}); err != nil {
		t.Fatalf("first UpsertDiscovered() error = %v", err)
	}
	first, err := store.ListByWorkspace(ctx, "workspace-1")
	if err != nil {
		t.Fatalf("first ListByWorkspace() error = %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("first ListByWorkspace() = %#v, want one repository", first)
	}

	remote.ExternalKey = "example/new-name"
	remote.Name = "new-name"
	remote.URL = "https://github.com/example/new-name"
	if err := store.UpsertDiscovered(ctx, source, "workspace-1", []repository.Remote{remote}); err != nil {
		t.Fatalf("second UpsertDiscovered() error = %v", err)
	}
	if err := store.UpsertDiscovered(ctx, source, "workspace-2", []repository.Remote{remote}); err != nil {
		t.Fatalf("second workspace UpsertDiscovered() error = %v", err)
	}
	updated, err := store.ListByWorkspace(ctx, "workspace-1")
	if err != nil {
		t.Fatalf("updated ListByWorkspace() error = %v", err)
	}
	if len(updated) != 1 || updated[0].ResourceID != first[0].ResourceID || updated[0].ExternalKey != "example/new-name" {
		t.Errorf("updated repository = %#v, want reused resource %q", updated, first[0].ResourceID)
	}
	secondWorkspace, err := store.ListByWorkspace(ctx, "workspace-2")
	if err != nil || len(secondWorkspace) != 1 || secondWorkspace[0].ResourceID != first[0].ResourceID {
		t.Errorf("second workspace repositories = %#v, %v", secondWorkspace, err)
	}

	var integrationCount, resourceCount, membershipCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM integrations WHERE provider = 'github' AND name = 'github.com'").Scan(&integrationCount); err != nil {
		t.Fatalf("count integrations: %v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM resources WHERE provider_id = 'R_1'").Scan(&resourceCount); err != nil {
		t.Fatalf("count resources: %v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM workspace_resources WHERE resource_id = ?", first[0].ResourceID).Scan(&membershipCount); err != nil {
		t.Fatalf("count memberships: %v", err)
	}
	if integrationCount != 1 || resourceCount != 1 || membershipCount != 2 {
		t.Errorf("counts = integration %d, resource %d, membership %d; want 1, 1, 2", integrationCount, resourceCount, membershipCount)
	}
}

func TestRepositoryResourceStoreReusesExternalIdentity(t *testing.T) {
	ctx := context.Background()
	db := openWorkspaceConfigTestDB(t)
	createWorkspaceConfigTestWorkspace(t, db, "workspace-1", "one")
	if _, err := db.ExecContext(ctx, `
		INSERT INTO integrations (id, provider, name, base_url)
		VALUES ('integration-1', 'github', 'github.com', 'https://github.com');
		INSERT INTO resources (id, type, integration_id, external_key, name, url)
		VALUES ('resource-1', 'repository', 'integration-1', 'team/api', 'api', 'https://github.com/team/api');
	`); err != nil {
		t.Fatalf("seed external identity: %v", err)
	}
	store := NewRepositoryResourceStore(db)
	remote := repository.Remote{ProviderID: "R_1", ExternalKey: "team/api", Name: "api", URL: "https://github.com/team/api"}
	if err := store.UpsertDiscovered(ctx, repository.Source{Provider: "github", Name: "github.com", BaseURL: "https://github.com"}, "workspace-1", []repository.Remote{remote}); err != nil {
		t.Fatalf("UpsertDiscovered() error = %v", err)
	}
	items, err := store.ListByWorkspace(ctx, "workspace-1")
	if err != nil {
		t.Fatalf("ListByWorkspace() error = %v", err)
	}
	if len(items) != 1 || items[0].ResourceID != "resource-1" || items[0].ProviderID != "R_1" {
		t.Errorf("repositories = %#v, want reused resource with provider ID", items)
	}
}

func TestRepositoryResourceStoreCheckoutLocation(t *testing.T) {
	ctx := context.Background()
	db := openWorkspaceConfigTestDB(t)
	createWorkspaceConfigTestWorkspace(t, db, "workspace-1", "one")
	store := NewRepositoryResourceStore(db)
	remote := repository.Remote{ProviderID: "R_1", ExternalKey: "example/api", Name: "api", URL: "https://github.com/example/api"}
	if err := store.UpsertDiscovered(ctx, repository.Source{Provider: "github", Name: "github.com", BaseURL: "https://github.com"}, "workspace-1", []repository.Remote{remote}); err != nil {
		t.Fatalf("UpsertDiscovered() error = %v", err)
	}
	items, err := store.ListByWorkspace(ctx, "workspace-1")
	if err != nil || len(items) != 1 {
		t.Fatalf("ListByWorkspace() = %#v, %v", items, err)
	}
	if items[0].CheckoutPath != "" {
		t.Fatalf("initial checkout path = %q, want empty", items[0].CheckoutPath)
	}

	if err := store.SetCheckout(ctx, "workspace-1", items[0].ResourceID, "/workspace/repos/api"); err != nil {
		t.Fatalf("SetCheckout() error = %v", err)
	}
	items, err = store.ListByWorkspace(ctx, "workspace-1")
	if err != nil || items[0].CheckoutPath != "/workspace/repos/api" {
		t.Errorf("checkout after registration = %#v, %v", items, err)
	}
	if err := store.SetCheckout(ctx, "workspace-1", items[0].ResourceID, "/workspace/repos/api-new"); err != nil {
		t.Fatalf("updated SetCheckout() error = %v", err)
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM resource_locations WHERE resource_id = ? AND location_type = 'local_checkout'", items[0].ResourceID).Scan(&count); err != nil {
		t.Fatalf("count checkout locations: %v", err)
	}
	if count != 1 {
		t.Errorf("checkout location count = %d, want 1", count)
	}
}

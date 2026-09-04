package sqlite

import (
	"context"
	"strings"
	"testing"

	"github.com/ngochc/dev-dash/internal/resource"
)

func TestResourceSyncStoreUpsertsPagesByProviderID(t *testing.T) {
	ctx := context.Background()
	db := openWorkspaceConfigTestDB(t)
	createWorkspaceConfigTestWorkspace(t, db, "workspace-1", "one")
	createWorkspaceConfigTestWorkspace(t, db, "workspace-2", "two")
	store := NewResourceSyncStore(db)
	source := resource.Source{Provider: "confluence", Name: "https://wiki.example/confluence", BaseURL: "https://wiki.example/confluence"}
	page := resource.Discovered{ProviderID: "123", ExternalKey: "DOC/123", Name: "Old title", URL: "https://wiki.example/old", Metadata: `{"confluence_updated_at":"old"}`}

	if err := store.UpsertDiscovered(ctx, source, "workspace-1", "confluence_page", []resource.Discovered{page}); err != nil {
		t.Fatalf("first UpsertDiscovered() error = %v", err)
	}
	first := listSyncedResources(t, store, "workspace-1")
	if len(first) != 1 {
		t.Fatalf("first list = %#v, want one page", first)
	}

	page.ExternalKey = "DOC/renamed-123"
	page.Name = "New title"
	page.URL = "https://wiki.example/new"
	page.Metadata = `{"confluence_updated_at":"new"}`
	if err := store.UpsertDiscovered(ctx, source, "workspace-1", "confluence_page", []resource.Discovered{page}); err != nil {
		t.Fatalf("second UpsertDiscovered() error = %v", err)
	}
	if err := store.UpsertDiscovered(ctx, source, "workspace-2", "confluence_page", []resource.Discovered{page}); err != nil {
		t.Fatalf("second workspace UpsertDiscovered() error = %v", err)
	}
	updated := listSyncedResources(t, store, "workspace-1")
	if len(updated) != 1 || updated[0].Resource.ID != first[0].Resource.ID {
		t.Fatalf("updated list = %#v, want stable resource %q", updated, first[0].Resource.ID)
	}
	if got := updated[0].Resource; got.ExternalKey != page.ExternalKey || got.Name != page.Name || got.URL != page.URL || got.Metadata != page.Metadata {
		t.Errorf("updated resource = %#v, want latest discovery %#v", got, page)
	}
	secondWorkspace := listSyncedResources(t, store, "workspace-2")
	if len(secondWorkspace) != 1 || secondWorkspace[0].Resource.ID != first[0].Resource.ID {
		t.Errorf("second workspace = %#v, want shared resource", secondWorkspace)
	}
}

func TestResourceSyncStorePromotesOnlyLegacyExternalIdentity(t *testing.T) {
	ctx := context.Background()
	db := openWorkspaceConfigTestDB(t)
	createWorkspaceConfigTestWorkspace(t, db, "workspace-1", "one")
	if _, err := db.ExecContext(ctx, `
		INSERT INTO integrations (id, provider, name, base_url)
		VALUES ('integration-1', 'confluence', 'https://wiki.example', 'https://wiki.example');
		INSERT INTO resources (id, type, integration_id, provider_id, external_key, name)
		VALUES ('provider-resource', 'confluence_page', 'integration-1', '123', 'DOC/old', 'provider match');
		INSERT INTO resources (id, type, integration_id, external_key, name)
		VALUES ('legacy-resource', 'confluence_page', 'integration-1', 'DOC/456', 'legacy match');
	`); err != nil {
		t.Fatalf("seed resources: %v", err)
	}
	store := NewResourceSyncStore(db)
	source := resource.Source{Provider: "confluence", Name: "https://wiki.example", BaseURL: "https://wiki.example"}
	items := []resource.Discovered{
		{ProviderID: "123", ExternalKey: "DOC/456", Name: "provider wins"},
		{ProviderID: "456", ExternalKey: "DOC/456", Name: "legacy promoted"},
	}
	if err := store.UpsertDiscovered(ctx, source, "workspace-1", "confluence_page", items); err != nil {
		t.Fatalf("UpsertDiscovered() error = %v", err)
	}
	var providerID, name string
	if err := db.QueryRowContext(ctx, `SELECT provider_id, name FROM resources WHERE id = 'provider-resource'`).Scan(&providerID, &name); err != nil {
		t.Fatalf("read provider resource: %v", err)
	}
	if providerID != "123" || name != "provider wins" {
		t.Errorf("provider resource = (%q, %q), want provider-ID update", providerID, name)
	}
	if err := db.QueryRowContext(ctx, `SELECT provider_id, name FROM resources WHERE id = 'legacy-resource'`).Scan(&providerID, &name); err != nil {
		t.Fatalf("read legacy resource: %v", err)
	}
	if providerID != "456" || name != "legacy promoted" {
		t.Errorf("legacy resource = (%q, %q), want promoted identity", providerID, name)
	}
}

func TestResourceSyncStoreIsAdditiveAndRollsBackBatch(t *testing.T) {
	ctx := context.Background()
	db := openWorkspaceConfigTestDB(t)
	createWorkspaceConfigTestWorkspace(t, db, "workspace-1", "one")
	store := NewResourceSyncStore(db)
	source := resource.Source{Provider: "confluence", Name: "https://wiki.example", BaseURL: "https://wiki.example"}
	if err := store.UpsertDiscovered(ctx, source, "workspace-1", "confluence_page", []resource.Discovered{
		{ProviderID: "2", ExternalKey: "DOC/2", Name: "Beta"},
		{ProviderID: "1", ExternalKey: "DOC/1", Name: "Alpha"},
	}); err != nil {
		t.Fatalf("initial UpsertDiscovered() error = %v", err)
	}
	if err := store.UpsertDiscovered(ctx, source, "workspace-1", "confluence_page", []resource.Discovered{{ProviderID: "3", ExternalKey: "DOC/3", Name: "Gamma"}}); err != nil {
		t.Fatalf("additive UpsertDiscovered() error = %v", err)
	}
	items := listSyncedResources(t, store, "workspace-1")
	if len(items) != 3 || items[0].Resource.ExternalKey != "DOC/1" || items[2].Resource.ExternalKey != "DOC/3" {
		t.Fatalf("additive deterministic list = %#v", items)
	}

	err := store.UpsertDiscovered(ctx, source, "workspace-1", "confluence_page", []resource.Discovered{
		{ProviderID: "4", ExternalKey: "DOC/4", Name: "Would insert"},
		{ProviderID: "5", ExternalKey: "DOC/5", Name: "Invalid metadata", Metadata: `{`},
	})
	if err == nil {
		t.Fatal("UpsertDiscovered() error = nil, want metadata constraint failure")
	}
	items = listSyncedResources(t, store, "workspace-1")
	if len(items) != 3 {
		t.Fatalf("list after rollback = %#v, want original three", items)
	}
}

func TestResourceSyncStoreRejectsBlankProviderIDBeforeTransaction(t *testing.T) {
	db := openWorkspaceConfigTestDB(t)
	store := NewResourceSyncStore(db)
	if err := db.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}
	err := store.UpsertDiscovered(context.Background(), resource.Source{}, "workspace", "confluence_page", []resource.Discovered{{ExternalKey: "DOC/1"}})
	if err == nil || !strings.Contains(err.Error(), "provider ID is required") {
		t.Fatalf("UpsertDiscovered() error = %v, want provider ID validation", err)
	}
}

func TestResourceSyncStoreLocationReplacementAndConflictRollback(t *testing.T) {
	ctx := context.Background()
	db := openWorkspaceConfigTestDB(t)
	createWorkspaceConfigTestWorkspace(t, db, "workspace-1", "one")
	store := NewResourceSyncStore(db)
	source := resource.Source{Provider: "confluence", Name: "https://wiki.example", BaseURL: "https://wiki.example"}
	if err := store.UpsertDiscovered(ctx, source, "workspace-1", "confluence_page", []resource.Discovered{
		{ProviderID: "1", ExternalKey: "DOC/1", Name: "One"},
		{ProviderID: "2", ExternalKey: "DOC/2", Name: "Two"},
	}); err != nil {
		t.Fatalf("UpsertDiscovered() error = %v", err)
	}
	items := listSyncedResources(t, store, "workspace-1")
	firstID, secondID := items[0].Resource.ID, items[1].Resource.ID
	if err := store.SetLocation(ctx, "workspace-1", firstID, "materialized_file", "/wiki/one.md"); err != nil {
		t.Fatalf("first SetLocation() error = %v", err)
	}
	if err := store.SetLocation(ctx, "workspace-1", firstID, "materialized_file", "/wiki/one-new.md"); err != nil {
		t.Fatalf("replacement SetLocation() error = %v", err)
	}
	if err := store.SetLocation(ctx, "workspace-1", secondID, "materialized_file", "/wiki/two.md"); err != nil {
		t.Fatalf("second SetLocation() error = %v", err)
	}
	if err := store.SetLocation(ctx, "workspace-1", firstID, "materialized_file", "/wiki/two.md"); err == nil {
		t.Fatal("conflicting SetLocation() error = nil")
	}
	items = listSyncedResources(t, store, "workspace-1")
	paths := map[string]string{}
	for _, item := range items {
		paths[item.Resource.ID] = item.LocationPath
	}
	if paths[firstID] != "/wiki/one-new.md" || paths[secondID] != "/wiki/two.md" {
		t.Fatalf("paths after conflict = %#v, want prior locations restored", paths)
	}
}

func listSyncedResources(t *testing.T, store *ResourceSyncStore, workspaceID string) []resource.Located {
	t.Helper()
	items, err := store.ListByWorkspace(context.Background(), workspaceID, "confluence_page", "materialized_file")
	if err != nil {
		t.Fatalf("ListByWorkspace() error = %v", err)
	}
	return items
}

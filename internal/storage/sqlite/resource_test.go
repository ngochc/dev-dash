package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ngochc/dev-dash/internal/resource"
)

func TestResourceRepositoryLifecycleAndFieldPreservation(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "devdash.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	repository := NewResourceRepository(db)

	if _, err := db.ExecContext(ctx, `
		INSERT INTO integrations (id, provider, name) VALUES ('integration-1', 'github', 'work');
		INSERT INTO resources (
			id, type, integration_id, provider_id, external_key, name, url, metadata,
			created_at, updated_at, last_seen_at
		) VALUES (
			'resource-1', 'repository', 'integration-1', 'provider-42', 'org/repo', 'old',
			'https://old.test', '{"key":"value"}', '2020-01-02 03:04:05',
			'2020-01-02 03:04:05', '2021-02-03 04:05:06'
		)
	`); err != nil {
		t.Fatalf("seed full resource: %v", err)
	}

	original, err := repository.Get(ctx, "resource-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if original.IntegrationID != "integration-1" || original.ProviderID != "provider-42" || original.ExternalKey != "org/repo" || original.Metadata != `{"key":"value"}` || original.LastSeenAt == nil {
		t.Fatalf("Get() = %#v, want complete provider row", original)
	}
	oldUpdatedAt := original.UpdatedAt
	original.Type = "service"
	original.Name = "new"
	original.URL = "https://new.test"
	if err := repository.Update(ctx, original); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	updated, err := repository.Get(ctx, original.ID)
	if err != nil {
		t.Fatalf("Get(updated) error = %v", err)
	}
	if updated.Type != "service" || updated.Name != "new" || updated.URL != "https://new.test" {
		t.Errorf("Get(updated) core fields = %#v", updated)
	}
	if updated.IntegrationID != "integration-1" || updated.ProviderID != "provider-42" || updated.ExternalKey != "org/repo" || updated.Metadata != `{"key":"value"}` || updated.LastSeenAt == nil {
		t.Errorf("Get(updated) changed provider fields: %#v", updated)
	}
	if !updated.CreatedAt.Equal(original.CreatedAt) {
		t.Errorf("created_at = %v, want %v", updated.CreatedAt, original.CreatedAt)
	}
	if !updated.UpdatedAt.After(oldUpdatedAt) {
		t.Errorf("updated_at = %v, want after %v", updated.UpdatedAt, oldUpdatedAt)
	}
	if !updated.LastSeenAt.Equal(*original.LastSeenAt) {
		t.Errorf("last_seen_at = %v, want %v", updated.LastSeenAt, original.LastSeenAt)
	}

	if err := repository.Delete(ctx, original.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := repository.Get(ctx, original.ID); !errors.Is(err, resource.ErrNotFound) {
		t.Errorf("Get(deleted) error = %v, want ErrNotFound", err)
	}
}

func TestResourceRepositoryCreateListNullsAndOrdering(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "devdash.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	repository := NewResourceRepository(db)

	items := []resource.Resource{
		{ID: "2", Type: "service", Name: "beta"},
		{ID: "3", Type: "service", Name: "alpha", URL: "https://example.test"},
		{ID: "1", Type: "service", Name: "alpha"},
	}
	for _, item := range items {
		if err := repository.Create(ctx, item); err != nil {
			t.Fatalf("Create(%s) error = %v", item.ID, err)
		}
	}
	got, err := repository.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 3 || got[0].ID != "1" || got[1].ID != "3" || got[2].ID != "2" {
		t.Fatalf("List() = %#v, want name then ID order", got)
	}
	if got[0].IntegrationID != "" || got[0].ProviderID != "" || got[0].ExternalKey != "" || got[0].URL != "" || got[0].Metadata != "" || got[0].LastSeenAt != nil {
		t.Errorf("List() nullable fields = %#v, want zero values", got[0])
	}
	if got[0].CreatedAt.IsZero() || got[0].UpdatedAt.IsZero() {
		t.Errorf("List() timestamps = %v, %v; want generated values", got[0].CreatedAt, got[0].UpdatedAt)
	}
}

func TestResourceRepositoryMissingAndForeignKey(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "devdash.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	repository := NewResourceRepository(db)

	if err := repository.Create(ctx, resource.Resource{ID: "bad", Type: "missing", Name: "bad"}); err == nil {
		t.Fatal("Create(unknown type) error = nil, want foreign-key error")
	}
	if _, err := repository.Get(ctx, "missing"); !errors.Is(err, resource.ErrNotFound) {
		t.Errorf("Get() error = %v, want ErrNotFound", err)
	}
	if err := repository.Update(ctx, resource.Resource{ID: "missing", Type: "service", Name: "missing"}); !errors.Is(err, resource.ErrNotFound) {
		t.Errorf("Update() error = %v, want ErrNotFound", err)
	}
	if err := repository.Delete(ctx, "missing"); !errors.Is(err, resource.ErrNotFound) {
		t.Errorf("Delete() error = %v, want ErrNotFound", err)
	}
}

func TestResourceRepositoryScansLastSeenTime(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "devdash.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	expected := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)
	if _, err := db.ExecContext(ctx, "INSERT INTO resources (id, type, name, last_seen_at) VALUES (?, ?, ?, ?)", "seen", "service", "seen", expected); err != nil {
		t.Fatalf("insert resource: %v", err)
	}
	got, err := NewResourceRepository(db).Get(ctx, "seen")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.LastSeenAt == nil || !got.LastSeenAt.Equal(expected) {
		t.Errorf("LastSeenAt = %v, want %v", got.LastSeenAt, expected)
	}
}

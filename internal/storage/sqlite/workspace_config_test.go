package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ngochc/dev-dash/internal/workspace"
)

func TestWorkspaceConfigRepositoryLifecycleAndIsolation(t *testing.T) {
	ctx := context.Background()
	db := openWorkspaceConfigTestDB(t)
	repository := NewWorkspaceConfigRepository(db)
	createWorkspaceConfigTestWorkspace(t, db, "workspace-1", "devdash")
	createWorkspaceConfigTestWorkspace(t, db, "workspace-2", "other")

	for _, entry := range []workspace.ConfigEntry{
		{Namespace: "jira", Key: "project", Value: "DD"},
		{Namespace: "github", Key: "org", Value: "acme"},
		{Namespace: "github", Key: "base_url", Value: "https://github.example"},
	} {
		if err := repository.Set(ctx, "workspace-1", entry); err != nil {
			t.Fatalf("Set(%s) error = %v", entry.FullKey(), err)
		}
	}
	if err := repository.Set(ctx, "workspace-2", workspace.ConfigEntry{Namespace: "github", Key: "org", Value: "other"}); err != nil {
		t.Fatalf("Set(other workspace) error = %v", err)
	}

	entry, err := repository.Get(ctx, "workspace-1", "github", "org")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if entry != (workspace.ConfigEntry{Namespace: "github", Key: "org", Value: "acme"}) {
		t.Errorf("Get() = %#v", entry)
	}

	entries, err := repository.List(ctx, "workspace-1")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	want := []workspace.ConfigEntry{
		{Namespace: "github", Key: "base_url", Value: "https://github.example"},
		{Namespace: "github", Key: "org", Value: "acme"},
		{Namespace: "jira", Key: "project", Value: "DD"},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("List() = %#v, want %#v", entries, want)
	}

	githubEntries, err := repository.ListNamespace(ctx, "workspace-1", "github")
	if err != nil {
		t.Fatalf("ListNamespace() error = %v", err)
	}
	if !reflect.DeepEqual(githubEntries, want[:2]) {
		t.Errorf("ListNamespace() = %#v, want %#v", githubEntries, want[:2])
	}

	otherEntries, err := repository.List(ctx, "workspace-2")
	if err != nil {
		t.Fatalf("List(other workspace) error = %v", err)
	}
	if len(otherEntries) != 1 || otherEntries[0].Value != "other" {
		t.Errorf("List(other workspace) = %#v, want isolated entry", otherEntries)
	}

	if err := repository.Unset(ctx, "workspace-1", "github", "org"); err != nil {
		t.Fatalf("Unset() error = %v", err)
	}
	if _, err := repository.Get(ctx, "workspace-1", "github", "org"); !errors.Is(err, workspace.ErrConfigNotFound) {
		t.Errorf("Get(removed) error = %v, want ErrConfigNotFound", err)
	}
	if err := repository.Unset(ctx, "workspace-1", "github", "org"); !errors.Is(err, workspace.ErrConfigNotFound) {
		t.Errorf("second Unset() error = %v, want ErrConfigNotFound", err)
	}
}

func TestWorkspaceConfigRepositoryUpsertPreservesCreatedAt(t *testing.T) {
	ctx := context.Background()
	db := openWorkspaceConfigTestDB(t)
	repository := NewWorkspaceConfigRepository(db)
	createWorkspaceConfigTestWorkspace(t, db, "workspace-1", "devdash")
	entry := workspace.ConfigEntry{Namespace: "github", Key: "org", Value: "old"}
	if err := repository.Set(ctx, "workspace-1", entry); err != nil {
		t.Fatalf("initial Set() error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		UPDATE workspace_config
		SET created_at = '2000-01-01 00:00:00', updated_at = '2001-01-01 00:00:00'
		WHERE workspace_id = 'workspace-1' AND namespace = 'github' AND key = 'org'
	`); err != nil {
		t.Fatalf("set historical timestamps: %v", err)
	}

	entry.Value = "new"
	if err := repository.Set(ctx, "workspace-1", entry); err != nil {
		t.Fatalf("updated Set() error = %v", err)
	}
	var value, createdAt, updatedAt string
	if err := db.QueryRowContext(ctx, `
		SELECT value, CAST(created_at AS TEXT), CAST(updated_at AS TEXT)
		FROM workspace_config
		WHERE workspace_id = 'workspace-1' AND namespace = 'github' AND key = 'org'
	`).Scan(&value, &createdAt, &updatedAt); err != nil {
		t.Fatalf("query timestamps: %v", err)
	}
	if value != "new" {
		t.Errorf("value = %q, want new", value)
	}
	if createdAt != "2000-01-01 00:00:00" {
		t.Errorf("created_at = %q, want preserved historical timestamp", createdAt)
	}
	if updatedAt == "2001-01-01 00:00:00" {
		t.Errorf("updated_at = %q, want advanced timestamp", updatedAt)
	}
}

func TestWorkspaceConfigRepositoryRejectsUnknownWorkspace(t *testing.T) {
	db := openWorkspaceConfigTestDB(t)
	err := NewWorkspaceConfigRepository(db).Set(context.Background(), "missing", workspace.ConfigEntry{
		Namespace: "github",
		Key:       "org",
		Value:     "acme",
	})
	if err == nil {
		t.Fatal("Set() error = nil, want foreign-key error")
	}
}

func TestWorkspaceConfigRepositoryReplaceAll(t *testing.T) {
	ctx := context.Background()
	db := openWorkspaceConfigTestDB(t)
	repository := NewWorkspaceConfigRepository(db)
	createWorkspaceConfigTestWorkspace(t, db, "workspace-1", "devdash")
	createWorkspaceConfigTestWorkspace(t, db, "workspace-2", "other")
	for _, entry := range []workspace.ConfigEntry{
		{Namespace: "github", Key: "base_url", Value: "old-url"},
		{Namespace: "github", Key: "org", Value: "old-org"},
		{Namespace: "jira", Key: "project", Value: "stale"},
	} {
		if err := repository.Set(ctx, "workspace-1", entry); err != nil {
			t.Fatalf("seed Set(%s) error = %v", entry.FullKey(), err)
		}
	}
	if err := repository.Set(ctx, "workspace-2", workspace.ConfigEntry{Namespace: "github", Key: "org", Value: "other"}); err != nil {
		t.Fatalf("seed other workspace: %v", err)
	}

	replacement := []workspace.ConfigEntry{
		{Namespace: "github", Key: "org", Value: "new-org"},
		{Namespace: "confluence", Key: "space", Value: "MQMS"},
	}
	if err := repository.ReplaceAll(ctx, "workspace-1", replacement); err != nil {
		t.Fatalf("ReplaceAll() error = %v", err)
	}
	entries, err := repository.List(ctx, "workspace-1")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	want := []workspace.ConfigEntry{
		{Namespace: "confluence", Key: "space", Value: "MQMS"},
		{Namespace: "github", Key: "org", Value: "new-org"},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("List() after replacement = %#v, want %#v", entries, want)
	}
	other, err := repository.List(ctx, "workspace-2")
	if err != nil {
		t.Fatalf("List(other) error = %v", err)
	}
	if len(other) != 1 || other[0].Value != "other" {
		t.Errorf("other workspace config = %#v, want untouched", other)
	}

	if err := repository.ReplaceAll(ctx, "workspace-1", nil); err != nil {
		t.Fatalf("empty ReplaceAll() error = %v", err)
	}
	entries, err = repository.List(ctx, "workspace-1")
	if err != nil {
		t.Fatalf("List() after empty replacement error = %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("List() after empty replacement = %#v, want empty", entries)
	}
	other, err = repository.List(ctx, "workspace-2")
	if err != nil || len(other) != 1 {
		t.Errorf("other workspace after empty replacement = %#v, %v", other, err)
	}
}

func TestWorkspaceConfigRepositoryReplaceAllRollsBack(t *testing.T) {
	ctx := context.Background()
	db := openWorkspaceConfigTestDB(t)
	repository := NewWorkspaceConfigRepository(db)
	createWorkspaceConfigTestWorkspace(t, db, "workspace-1", "devdash")
	old := []workspace.ConfigEntry{
		{Namespace: "github", Key: "org", Value: "old-org"},
		{Namespace: "jira", Key: "project", Value: "old-project"},
	}
	for _, entry := range old {
		if err := repository.Set(ctx, "workspace-1", entry); err != nil {
			t.Fatalf("seed Set(%s) error = %v", entry.FullKey(), err)
		}
	}
	if _, err := db.ExecContext(ctx, `
		CREATE TRIGGER reject_workspace_config
		BEFORE INSERT ON workspace_config
		WHEN NEW.namespace = 'trigger' AND NEW.key = 'fail'
		BEGIN
			SELECT RAISE(ABORT, 'replacement rejected');
		END
	`); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}

	err := repository.ReplaceAll(ctx, "workspace-1", []workspace.ConfigEntry{
		{Namespace: "github", Key: "org", Value: "partially-updated"},
		{Namespace: "trigger", Key: "fail", Value: "failure"},
	})
	if err == nil {
		t.Fatal("ReplaceAll() error = nil, want trigger failure")
	}
	entries, listErr := repository.List(ctx, "workspace-1")
	if listErr != nil {
		t.Fatalf("List() after failed replacement error = %v", listErr)
	}
	if !reflect.DeepEqual(entries, old) {
		t.Errorf("List() after failed replacement = %#v, want original %#v", entries, old)
	}
}

func TestWorkspaceConfigRepositoryReplaceUserPreservesInternal(t *testing.T) {
	ctx := context.Background()
	db := openWorkspaceConfigTestDB(t)
	repository := NewWorkspaceConfigRepository(db)
	createWorkspaceConfigTestWorkspace(t, db, "workspace-1", "devdash")
	for _, entry := range []workspace.ConfigEntry{
		{Namespace: "_repo", Key: "last_refresh", Value: "internal"},
		{Namespace: "github", Key: "org", Value: "old"},
	} {
		if err := repository.Set(ctx, "workspace-1", entry); err != nil {
			t.Fatalf("Set(%s) error = %v", entry.FullKey(), err)
		}
	}

	if err := repository.ReplaceUser(ctx, "workspace-1", []workspace.ConfigEntry{{Namespace: "github", Key: "org", Value: "new"}}); err != nil {
		t.Fatalf("ReplaceUser() error = %v", err)
	}
	entries, err := repository.List(ctx, "workspace-1")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	want := []workspace.ConfigEntry{
		{Namespace: "_repo", Key: "last_refresh", Value: "internal"},
		{Namespace: "github", Key: "org", Value: "new"},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("List() after user replacement = %#v, want %#v", entries, want)
	}

	if err := repository.ReplaceUser(ctx, "workspace-1", nil); err != nil {
		t.Fatalf("empty ReplaceUser() error = %v", err)
	}
	entries, err = repository.List(ctx, "workspace-1")
	if err != nil {
		t.Fatalf("List() after empty user replacement error = %v", err)
	}
	if len(entries) != 1 || entries[0].FullKey() != "_repo.last_refresh" {
		t.Errorf("List() after empty user replacement = %#v, want internal entry", entries)
	}
}

func TestWorkspaceConfigRepositoryWorkspaceDeleteCascades(t *testing.T) {
	ctx := context.Background()
	db := openWorkspaceConfigTestDB(t)
	repository := NewWorkspaceConfigRepository(db)
	createWorkspaceConfigTestWorkspace(t, db, "workspace-1", "devdash")
	if err := repository.Set(ctx, "workspace-1", workspace.ConfigEntry{Namespace: "github", Key: "org", Value: "acme"}); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM workspaces WHERE id = ?", "workspace-1"); err != nil {
		t.Fatalf("delete workspace: %v", err)
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM workspace_config WHERE workspace_id = ?", "workspace-1").Scan(&count); err != nil {
		t.Fatalf("query config count: %v", err)
	}
	if count != 0 {
		t.Errorf("workspace config count = %d, want 0", count)
	}
}

func openWorkspaceConfigTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "devdash.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func createWorkspaceConfigTestWorkspace(t *testing.T, db *sql.DB, id, name string) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO workspaces (id, name, local_path) VALUES (?, ?, ?)
	`, id, name, "/"+name); err != nil {
		t.Fatalf("create workspace %q: %v", name, err)
	}
}

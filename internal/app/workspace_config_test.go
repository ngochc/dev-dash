package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ngochc/dev-dash/internal/storage/sqlite"
	"github.com/ngochc/dev-dash/internal/workspace"
)

func TestFormatWorkspaceConfigSortsWithoutMutating(t *testing.T) {
	entries := []workspace.ConfigEntry{
		{Namespace: "jira", Key: "project", Value: "DD"},
		{Namespace: "github", Key: "org", Value: "acme"},
		{Namespace: "github", Key: "base_url", Value: "https://github.example"},
	}
	got := string(formatWorkspaceConfig(entries))
	want := "github.base_url=https://github.example\ngithub.org=acme\njira.project=DD\n"
	if got != want {
		t.Errorf("formatWorkspaceConfig() = %q, want %q", got, want)
	}
	if entries[0].FullKey() != "jira.project" {
		t.Error("formatWorkspaceConfig() mutated caller entries")
	}
}

func TestParseWorkspaceConfig(t *testing.T) {
	data := []byte("\n # comment\ngithub.api.version = v3\nsome.query=a=b=c\n\t\n")
	entries, err := parseWorkspaceConfig(data)
	if err != nil {
		t.Fatalf("parseWorkspaceConfig() error = %v", err)
	}
	want := []workspace.ConfigEntry{
		{Namespace: "github", Key: "api.version", Value: "v3"},
		{Namespace: "some", Key: "query", Value: "a=b=c"},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("parseWorkspaceConfig() = %#v, want %#v", entries, want)
	}
}

func TestParseWorkspaceConfigErrors(t *testing.T) {
	for _, test := range []struct {
		name    string
		data    string
		wantErr string
	}{
		{name: "missing delimiter", data: "# first\ngithub.org\n", wantErr: "invalid workspace config at line 2: expected key=value"},
		{name: "empty key", data: " =value\n", wantErr: "invalid workspace config at line 1: expected key=value"},
		{name: "empty value", data: "github.org= \n", wantErr: "invalid workspace config at line 1: value is required"},
		{name: "invalid key", data: "github/org=value\n", wantErr: `invalid workspace config at line 1: invalid workspace config key "github/org"`},
		{name: "duplicate", data: "github.org=first\n# between\ngithub.org=second\n", wantErr: `invalid workspace config at line 3: duplicate key "github.org"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			entries, err := parseWorkspaceConfig([]byte(test.data))
			if err == nil || err.Error() != test.wantErr {
				t.Fatalf("parseWorkspaceConfig() error = %v, want %q", err, test.wantErr)
			}
			if entries != nil {
				t.Errorf("parseWorkspaceConfig() entries = %#v, want nil on error", entries)
			}
		})
	}
}

func TestRunWorkspaceConfigLifecycleByNameAndID(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "devdash.db")
	t.Setenv("DEVDASH_DB", databasePath)
	ctx := context.Background()
	runAppCommand(t, ctx, "workspace", "add", "devdash", t.TempDir())
	show := runAppCommand(t, ctx, "workspace", "show", "devdash")
	workspaceID := strings.TrimSpace(strings.TrimPrefix(strings.Split(show, "\n")[0], "ID:"))
	if workspaceID == "" {
		t.Fatalf("workspace show output = %q, want ID", show)
	}

	if got := runAppCommand(t, ctx, "workspace", "config", "list", "devdash"); got != "No workspace config found.\n" {
		t.Errorf("empty list output = %q", got)
	}
	if got := runAppCommand(t, ctx, "workspace", "config", "set", "devdash", "github.org", "acme"); got != "Workspace config updated: github.org\n" {
		t.Errorf("set output = %q", got)
	}
	if got := runAppCommand(t, ctx, "workspace", "config", "list", "devdash"); got != "github.org   acme\n" {
		t.Errorf("single-entry list output = %q", got)
	}
	if got := runAppCommand(t, ctx, "workspace", "config", "set", workspaceID, "github.org", "new-org"); got != "Workspace config updated: github.org\n" {
		t.Errorf("update output = %q", got)
	}
	if got := runAppCommand(t, ctx, "workspace", "config", "set", workspaceID, "github.secret", "secret:github-work"); got != "Workspace config updated: github.secret\n" {
		t.Errorf("secret reference set output = %q", got)
	}
	if got := runAppCommand(t, ctx, "workspace", "config", "set", "devdash", "some.query", "a=b=c"); got != "Workspace config updated: some.query\n" {
		t.Errorf("equals value set output = %q", got)
	}
	if got := runAppCommand(t, ctx, "workspace", "config", "get", workspaceID, "github.org"); got != "new-org\n" {
		t.Errorf("get output = %q", got)
	}
	if got := runAppCommand(t, ctx, "workspace", "config", "get", "devdash", "github.secret"); got != "secret:github-work\n" {
		t.Errorf("secret reference get output = %q", got)
	}
	if got := runAppCommand(t, ctx, "workspace", "config", "get", "devdash", "some.query"); got != "a=b=c\n" {
		t.Errorf("equals value get output = %q", got)
	}

	fields := strings.Fields(runAppCommand(t, ctx, "workspace", "config", "list", workspaceID))
	wantFields := []string{"github.org", "new-org", "github.secret", "secret:github-work", "some.query", "a=b=c"}
	if !reflect.DeepEqual(fields, wantFields) {
		t.Errorf("list fields = %v, want ordered %v", fields, wantFields)
	}
	if got := runAppCommand(t, ctx, "workspace", "config", "unset", workspaceID, "github.org"); got != "Workspace config removed: github.org\n" {
		t.Errorf("unset output = %q", got)
	}
	_, err := runAppCommandError(ctx, "workspace", "config", "unset", "devdash", "github.org")
	if !errors.Is(err, workspace.ErrConfigNotFound) || err.Error() != `workspace config "github.org" not found` {
		t.Fatalf("second unset error = %v, want exact config not found", err)
	}
}

func TestRunWorkspaceConfigMissingErrors(t *testing.T) {
	t.Setenv("DEVDASH_DB", filepath.Join(t.TempDir(), "devdash.db"))
	ctx := context.Background()
	runAppCommand(t, ctx, "workspace", "add", "devdash", t.TempDir())

	_, err := runAppCommandError(ctx, "workspace", "config", "list", "missing")
	if !errors.Is(err, workspace.ErrNotFound) || err.Error() != `workspace "missing" not found` {
		t.Fatalf("missing workspace error = %v, want exact not found", err)
	}
	_, err = runAppCommandError(ctx, "workspace", "config", "get", "devdash", "github.org")
	if !errors.Is(err, workspace.ErrConfigNotFound) || err.Error() != `workspace config "github.org" not found` {
		t.Fatalf("missing config error = %v, want exact not found", err)
	}
}

func TestRunWorkspaceConfigEditReplacesCompleteSet(t *testing.T) {
	t.Setenv("DEVDASH_DB", filepath.Join(t.TempDir(), "devdash.db"))
	ctx := context.Background()
	runAppCommand(t, ctx, "workspace", "add", "devdash", t.TempDir())
	runAppCommand(t, ctx, "workspace", "config", "set", "devdash", "github.base_url", "https://old.example")
	runAppCommand(t, ctx, "workspace", "config", "set", "devdash", "github.org", "old-org")

	input := strings.NewReader("editor input")
	var output bytes.Buffer
	editorCalls := 0
	editor := func(gotCtx context.Context, initial []byte, gotInput io.Reader, gotOutput io.Writer) ([]byte, error) {
		editorCalls++
		if gotCtx != ctx || gotInput != input || gotOutput != &output {
			t.Error("editor did not receive command context and streams")
		}
		wantInitial := "github.base_url=https://old.example\ngithub.org=old-org\n"
		if string(initial) != wantInitial {
			t.Errorf("editor initial content = %q, want %q", initial, wantInitial)
		}
		return []byte("# replacement\ngithub.org=new-org\nconfluence.space=MQMS\nsome.query=a=b=c\n"), nil
	}
	if err := runWorkspaceConfigWithEditor(ctx, []string{"edit", "devdash"}, input, &output, editor); err != nil {
		t.Fatalf("runWorkspaceConfigWithEditor() error = %v", err)
	}
	if editorCalls != 1 {
		t.Errorf("editor calls = %d, want 1", editorCalls)
	}
	if got := output.String(); got != "Workspace config updated: devdash\n" {
		t.Errorf("edit output = %q", got)
	}
	fields := strings.Fields(runAppCommand(t, ctx, "workspace", "config", "list", "devdash"))
	wantFields := []string{"confluence.space", "MQMS", "github.org", "new-org", "some.query", "a=b=c"}
	if !reflect.DeepEqual(fields, wantFields) {
		t.Errorf("edited list fields = %v, want %v", fields, wantFields)
	}
	_, err := runAppCommandError(ctx, "workspace", "config", "get", "devdash", "github.base_url")
	if !errors.Is(err, workspace.ErrConfigNotFound) {
		t.Errorf("removed edited key error = %v, want ErrConfigNotFound", err)
	}
}

func TestRunWorkspaceConfigEditFailurePreservesRows(t *testing.T) {
	for _, test := range []struct {
		name    string
		edited  string
		editErr error
		wantErr string
	}{
		{name: "malformed", edited: "github.org\n", wantErr: "invalid workspace config at line 1: expected key=value"},
		{name: "duplicate", edited: "github.org=one\ngithub.org=two\n", wantErr: `invalid workspace config at line 2: duplicate key "github.org"`},
		{name: "editor", editErr: errors.New("editor failed"), wantErr: "editor failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("DEVDASH_DB", filepath.Join(t.TempDir(), "devdash.db"))
			ctx := context.Background()
			runAppCommand(t, ctx, "workspace", "add", "devdash", t.TempDir())
			runAppCommand(t, ctx, "workspace", "config", "set", "devdash", "github.org", "old-org")
			var output bytes.Buffer
			editor := func(context.Context, []byte, io.Reader, io.Writer) ([]byte, error) {
				return []byte(test.edited), test.editErr
			}
			err := runWorkspaceConfigWithEditor(ctx, []string{"edit", "devdash"}, strings.NewReader(""), &output, editor)
			if err == nil || err.Error() != test.wantErr {
				t.Fatalf("edit error = %v, want %q", err, test.wantErr)
			}
			if output.Len() != 0 {
				t.Errorf("failed edit output = %q, want empty", output.String())
			}
			if got := runAppCommand(t, ctx, "workspace", "config", "get", "devdash", "github.org"); got != "old-org\n" {
				t.Errorf("value after failed edit = %q, want old value", got)
			}
		})
	}
}

func TestRunWorkspaceConfigReservedKeysAndEditPreservation(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "devdash.db")
	t.Setenv("DEVDASH_DB", databasePath)
	ctx := context.Background()
	runAppCommand(t, ctx, "workspace", "add", "devdash", t.TempDir())
	runAppCommand(t, ctx, "workspace", "config", "set", "devdash", "github.org", "old")

	for _, command := range []string{"set", "unset"} {
		args := []string{"workspace", "config", command, "devdash", "_repo.test"}
		if command == "set" {
			args = append(args, "value")
		}
		_, err := runAppCommandError(ctx, args...)
		if err == nil || err.Error() != `config key "_repo.test" is reserved for internal use` {
			t.Fatalf("%s reserved key error = %v", command, err)
		}
	}

	show := runAppCommand(t, ctx, "workspace", "show", "devdash")
	workspaceID := strings.TrimSpace(strings.TrimPrefix(strings.Split(show, "\n")[0], "ID:"))
	db, err := sqlite.Open(ctx, databasePath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := sqlite.NewWorkspaceConfigRepository(db).Set(ctx, workspaceID, workspace.ConfigEntry{Namespace: "_repo", Key: "last_refresh", Value: "internal"}); err != nil {
		db.Close()
		t.Fatalf("set internal config: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}

	if got := runAppCommand(t, ctx, "workspace", "config", "list", "devdash"); got != "github.org   old\n" {
		t.Errorf("user config list = %q, want internal key hidden", got)
	}
	var output bytes.Buffer
	editor := func(_ context.Context, initial []byte, _ io.Reader, _ io.Writer) ([]byte, error) {
		if got := string(initial); got != "github.org=old\n" {
			t.Errorf("editor initial config = %q, want only user config", got)
		}
		return []byte("github.org=new\n"), nil
	}
	if err := runWorkspaceConfigWithEditor(ctx, []string{"edit", "devdash"}, strings.NewReader(""), &output, editor); err != nil {
		t.Fatalf("edit config: %v", err)
	}

	db, err = sqlite.Open(ctx, databasePath)
	if err != nil {
		t.Fatalf("reopen database: %v", err)
	}
	defer db.Close()
	repository := sqlite.NewWorkspaceConfigRepository(db)
	internal, err := repository.Get(ctx, workspaceID, "_repo", "last_refresh")
	if err != nil || internal.Value != "internal" {
		t.Errorf("internal config after edit = %#v, %v", internal, err)
	}
	user, err := repository.Get(ctx, workspaceID, "github", "org")
	if err != nil || user.Value != "new" {
		t.Errorf("user config after edit = %#v, %v", user, err)
	}
}

func TestRunWorkspaceConfigValidatesBeforeDatabaseOpen(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "missing command", args: []string{"workspace", "config"}, wantErr: "workspace config command required: edit, get, list, set, or unset"},
		{name: "unknown command", args: []string{"workspace", "config", "unknown"}, wantErr: "unknown workspace config command: unknown"},
		{name: "list missing workspace", args: []string{"workspace", "config", "list"}, wantErr: "usage: devdash workspace config list <workspace>"},
		{name: "list extra argument", args: []string{"workspace", "config", "list", "ws", "extra"}, wantErr: "usage: devdash workspace config list <workspace>"},
		{name: "get missing key", args: []string{"workspace", "config", "get", "ws"}, wantErr: "usage: devdash workspace config get <workspace> <key>"},
		{name: "get extra argument", args: []string{"workspace", "config", "get", "ws", "key", "extra"}, wantErr: "usage: devdash workspace config get <workspace> <key>"},
		{name: "set missing value", args: []string{"workspace", "config", "set", "ws", "key"}, wantErr: "usage: devdash workspace config set <workspace> <key> <value>"},
		{name: "set extra argument", args: []string{"workspace", "config", "set", "ws", "key", "value", "extra"}, wantErr: "usage: devdash workspace config set <workspace> <key> <value>"},
		{name: "unset missing key", args: []string{"workspace", "config", "unset", "ws"}, wantErr: "usage: devdash workspace config unset <workspace> <key>"},
		{name: "unset extra argument", args: []string{"workspace", "config", "unset", "ws", "key", "extra"}, wantErr: "usage: devdash workspace config unset <workspace> <key>"},
		{name: "edit missing workspace", args: []string{"workspace", "config", "edit"}, wantErr: "usage: devdash workspace config edit <workspace>"},
		{name: "edit extra argument", args: []string{"workspace", "config", "edit", "ws", "extra"}, wantErr: "usage: devdash workspace config edit <workspace>"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			databasePath := filepath.Join(t.TempDir(), "devdash.db")
			t.Setenv("DEVDASH_DB", databasePath)
			var output bytes.Buffer
			err := run(context.Background(), test.args, strings.NewReader(""), &output)
			if err == nil || err.Error() != test.wantErr {
				t.Fatalf("run() error = %v, want %q", err, test.wantErr)
			}
			if _, statErr := os.Stat(databasePath); !os.IsNotExist(statErr) {
				t.Fatalf("database stat error = %v, want file not created", statErr)
			}
		})
	}
}

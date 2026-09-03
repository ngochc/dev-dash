package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ngochc/dev-dash/internal/secret"
	"github.com/ngochc/dev-dash/internal/storage/sqlite"
	"github.com/ngochc/dev-dash/internal/workspace"
)

func TestRunTopLevelCommands(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "devdash.db")
	t.Setenv("DEVDASH_DB", databasePath)

	tests := []struct {
		name       string
		args       []string
		wantOutput string
		wantError  string
	}{
		{name: "root", wantOutput: "devdash\n"},
		{name: "help", args: []string{"help"}, wantOutput: "devdash update"},
		{name: "short help", args: []string{"-h"}, wantOutput: "Usage:"},
		{name: "long help", args: []string{"--help"}, wantOutput: "Usage:"},
		{name: "doctor", args: []string{"doctor"}, wantOutput: "migration  OK"},
		{name: "unknown", args: []string{"unknown"}, wantError: "unknown command: unknown"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			err := run(context.Background(), test.args, strings.NewReader(""), &output)
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("run() error = %v, want containing %q", err, test.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("run() error = %v", err)
			}
			if !strings.Contains(output.String(), test.wantOutput) {
				t.Errorf("run() output = %q, want containing %q", output.String(), test.wantOutput)
			}
		})
	}
}
func TestRunUpdate(t *testing.T) {
	ctx := context.Background()
	var output bytes.Buffer
	wantErr := errors.New("update failed")
	calls := 0

	updater := func(gotCtx context.Context, gotOutput io.Writer) error {
		calls++
		if gotCtx != ctx {
			t.Errorf("update context differs from run context")
		}
		if gotOutput != &output {
			t.Errorf("update output = %v, want run output", gotOutput)
		}
		fmt.Fprint(gotOutput, "updater output")
		return wantErr
	}

	err := runWithUpdater(ctx, []string{"update"}, strings.NewReader(""), &output, updater)
	if !errors.Is(err, wantErr) {
		t.Fatalf("runWithUpdater() error = %v, want %v", err, wantErr)
	}
	if calls != 1 {
		t.Errorf("update calls = %d, want 1", calls)
	}
	if got := output.String(); got != "updater output" {
		t.Errorf("runWithUpdater() output = %q, want updater output", got)
	}
}

func TestRunUpdateRejectsArguments(t *testing.T) {
	calls := 0
	updater := func(context.Context, io.Writer) error {
		calls++
		return nil
	}

	err := runWithUpdater(context.Background(), []string{"update", "--help"}, strings.NewReader(""), io.Discard, updater)
	if err == nil || err.Error() != "usage: devdash update" {
		t.Fatalf("runWithUpdater() error = %v, want usage error", err)
	}
	if calls != 0 {
		t.Errorf("update calls = %d, want 0", calls)
	}
}

func TestRunWorkspaceLifecycle(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "devdash.db")
	workspacePath := t.TempDir()
	t.Setenv("DEVDASH_DB", databasePath)
	ctx := context.Background()

	runCommand := func(args ...string) string {
		t.Helper()
		var output bytes.Buffer
		if err := run(ctx, args, strings.NewReader(""), &output); err != nil {
			t.Fatalf("run(%v) error = %v", args, err)
		}
		return output.String()
	}

	if got := runCommand("workspace", "list"); got != "No workspaces found.\n" {
		t.Errorf("empty list output = %q", got)
	}
	if got := runCommand("workspace", "add", "devdash", workspacePath); got != "Workspace added: devdash\n" {
		t.Errorf("add output = %q", got)
	}

	listOutput := runCommand("workspace", "list")
	if !strings.Contains(listOutput, "NAME") || !strings.Contains(listOutput, "PATH") || !strings.Contains(listOutput, "devdash") || !strings.Contains(listOutput, workspacePath) {
		t.Errorf("list output = %q, want workspace table", listOutput)
	}

	showOutput := runCommand("workspace", "show", "devdash")
	for _, expected := range []string{"ID:   ", "Name: devdash", "Path: " + workspacePath} {
		if !strings.Contains(showOutput, expected) {
			t.Errorf("show output = %q, want containing %q", showOutput, expected)
		}
	}

	if got := runCommand("workspace", "remove", "devdash"); got != "Workspace removed: devdash\n" {
		t.Errorf("remove output = %q", got)
	}
	if got := runCommand("workspace", "list"); got != "No workspaces found.\n" {
		t.Errorf("final list output = %q", got)
	}
}

func TestRunWorkspaceErrors(t *testing.T) {
	t.Setenv("DEVDASH_DB", filepath.Join(t.TempDir(), "devdash.db"))
	missingPath := filepath.Join(t.TempDir(), "missing")

	tests := []struct {
		name      string
		args      []string
		wantError string
	}{
		{name: "missing subcommand", args: []string{"workspace"}, wantError: "workspace command required"},
		{name: "unknown subcommand", args: []string{"workspace", "unknown"}, wantError: "unknown workspace command"},
		{name: "list arguments", args: []string{"workspace", "list", "extra"}, wantError: "usage: devdash workspace list"},
		{name: "add missing name", args: []string{"workspace", "add"}, wantError: "usage: devdash workspace add"},
		{name: "add extra arguments", args: []string{"workspace", "add", "name", "/tmp", "extra"}, wantError: "usage: devdash workspace add"},
		{name: "show missing identifier", args: []string{"workspace", "show"}, wantError: "usage: devdash workspace show"},
		{name: "show missing workspace", args: []string{"workspace", "show", "missing"}, wantError: "workspace not found"},
		{name: "remove missing identifier", args: []string{"workspace", "remove"}, wantError: "usage: devdash workspace remove"},
		{name: "remove missing workspace", args: []string{"workspace", "remove", "missing"}, wantError: "workspace not found"},
		{name: "invalid directory", args: []string{"workspace", "add", "devdash", missingPath}, wantError: "stat workspace directory"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			err := run(context.Background(), test.args, strings.NewReader(""), &output)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("run() error = %v, want containing %q", err, test.wantError)
			}
		})
	}
}

func TestPrintWorkspaceList(t *testing.T) {
	var output bytes.Buffer
	printWorkspaceList(&output, []workspace.Workspace{
		{Name: "devdash", LocalPath: "/work/devdash"},
		{Name: "frontend", LocalPath: "/work/frontend"},
	})

	got := output.String()
	for _, expected := range []string{"NAME", "PATH", "devdash", "/work/devdash", "frontend", "/work/frontend"} {
		if !strings.Contains(got, expected) {
			t.Errorf("printWorkspaceList() output = %q, want containing %q", got, expected)
		}
	}
}

func TestRunSecretLifecycle(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "devdash.db")
	t.Setenv("DEVDASH_DB", databasePath)
	ctx := context.Background()

	runCommand := func(input string, args ...string) (string, error) {
		var output bytes.Buffer
		err := run(ctx, args, strings.NewReader(input), &output)
		return output.String(), err
	}

	output, err := runCommand("", "secret", "list")
	if err != nil {
		t.Fatalf("empty secret list error = %v", err)
	}
	if output != "No secrets found.\n" {
		t.Error("empty secret list output did not match")
	}

	const (
		firstValue       = "ghp_12345678"
		secondValue      = "second-sensitive-value"
		replacementValue = "123456789"
	)
	output, err = runCommand(firstValue, "secret", "set", "zeta")
	if err != nil {
		t.Fatalf("set zeta error = %v", err)
	}
	if output != "Secret stored: zeta\n" {
		t.Error("set zeta output did not match")
	}
	if strings.Contains(output, firstValue) {
		t.Error("set output disclosed the secret value")
	}

	output, err = runCommand(secondValue, "secret", "set", "alpha")
	if err != nil {
		t.Fatalf("set alpha error = %v", err)
	}
	if output != "Secret stored: alpha\n" {
		t.Error("set alpha output did not match")
	}
	if strings.Contains(output, secondValue) {
		t.Error("set output disclosed the secret value")
	}

	output, err = runCommand(replacementValue, "secret", "set", "zeta")
	if err != nil {
		t.Fatalf("replace zeta error = %v", err)
	}
	if output != "Secret stored: zeta\n" {
		t.Error("replace output did not match")
	}
	if strings.Contains(output, replacementValue) {
		t.Error("replace output disclosed the secret value")
	}

	output, err = runCommand("", "secret", "list")
	if err != nil {
		t.Fatalf("secret list error = %v", err)
	}
	if output != "alpha\nzeta\n" {
		t.Error("secret list was not alphabetical and key-only")
	}
	if strings.Contains(output, secondValue) || strings.Contains(output, replacementValue) {
		t.Error("list output disclosed a secret value")
	}

	output, err = runCommand("", "secret", "show", "zeta")
	if err != nil {
		t.Fatalf("show zeta error = %v", err)
	}
	if output != "zeta  1234…6789\n" {
		t.Error("show output did not contain the exact masked form")
	}
	if strings.Contains(output, replacementValue) {
		t.Error("show output disclosed the complete secret value")
	}

	output, err = runCommand("", "secret", "get", "zeta")
	if err != nil {
		t.Fatalf("get zeta error = %v", err)
	}
	if output != replacementValue+"\n" {
		t.Error("get output did not contain exactly one raw value line")
	}

	db, err := sqlite.Open(ctx, databasePath)
	if err != nil {
		t.Fatalf("inspect database: %v", err)
	}
	rows, err := db.QueryContext(ctx, "SELECT key, created_at, updated_at FROM secrets ORDER BY key")
	if err != nil {
		db.Close()
		t.Fatalf("query secret metadata: %v", err)
	}
	var metadataCount int
	for rows.Next() {
		var key string
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&key, &createdAt, &updatedAt); err != nil {
			rows.Close()
			db.Close()
			t.Fatalf("scan secret metadata: %v", err)
		}
		metadataCount++
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		db.Close()
		t.Fatalf("iterate secret metadata: %v", err)
	}
	rows.Close()
	db.Close()
	if metadataCount != 2 {
		t.Errorf("secret metadata row count = %d, want 2", metadataCount)
	}

	output, err = runCommand("", "secret", "delete", "zeta")
	if err != nil {
		t.Fatalf("delete zeta error = %v", err)
	}
	if output != "Secret deleted: zeta\n" {
		t.Error("delete output did not match")
	}

	if _, err := runCommand("", "secret", "get", "zeta"); err == nil || err.Error() != `secret "zeta" not found` {
		t.Fatalf("missing get error = %v, want exact keyed not-found error", err)
	}
	if _, err := runCommand("", "secret", "delete", "zeta"); err == nil || err.Error() != `secret "zeta" not found` {
		t.Fatalf("missing delete error = %v, want exact keyed not-found error", err)
	}
}

func TestRunSecretArgumentErrors(t *testing.T) {
	t.Setenv("DEVDASH_DB", filepath.Join(t.TempDir(), "devdash.db"))
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing subcommand", args: []string{"secret"}, want: "secret command required: set, get, show, list, or delete"},
		{name: "unknown subcommand", args: []string{"secret", "unknown"}, want: "unknown secret command: unknown"},
		{name: "set missing key", args: []string{"secret", "set"}, want: "usage: devdash secret set <key>"},
		{name: "set extra argument", args: []string{"secret", "set", "key", "extra"}, want: "usage: devdash secret set <key>"},
		{name: "get missing key", args: []string{"secret", "get"}, want: "usage: devdash secret get <key>"},
		{name: "get extra argument", args: []string{"secret", "get", "key", "extra"}, want: "usage: devdash secret get <key>"},
		{name: "show missing key", args: []string{"secret", "show"}, want: "usage: devdash secret show <key>"},
		{name: "show extra argument", args: []string{"secret", "show", "key", "extra"}, want: "usage: devdash secret show <key>"},
		{name: "list extra argument", args: []string{"secret", "list", "extra"}, want: "usage: devdash secret list"},
		{name: "delete missing key", args: []string{"secret", "delete"}, want: "usage: devdash secret delete <key>"},
		{name: "delete extra argument", args: []string{"secret", "delete", "key", "extra"}, want: "usage: devdash secret delete <key>"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := run(context.Background(), test.args, strings.NewReader(""), io.Discard)
			if err == nil || err.Error() != test.want {
				t.Errorf("run() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestRunSecretRejectsInvalidKeyBeforeStorage(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "devdash.db")
	t.Setenv("DEVDASH_DB", databasePath)

	err := run(context.Background(), []string{"secret", "get", ".invalid"}, strings.NewReader(""), io.Discard)
	if !errors.Is(err, secret.ErrInvalidKey) {
		t.Fatalf("run() error = %v, want ErrInvalidKey", err)
	}
	if _, err := os.Stat(databasePath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("database stat error = %v, want not exist", err)
	}
}

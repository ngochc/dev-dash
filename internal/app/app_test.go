package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"

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
			err := run(context.Background(), test.args, &output)
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

	err := runWithUpdater(ctx, []string{"update"}, &output, updater)
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

	err := runWithUpdater(context.Background(), []string{"update", "--help"}, io.Discard, updater)
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
		if err := run(ctx, args, &output); err != nil {
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
			err := run(context.Background(), test.args, &output)
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

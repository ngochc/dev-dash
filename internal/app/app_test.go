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

	"github.com/ngochc/dev-dash/internal/graph"
	"github.com/ngochc/dev-dash/internal/resource"
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

func TestRunVersion(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "devdash.db")
	t.Setenv("DEVDASH_DB", databasePath)

	tests := []struct {
		name       string
		args       []string
		wantOutput string
		wantError  string
	}{
		{name: "valid", args: []string{"version"}, wantOutput: "devdash devel\n"},
		{name: "invalid arguments", args: []string{"version", "extra"}, wantError: "usage: devdash version"},
		{name: "long alias remains unknown", args: []string{"--version"}, wantError: "unknown command: --version"},
		{name: "short alias remains unknown", args: []string{"-v"}, wantError: "unknown command: -v"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			err := run(context.Background(), test.args, strings.NewReader(""), &output)
			if test.wantError == "" {
				if err != nil {
					t.Fatalf("run() error = %v", err)
				}
			} else if err == nil || err.Error() != test.wantError {
				t.Fatalf("run() error = %v, want %q", err, test.wantError)
			}
			if got := output.String(); got != test.wantOutput {
				t.Errorf("run() output = %q, want %q", got, test.wantOutput)
			}
		})
	}

	if _, err := os.Stat(databasePath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("database stat error = %v, want file not to exist", err)
	}
}

func TestHelpListsVersionCommand(t *testing.T) {
	var output bytes.Buffer
	printHelp(&output)
	if !strings.Contains(output.String(), "devdash version") {
		t.Errorf("help output = %q, want containing %q", output.String(), "devdash version")
	}
}

func TestHelpListsGuidedWorkspaceCommands(t *testing.T) {
	var output bytes.Buffer
	printHelp(&output)
	for _, command := range []string{
		"devdash workspace setup <workspace>",
		"devdash workspace check <workspace>",
		"devdash repo pick <workspace>",
	} {
		if !strings.Contains(output.String(), command) {
			t.Errorf("help output = %q, want containing %q", output.String(), command)
		}
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

func TestRunResourceTypeLifecycle(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "devdash.db")
	t.Setenv("DEVDASH_DB", databasePath)
	ctx := context.Background()

	if got := runAppCommand(t, ctx, "resource-type", "add", "service_component", "Service Component", "core", "Deployable service unit"); got != "Resource type registered: service_component\n" {
		t.Errorf("add output = %q", got)
	}
	wantShow := "Name: service_component\nDisplay name: Service Component\nOwner: core\nDescription: Deployable service unit\n"
	if got := runAppCommand(t, ctx, "resource-type", "show", "service_component"); got != wantShow {
		t.Errorf("show output = %q, want %q", got, wantShow)
	}
	list := runAppCommand(t, ctx, "resource-type", "list")
	for _, want := range []string{"NAME", "DISPLAY NAME", "OWNER", "repository", "service_component", "Service Component", "core"} {
		if !strings.Contains(list, want) {
			t.Errorf("list output = %q, want containing %q", list, want)
		}
	}
	if _, err := runAppCommandError(ctx, "resource-type", "add", "service_component", "Other"); err == nil || err.Error() != `resource type "service_component" already exists` || !errors.Is(err, resource.ErrTypeExists) {
		t.Fatalf("duplicate add error = %v, want exact ErrTypeExists", err)
	}
	if _, err := runAppCommandError(ctx, "resource-type", "show", "missing"); err == nil || err.Error() != `resource type "missing" not found` || !errors.Is(err, resource.ErrTypeNotFound) {
		t.Fatalf("missing show error = %v, want exact ErrTypeNotFound", err)
	}
}

func TestRunRelationTypeLifecycle(t *testing.T) {
	t.Setenv("DEVDASH_DB", filepath.Join(t.TempDir(), "devdash.db"))
	ctx := context.Background()

	if got := runAppCommand(t, ctx, "relation-type", "add", "supports", "Supports", "supported_by", "false", "core", "Source supports target"); got != "Relation type registered: supports\n" {
		t.Errorf("add output = %q", got)
	}
	wantShow := "Name: supports\nDisplay name: Supports\nInverse: supported_by\nSymmetric: false\nOwner: core\nDescription: Source supports target\n"
	if got := runAppCommand(t, ctx, "relation-type", "show", "supports"); got != wantShow {
		t.Errorf("show output = %q, want %q", got, wantShow)
	}
	if got := runAppCommand(t, ctx, "relation-type", "add", "cooperates_with", "Cooperates With", "-", "true"); got != "Relation type registered: cooperates_with\n" {
		t.Errorf("symmetric add output = %q", got)
	}
	if got := runAppCommand(t, ctx, "relation-type", "show", "cooperates_with"); !strings.Contains(got, "Inverse: cooperates_with\nSymmetric: true\n") {
		t.Errorf("symmetric show output = %q", got)
	}
	list := runAppCommand(t, ctx, "relation-type", "list")
	for _, want := range []string{"NAME", "DISPLAY NAME", "INVERSE", "SYMMETRIC", "OWNER", "supports", "supported_by", "false"} {
		if !strings.Contains(list, want) {
			t.Errorf("list output = %q, want containing %q", list, want)
		}
	}
	if _, err := runAppCommandError(ctx, "relation-type", "add", "supports", "Other", "-", "false"); err == nil || err.Error() != `relation type "supports" already exists` || !errors.Is(err, graph.ErrTypeExists) {
		t.Fatalf("duplicate add error = %v, want exact ErrTypeExists", err)
	}
	if _, err := runAppCommandError(ctx, "relation-type", "show", "missing"); err == nil || err.Error() != `relation type "missing" not found` || !errors.Is(err, graph.ErrTypeNotFound) {
		t.Fatalf("missing show error = %v, want exact ErrTypeNotFound", err)
	}
}

func TestRunResourceLifecycle(t *testing.T) {
	t.Setenv("DEVDASH_DB", filepath.Join(t.TempDir(), "devdash.db"))
	ctx := context.Background()

	addOutput := runAppCommand(t, ctx, "resource", "add", "service", "api", "https://example.test/api")
	const prefix = "Resource added: "
	if !strings.HasPrefix(addOutput, prefix) || !strings.HasSuffix(addOutput, "\n") {
		t.Fatalf("add output = %q", addOutput)
	}
	resourceID := strings.TrimSuffix(strings.TrimPrefix(addOutput, prefix), "\n")
	if resourceID == "" {
		t.Fatal("add output contains empty resource ID")
	}
	wantShow := fmt.Sprintf("ID: %s\nType: service\nName: api\nURL: https://example.test/api\n", resourceID)
	if got := runAppCommand(t, ctx, "resource", "show", resourceID); got != wantShow {
		t.Errorf("show output = %q, want %q", got, wantShow)
	}
	list := runAppCommand(t, ctx, "resource", "list")
	for _, want := range []string{"ID", "TYPE", "NAME", "URL", resourceID, "service", "api", "https://example.test/api"} {
		if !strings.Contains(list, want) {
			t.Errorf("list output = %q, want containing %q", list, want)
		}
	}
	if got := runAppCommand(t, ctx, "resource", "update", resourceID, "service", "api-v2"); got != "Resource updated: "+resourceID+"\n" {
		t.Errorf("update output = %q", got)
	}
	wantUpdated := fmt.Sprintf("ID: %s\nType: service\nName: api-v2\nURL: \n", resourceID)
	if got := runAppCommand(t, ctx, "resource", "show", resourceID); got != wantUpdated {
		t.Errorf("updated show output = %q, want %q", got, wantUpdated)
	}
	if got := runAppCommand(t, ctx, "resource", "remove", resourceID); got != "Resource removed: "+resourceID+"\n" {
		t.Errorf("remove output = %q", got)
	}
	if _, err := runAppCommandError(ctx, "resource", "show", resourceID); err == nil || err.Error() != fmt.Sprintf("resource %q not found", resourceID) || !errors.Is(err, resource.ErrNotFound) {
		t.Fatalf("show removed error = %v, want exact ErrNotFound", err)
	}
}

func TestRunWorkspaceResourceLifecycle(t *testing.T) {
	t.Setenv("DEVDASH_DB", filepath.Join(t.TempDir(), "devdash.db"))
	ctx := context.Background()
	workspacePath := t.TempDir()
	runAppCommand(t, ctx, "workspace", "add", "demo", workspacePath)
	addOutput := runAppCommand(t, ctx, "resource", "add", "service", "api")
	resourceID := strings.TrimSuffix(strings.TrimPrefix(addOutput, "Resource added: "), "\n")

	if got := runAppCommand(t, ctx, "workspace", "resource", "list", "demo"); got != "No resources found in workspace: demo\n" {
		t.Errorf("empty list output = %q", got)
	}
	if got := runAppCommand(t, ctx, "workspace", "resource", "add", "demo", resourceID, "primary"); got != fmt.Sprintf("Resource added to workspace demo: %s\n", resourceID) {
		t.Errorf("add output = %q", got)
	}
	if _, err := runAppCommandError(ctx, "workspace", "resource", "add", "demo", resourceID, "dependency"); err == nil || err.Error() != fmt.Sprintf("resource %q is already in workspace %q", resourceID, "demo") || !errors.Is(err, workspace.ErrMembershipExists) {
		t.Fatalf("duplicate add error = %v, want exact ErrMembershipExists", err)
	}
	list := runAppCommand(t, ctx, "workspace", "resource", "list", "demo")
	for _, want := range []string{"ID", "TYPE", "NAME", "ROLE", resourceID, "service", "api", "primary"} {
		if !strings.Contains(list, want) {
			t.Errorf("list output = %q, want containing %q", list, want)
		}
	}
	if got := runAppCommand(t, ctx, "workspace", "resource", "remove", "demo", resourceID); got != fmt.Sprintf("Resource removed from workspace demo: %s\n", resourceID) {
		t.Errorf("remove output = %q", got)
	}
	if got := runAppCommand(t, ctx, "resource", "show", resourceID); !strings.Contains(got, "ID: "+resourceID+"\n") {
		t.Errorf("resource after membership removal output = %q", got)
	}
	if _, err := runAppCommandError(ctx, "workspace", "resource", "remove", "demo", resourceID); err == nil || err.Error() != fmt.Sprintf("resource %q is not in workspace %q", resourceID, "demo") || !errors.Is(err, workspace.ErrMembershipNotFound) {
		t.Fatalf("missing remove error = %v, want exact ErrMembershipNotFound", err)
	}
}

func TestResourceCommandArgumentErrorsBeforeDatabaseCreation(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "resource type missing", args: []string{"resource-type"}, want: "resource-type command required: add, list, or show"},
		{name: "resource type unknown", args: []string{"resource-type", "unknown"}, want: "unknown resource-type command: unknown"},
		{name: "resource type list extra", args: []string{"resource-type", "list", "extra"}, want: "usage: devdash resource-type list"},
		{name: "resource type show missing", args: []string{"resource-type", "show"}, want: "usage: devdash resource-type show <name>"},
		{name: "resource type show extra", args: []string{"resource-type", "show", "name", "extra"}, want: "usage: devdash resource-type show <name>"},
		{name: "resource type add missing", args: []string{"resource-type", "add", "name"}, want: "usage: devdash resource-type add <name> <display-name> [owner] [description]"},
		{name: "resource type add extra", args: []string{"resource-type", "add", "name", "display", "owner", "description", "extra"}, want: "usage: devdash resource-type add <name> <display-name> [owner] [description]"},
		{name: "relation type missing", args: []string{"relation-type"}, want: "relation-type command required: add, list, or show"},
		{name: "relation type unknown", args: []string{"relation-type", "unknown"}, want: "unknown relation-type command: unknown"},
		{name: "relation type list extra", args: []string{"relation-type", "list", "extra"}, want: "usage: devdash relation-type list"},
		{name: "relation type show missing", args: []string{"relation-type", "show"}, want: "usage: devdash relation-type show <name>"},
		{name: "relation type show extra", args: []string{"relation-type", "show", "name", "extra"}, want: "usage: devdash relation-type show <name>"},
		{name: "relation type add missing", args: []string{"relation-type", "add", "name", "display", "inverse"}, want: "usage: devdash relation-type add <name> <display-name> <inverse-name-or-> <true|false> [owner] [description]"},
		{name: "relation type add extra", args: []string{"relation-type", "add", "name", "display", "inverse", "false", "owner", "description", "extra"}, want: "usage: devdash relation-type add <name> <display-name> <inverse-name-or-> <true|false> [owner] [description]"},
		{name: "relation type invalid symmetric", args: []string{"relation-type", "add", "name", "display", "-", "yes"}, want: "symmetric must be true or false"},
		{name: "resource missing", args: []string{"resource"}, want: "resource command required: add, list, show, update, or remove"},
		{name: "resource unknown", args: []string{"resource", "unknown"}, want: "unknown resource command: unknown"},
		{name: "resource list extra", args: []string{"resource", "list", "extra"}, want: "usage: devdash resource list"},
		{name: "resource show missing", args: []string{"resource", "show"}, want: "usage: devdash resource show <id>"},
		{name: "resource show extra", args: []string{"resource", "show", "id", "extra"}, want: "usage: devdash resource show <id>"},
		{name: "resource add missing", args: []string{"resource", "add", "type"}, want: "usage: devdash resource add <type> <name> [url]"},
		{name: "resource add extra", args: []string{"resource", "add", "type", "name", "url", "extra"}, want: "usage: devdash resource add <type> <name> [url]"},
		{name: "resource update missing", args: []string{"resource", "update", "id", "type"}, want: "usage: devdash resource update <id> <type> <name> [url]"},
		{name: "resource update extra", args: []string{"resource", "update", "id", "type", "name", "url", "extra"}, want: "usage: devdash resource update <id> <type> <name> [url]"},
		{name: "resource remove missing", args: []string{"resource", "remove"}, want: "usage: devdash resource remove <id>"},
		{name: "resource remove extra", args: []string{"resource", "remove", "id", "extra"}, want: "usage: devdash resource remove <id>"},
		{name: "workspace resource missing", args: []string{"workspace", "resource"}, want: "workspace resource command required: add, list, or remove"},
		{name: "workspace resource unknown", args: []string{"workspace", "resource", "unknown"}, want: "unknown workspace resource command: unknown"},
		{name: "workspace resource list missing", args: []string{"workspace", "resource", "list"}, want: "usage: devdash workspace resource list <workspace-name-or-id>"},
		{name: "workspace resource list extra", args: []string{"workspace", "resource", "list", "workspace", "extra"}, want: "usage: devdash workspace resource list <workspace-name-or-id>"},
		{name: "workspace resource add missing", args: []string{"workspace", "resource", "add", "workspace"}, want: "usage: devdash workspace resource add <workspace-name-or-id> <resource-id> [role]"},
		{name: "workspace resource add extra", args: []string{"workspace", "resource", "add", "workspace", "resource", "role", "extra"}, want: "usage: devdash workspace resource add <workspace-name-or-id> <resource-id> [role]"},
		{name: "workspace resource remove missing", args: []string{"workspace", "resource", "remove", "workspace"}, want: "usage: devdash workspace resource remove <workspace-name-or-id> <resource-id>"},
		{name: "workspace resource remove extra", args: []string{"workspace", "resource", "remove", "workspace", "resource", "extra"}, want: "usage: devdash workspace resource remove <workspace-name-or-id> <resource-id>"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			databasePath := filepath.Join(t.TempDir(), "devdash.db")
			t.Setenv("DEVDASH_DB", databasePath)
			_, err := runAppCommandError(context.Background(), test.args...)
			if err == nil || err.Error() != test.want {
				t.Fatalf("run() error = %v, want %q", err, test.want)
			}
			if _, err := os.Stat(databasePath); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("database stat error = %v, want not exist", err)
			}
		})
	}
}

func TestResourceListEmptyOutput(t *testing.T) {
	var output bytes.Buffer
	printResourceTypeList(&output, nil)
	if got := output.String(); got != "No resource types found.\n" {
		t.Errorf("resource type output = %q", got)
	}
	output.Reset()
	printRelationTypeList(&output, nil)
	if got := output.String(); got != "No relation types found.\n" {
		t.Errorf("relation type output = %q", got)
	}
	output.Reset()
	printResourceList(&output, nil)
	if got := output.String(); got != "No resources found.\n" {
		t.Errorf("resource output = %q", got)
	}
}

func runAppCommand(t *testing.T, ctx context.Context, args ...string) string {
	t.Helper()
	output, err := runAppCommandError(ctx, args...)
	if err != nil {
		t.Fatalf("run(%v) error = %v", args, err)
	}
	return output
}

func runAppCommandError(ctx context.Context, args ...string) (string, error) {
	var output bytes.Buffer
	err := run(ctx, args, strings.NewReader(""), &output)
	return output.String(), err
}

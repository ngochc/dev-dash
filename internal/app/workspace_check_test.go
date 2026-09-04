package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	confluenceintegration "github.com/ngochc/dev-dash/internal/integration/confluence"
	githubintegration "github.com/ngochc/dev-dash/internal/integration/github"
	repositorydomain "github.com/ngochc/dev-dash/internal/repository"
	"github.com/ngochc/dev-dash/internal/secret"
	"github.com/ngochc/dev-dash/internal/wiki"
	"github.com/ngochc/dev-dash/internal/workspace"
)

func TestExecuteWorkspaceCheckIncompleteConfiguration(t *testing.T) {
	dependencies := newCheckDependencies()
	dependencies.config.(*fakeSetupConfig).values = map[string]string{"extra": "configured"}
	var output bytes.Buffer

	err := executeWorkspaceCheck(context.Background(), "workspace", &output, &bytes.Buffer{}, dependencies)
	if !errors.Is(err, ErrWorkspaceIncomplete) {
		t.Fatalf("executeWorkspaceCheck() error = %v, want ErrWorkspaceIncomplete", err)
	}
	if dependencies.github.(*fakeReadinessChecker).calls != 0 {
		t.Fatal("workspace check performed external validation without required organization")
	}
	for _, text := range []string{
		"Workspace\n", "GitHub\n", "Repositories\n", "Status\n",
		"github.base_url: https://github.com (default)", "github.org: MISSING",
		"Status: incomplete", "Configure with:\n  devdash workspace setup workspace",
	} {
		if !strings.Contains(output.String(), text) {
			t.Errorf("output = %q, want containing %q", output.String(), text)
		}
	}
}

func TestExecuteWorkspaceCheckReportsConfiguredAndInvalidBaseURL(t *testing.T) {
	t.Run("configured", func(t *testing.T) {
		dependencies := newCheckDependencies()
		dependencies.config.(*fakeSetupConfig).values = map[string]string{"base_url": "https://git.example.com/", "org": "team"}
		var output bytes.Buffer
		if err := executeWorkspaceCheck(context.Background(), "workspace", &output, &bytes.Buffer{}, dependencies); err != nil {
			t.Fatalf("executeWorkspaceCheck() error = %v", err)
		}
		if !strings.Contains(output.String(), "github.base_url: https://git.example.com (configured)") {
			t.Errorf("output = %q, want configured base URL", output.String())
		}
	})

	t.Run("invalid", func(t *testing.T) {
		dependencies := newCheckDependencies()
		dependencies.config.(*fakeSetupConfig).values = map[string]string{"base_url": "not-a-url", "org": "team"}
		var output bytes.Buffer
		err := executeWorkspaceCheck(context.Background(), "workspace", &output, &bytes.Buffer{}, dependencies)
		if !errors.Is(err, ErrWorkspaceDegraded) {
			t.Fatalf("executeWorkspaceCheck() error = %v, want ErrWorkspaceDegraded", err)
		}
		if dependencies.github.(*fakeReadinessChecker).calls != 0 {
			t.Fatal("workspace check validated an invalid host")
		}
		if !strings.Contains(output.String(), `github.base_url: "not-a-url" (INVALID)`) || !strings.Contains(output.String(), "Status: degraded") {
			t.Errorf("output = %q, want invalid degraded status", output.String())
		}
	})
}

func TestExecuteWorkspaceCheckReadyCountsCachedRepositoryStates(t *testing.T) {
	dependencies := newCheckDependencies()
	dependencies.config.(*fakeSetupConfig).values = map[string]string{"org": "team"}
	dependencies.repositories.(*fakeCheckRepositories).items = []repositorydomain.Listed{
		{State: repositorydomain.StateCloned},
		{State: repositorydomain.StateCloned},
		{State: repositorydomain.StateMissing},
		{State: repositorydomain.StateInvalid},
		{State: repositorydomain.StateNotCloned},
	}
	var output, feedback bytes.Buffer
	github := dependencies.github.(*fakeReadinessChecker)
	repositories := dependencies.repositories.(*fakeCheckRepositories)
	github.beforeCheck = func() {
		if got, want := feedback.String(), "Checking GitHub authentication...\n"; got != want {
			t.Errorf("feedback at authentication check = %q, want %q", got, want)
		}
	}
	repositories.beforeList = func() {
		want := "Checking GitHub authentication...\nChecking GitHub authentication: done\nInspecting repositories...\n"
		if got := feedback.String(); got != want {
			t.Errorf("feedback at repository inspection = %q, want %q", got, want)
		}
	}

	if err := executeWorkspaceCheck(context.Background(), "workspace", &output, &feedback, dependencies); err != nil {
		t.Fatalf("executeWorkspaceCheck() error = %v", err)
	}
	if github.calls != 1 || repositories.calls != 1 {
		t.Fatalf("check calls = GitHub %d, repositories %d; want one each", github.calls, repositories.calls)
	}
	for _, text := range []string{
		"gh: installed", "Authentication: authenticated", "Discovered: 5", "Cloned: 2",
		"Missing: 1", "Invalid: 1", "Not cloned: 1", "Status: ready",
	} {
		if !strings.Contains(output.String(), text) {
			t.Errorf("output = %q, want containing %q", output.String(), text)
		}
	}
	if strings.Contains(output.String(), "Configure with:") {
		t.Errorf("ready output contains setup guidance: %q", output.String())
	}
	wantFeedback := "Checking GitHub authentication...\nChecking GitHub authentication: done\nInspecting repositories...\nInspecting repositories: done\nInspecting wiki pages...\nInspecting wiki pages: done\n"
	if got := feedback.String(); got != wantFeedback {
		t.Errorf("feedback = %q, want %q", got, wantFeedback)
	}
}

func TestExecuteWorkspaceCheckMapsGitHubFailures(t *testing.T) {
	for _, test := range []struct {
		name      string
		checkErr  error
		wantCLI   string
		wantAuth  string
		wantCause error
	}{
		{name: "missing CLI", checkErr: githubintegration.ErrCLIUnavailable, wantCLI: "gh: missing", wantAuth: "Authentication: not checked", wantCause: githubintegration.ErrCLIUnavailable},
		{name: "authentication", checkErr: githubintegration.ErrAuthentication, wantCLI: "gh: installed", wantAuth: "Authentication: failed", wantCause: githubintegration.ErrAuthentication},
	} {
		t.Run(test.name, func(t *testing.T) {
			dependencies := newCheckDependencies()
			dependencies.config.(*fakeSetupConfig).values = map[string]string{"org": "team"}
			dependencies.github.(*fakeReadinessChecker).err = test.checkErr
			var output bytes.Buffer
			err := executeWorkspaceCheck(context.Background(), "workspace", &output, &bytes.Buffer{}, dependencies)
			if !errors.Is(err, ErrWorkspaceDegraded) || !errors.Is(err, test.wantCause) {
				t.Fatalf("executeWorkspaceCheck() error = %v, want degraded and %v", err, test.wantCause)
			}
			if !strings.Contains(output.String(), test.wantCLI) || !strings.Contains(output.String(), test.wantAuth) || !strings.Contains(output.String(), "Status: degraded") {
				t.Errorf("output = %q, want GitHub failure status", output.String())
			}
		})
	}
}

func TestExecuteWorkspaceCheckDegradesForMissingRoot(t *testing.T) {
	dependencies := newCheckDependencies()
	dependencies.config.(*fakeSetupConfig).values = map[string]string{"org": "team"}
	dependencies.directories.(*fakeDirectoryChecker).exists = false
	var output bytes.Buffer

	err := executeWorkspaceCheck(context.Background(), "workspace", &output, &bytes.Buffer{}, dependencies)
	if !errors.Is(err, ErrWorkspaceDegraded) {
		t.Fatalf("executeWorkspaceCheck() error = %v, want ErrWorkspaceDegraded", err)
	}
	if !strings.Contains(output.String(), "Root: MISSING") || !strings.Contains(output.String(), "Status: degraded") {
		t.Errorf("output = %q, want missing root", output.String())
	}
}

func TestExecuteWorkspaceCheckDegradesForRepositoryInspectionFailure(t *testing.T) {
	dependencies := newCheckDependencies()
	dependencies.config.(*fakeSetupConfig).values = map[string]string{"org": "team"}
	inspectionErr := errors.New("inspect checkout failed")
	dependencies.repositories.(*fakeCheckRepositories).err = inspectionErr
	var output bytes.Buffer

	err := executeWorkspaceCheck(context.Background(), "workspace", &output, &bytes.Buffer{}, dependencies)
	if !errors.Is(err, ErrWorkspaceDegraded) || !errors.Is(err, inspectionErr) {
		t.Fatalf("executeWorkspaceCheck() error = %v, want degraded inspection error", err)
	}
	if !strings.Contains(output.String(), "Inspection: failed: inspect checkout failed") {
		t.Errorf("output = %q, want inspection failure", output.String())
	}
}

func TestExecuteWorkspaceCheckUnconfiguredProvidersAreOptional(t *testing.T) {
	dependencies := newCheckDependencies()
	var output bytes.Buffer
	if err := executeWorkspaceCheck(context.Background(), "workspace", &output, &bytes.Buffer{}, dependencies); err != nil {
		t.Fatalf("executeWorkspaceCheck() error = %v", err)
	}
	if dependencies.github.(*fakeReadinessChecker).calls != 0 || dependencies.confluence.(*fakeConfluenceReadinessChecker).calls != 0 {
		t.Fatal("unconfigured provider made external call")
	}
	if strings.Count(output.String(), "status: not configured") != 2 || !strings.Contains(output.String(), "Status: ready") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestExecuteWorkspaceCheckConfluenceOnlyReadyWithWikiCounts(t *testing.T) {
	dependencies := newCheckDependencies()
	dependencies.config.(*fakeSetupConfig).confluenceValues = map[string]string{
		"base_url":  "https://wiki.example/confluence/",
		"space":     "DOC",
		"secret":    "secret:confluence.pat",
		"root_page": "123",
	}
	dependencies.wiki.(*fakeCheckWiki).items = []wiki.Listed{
		{State: wiki.StateFetched},
		{State: wiki.StateMissing},
		{State: wiki.StateNotFetched},
	}
	var output bytes.Buffer
	if err := executeWorkspaceCheck(context.Background(), "workspace", &output, &bytes.Buffer{}, dependencies); err != nil {
		t.Fatalf("executeWorkspaceCheck() error = %v", err)
	}
	if dependencies.github.(*fakeReadinessChecker).calls != 0 || dependencies.confluence.(*fakeConfluenceReadinessChecker).calls != 1 {
		t.Fatalf("provider calls = GitHub %d Confluence %d", dependencies.github.(*fakeReadinessChecker).calls, dependencies.confluence.(*fakeConfluenceReadinessChecker).calls)
	}
	for _, text := range []string{"GitHub\n  status: not configured", "confluence.base_url: https://wiki.example/confluence", "confluence.space: DOC", "confluence.secret: secret:confluence.pat", "confluence.root_page: 123", "auth: PAT OK", "Discovered: 3", "Fetched: 1", "Missing: 1", "Not fetched: 1", "Status: ready"} {
		if !strings.Contains(output.String(), text) {
			t.Errorf("output = %q, want %q", output.String(), text)
		}
	}
}

func TestExecuteWorkspaceCheckConfluenceIncompleteSuppressesExternalCall(t *testing.T) {
	dependencies := newCheckDependencies()
	dependencies.config.(*fakeSetupConfig).confluenceValues = map[string]string{"base_url": "https://wiki.example"}
	var output bytes.Buffer
	err := executeWorkspaceCheck(context.Background(), "workspace", &output, &bytes.Buffer{}, dependencies)
	if !errors.Is(err, ErrWorkspaceIncomplete) || dependencies.confluence.(*fakeConfluenceReadinessChecker).calls != 0 {
		t.Fatalf("check error/calls = %v/%d", err, dependencies.confluence.(*fakeConfluenceReadinessChecker).calls)
	}
	if !strings.Contains(output.String(), "confluence.space: MISSING") || !strings.Contains(output.String(), "confluence.secret: MISSING") || !strings.Contains(output.String(), "Status: incomplete") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestExecuteWorkspaceCheckConfluenceInvalidReferenceIsSafeAndDegraded(t *testing.T) {
	dependencies := newCheckDependencies()
	dependencies.config.(*fakeSetupConfig).confluenceValues = map[string]string{"base_url": "https://wiki.example", "space": "DOC", "secret": "raw-private-pat"}
	var output bytes.Buffer
	err := executeWorkspaceCheck(context.Background(), "workspace", &output, &bytes.Buffer{}, dependencies)
	if !errors.Is(err, ErrWorkspaceDegraded) || dependencies.confluence.(*fakeConfluenceReadinessChecker).calls != 0 {
		t.Fatalf("check error/calls = %v/%d", err, dependencies.confluence.(*fakeConfluenceReadinessChecker).calls)
	}
	if strings.Contains(output.String(), "raw-private-pat") || !strings.Contains(output.String(), "confluence.secret: INVALID") {
		t.Fatalf("unsafe output = %q", output.String())
	}
}

func TestExecuteWorkspaceCheckConfluenceFailureAndWikiInspectionDegrade(t *testing.T) {
	dependencies := newCheckDependencies()
	dependencies.config.(*fakeSetupConfig).confluenceValues = map[string]string{"base_url": "https://wiki.example", "space": "DOC", "secret": "secret:confluence.pat"}
	dependencies.confluence.(*fakeConfluenceReadinessChecker).err = secret.ErrNotFound
	inspectionErr := errors.New("inspect wiki failed")
	dependencies.wiki.(*fakeCheckWiki).err = inspectionErr
	var output bytes.Buffer
	err := executeWorkspaceCheck(context.Background(), "workspace", &output, &bytes.Buffer{}, dependencies)
	if !errors.Is(err, ErrWorkspaceDegraded) || !errors.Is(err, secret.ErrNotFound) || !errors.Is(err, inspectionErr) {
		t.Fatalf("executeWorkspaceCheck() error = %v", err)
	}
	if !strings.Contains(output.String(), "auth: PAT failed") || !strings.Contains(output.String(), "Inspection: failed: inspect wiki failed") || !strings.Contains(output.String(), "Status: degraded") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestExecuteWorkspaceCheckIncompletePrecedesDegraded(t *testing.T) {
	dependencies := newCheckDependencies()
	dependencies.config.(*fakeSetupConfig).values = map[string]string{"base_url": "https://github.com"}
	dependencies.config.(*fakeSetupConfig).confluenceValues = map[string]string{"base_url": "bad", "space": "DOC", "secret": "secret:pat"}
	err := executeWorkspaceCheck(context.Background(), "workspace", &bytes.Buffer{}, &bytes.Buffer{}, dependencies)
	if !errors.Is(err, ErrWorkspaceIncomplete) || errors.Is(err, ErrWorkspaceDegraded) {
		t.Fatalf("executeWorkspaceCheck() error = %v, want incomplete precedence", err)
	}
}

func TestWorkspaceCheckValidatesArgumentsBeforeDatabaseCreation(t *testing.T) {
	for _, args := range [][]string{{"workspace", "check"}, {"workspace", "check", "workspace", "extra"}} {
		databasePath := filepath.Join(t.TempDir(), "devdash.db")
		t.Setenv("DEVDASH_DB", databasePath)
		err := run(context.Background(), args, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil || err.Error() != "usage: devdash workspace check <workspace>" {
			t.Fatalf("run(%v) error = %v, want check usage", args, err)
		}
		if _, statErr := os.Stat(databasePath); !os.IsNotExist(statErr) {
			t.Fatalf("database stat error = %v, want missing", statErr)
		}
	}
}

func newCheckDependencies() workspaceCheckDependencies {
	return workspaceCheckDependencies{
		workspaces:   &fakeSetupWorkspace{item: workspace.Workspace{ID: "workspace-id", Name: "workspace", LocalPath: "/work/workspace"}},
		config:       &fakeSetupConfig{setValues: make(map[string]string)},
		github:       &fakeReadinessChecker{},
		confluence:   &fakeConfluenceReadinessChecker{},
		repositories: &fakeCheckRepositories{},
		wiki:         &fakeCheckWiki{},
		directories:  &fakeDirectoryChecker{exists: true},
	}
}

type fakeReadinessChecker struct {
	calls       int
	err         error
	beforeCheck func()
}

func (f *fakeReadinessChecker) Check(context.Context, string) (workspace.Workspace, githubintegration.Config, error) {
	if f.beforeCheck != nil {
		f.beforeCheck()
	}
	f.calls++
	return workspace.Workspace{}, githubintegration.Config{}, f.err
}

type fakeConfluenceReadinessChecker struct {
	calls int
	err   error
}

func (f *fakeConfluenceReadinessChecker) Check(context.Context, string) (workspace.Workspace, confluenceintegration.Config, error) {
	f.calls++
	return workspace.Workspace{}, confluenceintegration.Config{}, f.err
}

type fakeCheckRepositories struct {
	items      []repositorydomain.Listed
	err        error
	calls      int
	beforeList func()
}

func (f *fakeCheckRepositories) List(context.Context, string) (workspace.Workspace, []repositorydomain.Listed, error) {
	if f.beforeList != nil {
		f.beforeList()
	}
	f.calls++
	return workspace.Workspace{}, append([]repositorydomain.Listed(nil), f.items...), f.err
}

type fakeCheckWiki struct {
	items []wiki.Listed
	err   error
	calls int
}

func (f *fakeCheckWiki) List(context.Context, string) (workspace.Workspace, []wiki.Listed, error) {
	f.calls++
	return workspace.Workspace{}, append([]wiki.Listed(nil), f.items...), f.err
}

type fakeDirectoryChecker struct {
	exists bool
	err    error
	calls  int
}

func (f *fakeDirectoryChecker) Exists(string) (bool, error) {
	f.calls++
	return f.exists, f.err
}

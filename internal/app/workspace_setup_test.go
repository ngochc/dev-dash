package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	githubintegration "github.com/ngochc/dev-dash/internal/integration/github"
	repositorydomain "github.com/ngochc/dev-dash/internal/repository"
	"github.com/ngochc/dev-dash/internal/ui/picker"
	"github.com/ngochc/dev-dash/internal/workspace"
)

func TestExecuteWorkspaceSetupKeepsExistingConfiguration(t *testing.T) {
	dependencies := newSetupDependencies()
	dependencies.config.(*fakeSetupConfig).values = map[string]string{"org": "team"}
	dependencies.github.(*fakeSetupGitHub).owners = []githubintegration.Owner{{Login: "team"}}
	dependencies.picker.(*fakeSetupPicker).confirms = []bool{true, true}
	var output bytes.Buffer

	if err := executeWorkspaceSetup(context.Background(), "workspace", &output, &bytes.Buffer{}, dependencies); err != nil {
		t.Fatalf("executeWorkspaceSetup() error = %v", err)
	}
	githubClient := dependencies.github.(*fakeSetupGitHub)
	wantConfig := githubintegration.Config{BaseURL: githubintegration.DefaultBaseURL, Host: "github.com"}
	if githubClient.validated != wantConfig {
		t.Errorf("validated config = %#v, want %#v", githubClient.validated, wantConfig)
	}
	config := dependencies.config.(*fakeSetupConfig)
	if _, wroteBaseURL := config.setValues[githubintegration.BaseURLKey]; wroteBaseURL {
		t.Error("setup stored a default base URL while keeping an absent override")
	}
	if config.setValues[githubintegration.OrganizationKey] != "team" {
		t.Errorf("stored organization = %q, want team", config.setValues[githubintegration.OrganizationKey])
	}
	if !strings.Contains(output.String(), "Current GitHub configuration:") || !strings.Contains(output.String(), "Workspace setup complete.") {
		t.Errorf("output = %q, want current configuration and completion", output.String())
	}
}

func TestExecuteWorkspaceSetupSelectsDefaultHostAndPersonalOwner(t *testing.T) {
	dependencies := newSetupDependencies()
	dependencies.github.(*fakeSetupGitHub).owners = []githubintegration.Owner{{Login: "team"}, {Login: "personal", Personal: true}}
	pickerFake := dependencies.picker.(*fakeSetupPicker)
	pickerFake.one = []string{"github.com", "personal"}
	var output bytes.Buffer

	if err := executeWorkspaceSetup(context.Background(), "workspace", &output, &bytes.Buffer{}, dependencies); err != nil {
		t.Fatalf("executeWorkspaceSetup() error = %v", err)
	}
	config := dependencies.config.(*fakeSetupConfig)
	if config.setValues[githubintegration.BaseURLKey] != githubintegration.DefaultBaseURL || config.setValues[githubintegration.OrganizationKey] != "personal" {
		t.Errorf("stored config = %#v", config.setValues)
	}
	if len(pickerFake.oneOptions) != 2 || pickerFake.oneOptions[1][1].Value != "personal" || pickerFake.oneOptions[1][1].Label != "personal (personal)" {
		t.Errorf("owner options = %#v, want personal account mapping", pickerFake.oneOptions)
	}
	if got, want := pickerFake.oneDefaults, []string{"github.com", "personal"}; !reflect.DeepEqual(got, want) {
		t.Errorf("picker defaults = %#v, want %#v", got, want)
	}
}

func TestExecuteWorkspaceSetupNormalizesCustomGHESHost(t *testing.T) {
	dependencies := newSetupDependencies()
	dependencies.github.(*fakeSetupGitHub).owners = []githubintegration.Owner{{Login: "team"}}
	pickerFake := dependencies.picker.(*fakeSetupPicker)
	pickerFake.one = []string{"custom", "team"}
	pickerFake.inputs = []string{"not a URL", "https://Git.Example.com///"}
	var output bytes.Buffer

	if err := executeWorkspaceSetup(context.Background(), "workspace", &output, &bytes.Buffer{}, dependencies); err != nil {
		t.Fatalf("executeWorkspaceSetup() error = %v", err)
	}
	config := dependencies.config.(*fakeSetupConfig)
	if got := config.setValues[githubintegration.BaseURLKey]; got != "https://Git.Example.com" {
		t.Errorf("stored base URL = %q, want normalized GHES URL", got)
	}
	if got := dependencies.github.(*fakeSetupGitHub).validated.Host; got != "git.example.com" {
		t.Errorf("validated host = %q, want git.example.com", got)
	}
	if !strings.Contains(output.String(), "Invalid GitHub configuration") {
		t.Errorf("output = %q, want invalid URL guidance", output.String())
	}
}

func TestExecuteWorkspaceSetupMapsGitHubReadinessErrors(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want error
		text string
	}{
		{name: "missing CLI", err: githubintegration.ErrCLIUnavailable, want: githubintegration.ErrCLIUnavailable, text: "GitHub CLI is required for GitHub setup.\n\nInstall `gh` and run:\n  devdash workspace setup workspace"},
		{name: "authentication", err: githubintegration.ErrAuthentication, want: githubintegration.ErrAuthentication, text: "GitHub CLI is not authenticated for github.com.\n\nAuthenticate with:\n  gh auth login --hostname github.com\n\nThen run:\n  devdash workspace setup workspace"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dependencies := newSetupDependencies()
			dependencies.github.(*fakeSetupGitHub).validateErr = test.err
			dependencies.picker.(*fakeSetupPicker).one = []string{"github.com"}
			err := executeWorkspaceSetup(context.Background(), "workspace", &bytes.Buffer{}, &bytes.Buffer{}, dependencies)
			if !errors.Is(err, test.want) || err.Error() != test.text {
				t.Fatalf("executeWorkspaceSetup() error = %v, want %q preserving %v", err, test.text, test.want)
			}
			if dependencies.github.(*fakeSetupGitHub).discoverCalls != 0 || dependencies.repositories.(*fakeSetupRepositories).refreshCalls != 0 {
				t.Fatal("setup continued after GitHub readiness failure")
			}
		})
	}
}

func TestExecuteWorkspaceSetupFallsBackToManualOwner(t *testing.T) {
	dependencies := newSetupDependencies()
	dependencies.config.(*fakeSetupConfig).values = map[string]string{"base_url": "https://github.com", "org": "existing"}
	dependencies.github.(*fakeSetupGitHub).ownerErr = errors.New("owner API failed")
	pickerFake := dependencies.picker.(*fakeSetupPicker)
	pickerFake.confirms = []bool{true}
	pickerFake.inputs = []string{"manual-owner"}
	var output bytes.Buffer

	if err := executeWorkspaceSetup(context.Background(), "workspace", &output, &bytes.Buffer{}, dependencies); err != nil {
		t.Fatalf("executeWorkspaceSetup() error = %v", err)
	}
	if got := dependencies.config.(*fakeSetupConfig).setValues[githubintegration.OrganizationKey]; got != "manual-owner" {
		t.Errorf("stored organization = %q, want manual-owner", got)
	}
	if !strings.Contains(output.String(), "Could not discover GitHub owners: owner API failed") {
		t.Errorf("output = %q, want discovery failure notice", output.String())
	}
}

func TestExecuteWorkspaceSetupStopsOnRefreshFailure(t *testing.T) {
	dependencies := newSetupDependencies()
	dependencies.github.(*fakeSetupGitHub).owners = []githubintegration.Owner{{Login: "team"}}
	dependencies.picker.(*fakeSetupPicker).one = []string{"github.com", "team"}
	refreshErr := errors.New("refresh failed")
	dependencies.repositories.(*fakeSetupRepositories).refreshErr = refreshErr

	err := executeWorkspaceSetup(context.Background(), "workspace", &bytes.Buffer{}, &bytes.Buffer{}, dependencies)
	if !errors.Is(err, refreshErr) {
		t.Fatalf("executeWorkspaceSetup() error = %v, want refresh error", err)
	}
	if dependencies.repositories.(*fakeSetupRepositories).listCalls != 0 || dependencies.picker.(*fakeSetupPicker).manyCalls != 0 {
		t.Fatal("setup listed or prompted after refresh failure")
	}
}

func TestExecuteWorkspaceSetupClonesSelectedExternalKeysAndSummarizes(t *testing.T) {
	dependencies := newSetupDependencies()
	dependencies.github.(*fakeSetupGitHub).owners = []githubintegration.Owner{{Login: "team"}}
	dependencies.repositories.(*fakeSetupRepositories).listed = []repositorydomain.Listed{
		{Repository: repositorydomain.Repository{ExternalKey: "team/api"}, State: repositorydomain.StateNotCloned},
		{Repository: repositorydomain.Repository{ExternalKey: "team/web"}, State: repositorydomain.StateMissing},
		{Repository: repositorydomain.Repository{ExternalKey: "team/broken"}, State: repositorydomain.StateInvalid},
	}
	cloneErr := errors.New("aggregate clone failure")
	dependencies.repositories.(*fakeSetupRepositories).cloneErr = cloneErr
	dependencies.repositories.(*fakeSetupRepositories).cloneResults = []repositorydomain.CloneResult{
		{Repository: "team/api", Status: "cloned"},
		{Repository: "team/web", Status: "restored"},
		{Repository: "team/existing", Status: "already cloned"},
		{Repository: "team/broken", Error: errors.New("conflict")},
	}
	pickerFake := dependencies.picker.(*fakeSetupPicker)
	pickerFake.one = []string{"github.com", "team"}
	pickerFake.many = [][]string{{"team/web", "team/api", "team/broken"}}
	pickerFake.confirms = []bool{true}
	var output, feedback bytes.Buffer
	wantAtOperation := []string{
		"Checking GitHub authentication...\n",
		"Checking GitHub authentication...\nChecking GitHub authentication: done\nDiscovering GitHub owners...\n",
		"Checking GitHub authentication...\nChecking GitHub authentication: done\nDiscovering GitHub owners...\nDiscovering GitHub owners: done\nRefreshing repositories...\n",
		"Checking GitHub authentication...\nChecking GitHub authentication: done\nDiscovering GitHub owners...\nDiscovering GitHub owners: done\nRefreshing repositories...\nRefreshing repositories: done\nInspecting repositories...\n",
		"Checking GitHub authentication...\nChecking GitHub authentication: done\nDiscovering GitHub owners...\nDiscovering GitHub owners: done\nRefreshing repositories...\nRefreshing repositories: done\nInspecting repositories...\nInspecting repositories: done\nCloning selected repositories...\n",
	}
	operation := 0
	assertProgressStarted := func() {
		t.Helper()
		if got, want := feedback.String(), wantAtOperation[operation]; got != want {
			t.Errorf("feedback before operation %d = %q, want %q", operation, got, want)
		}
		operation++
	}
	github := dependencies.github.(*fakeSetupGitHub)
	github.beforeValidate = assertProgressStarted
	github.beforeDiscover = assertProgressStarted
	repositories := dependencies.repositories.(*fakeSetupRepositories)
	repositories.beforeRefresh = assertProgressStarted
	repositories.beforeList = assertProgressStarted
	repositories.beforeClone = assertProgressStarted

	err := executeWorkspaceSetup(context.Background(), "workspace", &output, &feedback, dependencies)
	if !errors.Is(err, cloneErr) {
		t.Fatalf("executeWorkspaceSetup() error = %v, want aggregate clone error", err)
	}
	if operation != len(wantAtOperation) {
		t.Fatalf("setup operations = %d, want %d", operation, len(wantAtOperation))
	}
	wantSelectors := []string{"team/web", "team/api", "team/broken"}
	if !reflect.DeepEqual(repositories.cloneSelectors, wantSelectors) {
		t.Errorf("CloneKnown selectors = %#v, want %#v", repositories.cloneSelectors, wantSelectors)
	}
	wantOptionValues := []string{"team/api", "team/web", "team/broken"}
	gotOptionValues := make([]string, len(pickerFake.manyOptions[0]))
	for index, option := range pickerFake.manyOptions[0] {
		gotOptionValues[index] = option.Value
	}
	if !reflect.DeepEqual(gotOptionValues, wantOptionValues) {
		t.Errorf("repository option values = %#v, want %#v", gotOptionValues, wantOptionValues)
	}
	for _, text := range []string{"Selected repositories:", "team/web", "team/broken", "failed: conflict", "Repositories: cloned 2, existing 1, failed 1", "Next:\n  devdash repo list workspace"} {
		if !strings.Contains(output.String(), text) {
			t.Errorf("output = %q, want containing %q", output.String(), text)
		}
	}
	if strings.Contains(output.String(), "Checking GitHub") || strings.Contains(output.String(), "Discovering GitHub") || strings.Contains(output.String(), "Refreshing repositories") || strings.Contains(output.String(), "Inspecting repositories") || strings.Contains(output.String(), "Cloning selected repositories") {
		t.Errorf("setup result contains progress feedback: %q", output.String())
	}
	wantFeedback := wantAtOperation[len(wantAtOperation)-1] + "Cloning selected repositories: failed\n"
	if got := feedback.String(); got != wantFeedback {
		t.Errorf("setup feedback = %q, want %q", got, wantFeedback)
	}
}

func TestExecuteWorkspaceSetupDeclineAndCancellationDoNotClone(t *testing.T) {
	t.Run("declined clone", func(t *testing.T) {
		dependencies := newSetupDependencies()
		dependencies.github.(*fakeSetupGitHub).owners = []githubintegration.Owner{{Login: "team"}}
		pickerFake := dependencies.picker.(*fakeSetupPicker)
		pickerFake.one = []string{"github.com", "team"}
		pickerFake.many = [][]string{{"team/api"}}
		pickerFake.confirms = []bool{false}
		if err := executeWorkspaceSetup(context.Background(), "workspace", &bytes.Buffer{}, &bytes.Buffer{}, dependencies); err != nil {
			t.Fatalf("executeWorkspaceSetup() error = %v", err)
		}
		if dependencies.repositories.(*fakeSetupRepositories).cloneCalls != 0 {
			t.Fatal("setup cloned after confirmation declined")
		}
	})

	t.Run("host cancelled", func(t *testing.T) {
		dependencies := newSetupDependencies()
		dependencies.picker.(*fakeSetupPicker).oneErrors = []error{picker.ErrCancelled}
		var output bytes.Buffer
		if err := executeWorkspaceSetup(context.Background(), "workspace", &output, &bytes.Buffer{}, dependencies); err != nil {
			t.Fatalf("executeWorkspaceSetup() error = %v", err)
		}
		if len(dependencies.config.(*fakeSetupConfig).setValues) != 0 || dependencies.github.(*fakeSetupGitHub).validateCalls != 0 {
			t.Fatal("setup changed config or validated after host cancellation")
		}
		if !strings.Contains(output.String(), "Workspace setup cancelled.") {
			t.Errorf("output = %q, want cancellation", output.String())
		}
	})

	t.Run("owner cancelled retains host", func(t *testing.T) {
		dependencies := newSetupDependencies()
		dependencies.github.(*fakeSetupGitHub).owners = []githubintegration.Owner{{Login: "team"}}
		pickerFake := dependencies.picker.(*fakeSetupPicker)
		pickerFake.one = []string{"github.com"}
		pickerFake.oneErrors = []error{nil, picker.ErrCancelled}
		if err := executeWorkspaceSetup(context.Background(), "workspace", &bytes.Buffer{}, &bytes.Buffer{}, dependencies); err != nil {
			t.Fatalf("executeWorkspaceSetup() error = %v", err)
		}
		if got, want := pickerFake.oneDefaults, []string{"github.com", ""}; !reflect.DeepEqual(got, want) {
			t.Errorf("picker defaults = %#v, want %#v", got, want)
		}
		config := dependencies.config.(*fakeSetupConfig)
		if config.setValues[githubintegration.BaseURLKey] != githubintegration.DefaultBaseURL {
			t.Error("setup did not retain explicit host before owner cancellation")
		}
		if _, storedOwner := config.setValues[githubintegration.OrganizationKey]; storedOwner {
			t.Error("setup stored an owner after cancellation")
		}
	})
}

func TestWorkspaceSetupValidatesArgumentsBeforeDatabaseCreation(t *testing.T) {
	for _, args := range [][]string{{"workspace", "setup"}, {"workspace", "setup", "workspace", "extra"}} {
		databasePath := filepath.Join(t.TempDir(), "devdash.db")
		t.Setenv("DEVDASH_DB", databasePath)
		err := run(context.Background(), args, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil || err.Error() != "usage: devdash workspace setup <workspace>" {
			t.Fatalf("run(%v) error = %v, want setup usage", args, err)
		}
		if _, statErr := os.Stat(databasePath); !os.IsNotExist(statErr) {
			t.Fatalf("database stat error = %v, want missing", statErr)
		}
	}
}

func newSetupDependencies() workspaceSetupDependencies {
	return workspaceSetupDependencies{
		workspaces:   &fakeSetupWorkspace{item: workspace.Workspace{ID: "workspace-id", Name: "workspace", LocalPath: "/work/workspace"}},
		config:       &fakeSetupConfig{setValues: make(map[string]string)},
		github:       &fakeSetupGitHub{},
		repositories: &fakeSetupRepositories{},
		picker:       &fakeSetupPicker{},
	}
}

type fakeSetupWorkspace struct {
	item workspace.Workspace
	err  error
}

func (f *fakeSetupWorkspace) Get(context.Context, string) (workspace.Workspace, error) {
	return f.item, f.err
}

type fakeSetupConfig struct {
	values    map[string]string
	setValues map[string]string
}

func (f *fakeSetupConfig) Namespace(context.Context, string, string) (map[string]string, error) {
	values := make(map[string]string, len(f.values))
	for key, value := range f.values {
		values[key] = value
	}
	return values, nil
}

func (f *fakeSetupConfig) SetUser(_ context.Context, _ string, key, value string) (workspace.Workspace, error) {
	f.setValues[key] = value
	return workspace.Workspace{}, nil
}

type fakeSetupGitHub struct {
	validated      githubintegration.Config
	validateErr    error
	owners         []githubintegration.Owner
	ownerErr       error
	validateCalls  int
	discoverCalls  int
	beforeValidate func()
	beforeDiscover func()
}

func (f *fakeSetupGitHub) Validate(_ context.Context, config githubintegration.Config) error {
	if f.beforeValidate != nil {
		f.beforeValidate()
	}
	f.validateCalls++
	f.validated = config
	return f.validateErr
}

func (f *fakeSetupGitHub) DiscoverOwners(context.Context, githubintegration.Config) ([]githubintegration.Owner, error) {
	if f.beforeDiscover != nil {
		f.beforeDiscover()
	}
	f.discoverCalls++
	return append([]githubintegration.Owner(nil), f.owners...), f.ownerErr
}

type fakeSetupRepositories struct {
	refreshErr     error
	listed         []repositorydomain.Listed
	cloneResults   []repositorydomain.CloneResult
	cloneErr       error
	refreshCalls   int
	listCalls      int
	cloneCalls     int
	cloneSelectors []string
	beforeRefresh  func()
	beforeList     func()
	beforeClone    func()
}

func (f *fakeSetupRepositories) Refresh(context.Context, string) (workspace.Workspace, int, error) {
	if f.beforeRefresh != nil {
		f.beforeRefresh()
	}
	f.refreshCalls++
	return workspace.Workspace{}, len(f.listed), f.refreshErr
}

func (f *fakeSetupRepositories) List(context.Context, string) (workspace.Workspace, []repositorydomain.Listed, error) {
	if f.beforeList != nil {
		f.beforeList()
	}
	f.listCalls++
	return workspace.Workspace{}, append([]repositorydomain.Listed(nil), f.listed...), nil
}

func (f *fakeSetupRepositories) CloneKnown(_ context.Context, _ string, selectors []string) (workspace.Workspace, []repositorydomain.CloneResult, error) {
	if f.beforeClone != nil {
		f.beforeClone()
	}
	f.cloneCalls++
	f.cloneSelectors = append([]string(nil), selectors...)
	return workspace.Workspace{}, append([]repositorydomain.CloneResult(nil), f.cloneResults...), f.cloneErr
}

type fakeSetupPicker struct {
	one         []string
	oneErrors   []error
	many        [][]string
	manyErrors  []error
	confirms    []bool
	confirmErrs []error
	inputs      []string
	inputErrors []error
	oneOptions  [][]picker.Option
	oneDefaults []string
	manyOptions [][]picker.Option
	manyCalls   int
}

func (f *fakeSetupPicker) PickOne(_ context.Context, _ string, options []picker.Option, defaultValue string) (string, error) {
	f.oneOptions = append(f.oneOptions, append([]picker.Option(nil), options...))
	f.oneDefaults = append(f.oneDefaults, defaultValue)
	var value string
	if len(f.one) > 0 {
		value, f.one = f.one[0], f.one[1:]
	}
	var err error
	if len(f.oneErrors) > 0 {
		err, f.oneErrors = f.oneErrors[0], f.oneErrors[1:]
	}
	return value, err
}

func (f *fakeSetupPicker) PickMany(_ context.Context, _ string, options []picker.Option) ([]string, error) {
	f.manyCalls++
	f.manyOptions = append(f.manyOptions, append([]picker.Option(nil), options...))
	var values []string
	if len(f.many) > 0 {
		values, f.many = f.many[0], f.many[1:]
	}
	var err error
	if len(f.manyErrors) > 0 {
		err, f.manyErrors = f.manyErrors[0], f.manyErrors[1:]
	}
	return append([]string(nil), values...), err
}

func (f *fakeSetupPicker) Confirm(string, bool) (bool, error) {
	var value bool
	if len(f.confirms) > 0 {
		value, f.confirms = f.confirms[0], f.confirms[1:]
	}
	var err error
	if len(f.confirmErrs) > 0 {
		err, f.confirmErrs = f.confirmErrs[0], f.confirmErrs[1:]
	}
	return value, err
}

func (f *fakeSetupPicker) Input(string, string) (string, error) {
	var value string
	if len(f.inputs) > 0 {
		value, f.inputs = f.inputs[0], f.inputs[1:]
	}
	var err error
	if len(f.inputErrors) > 0 {
		err, f.inputErrors = f.inputErrors[0], f.inputErrors[1:]
	}
	return value, err
}

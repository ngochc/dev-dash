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

	confluenceintegration "github.com/ngochc/dev-dash/internal/integration/confluence"
	githubintegration "github.com/ngochc/dev-dash/internal/integration/github"
	repositorydomain "github.com/ngochc/dev-dash/internal/repository"
	"github.com/ngochc/dev-dash/internal/secret"
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

func TestExecuteWorkspaceSetupShowsProviderStatesAndCancelsWithoutChanges(t *testing.T) {
	dependencies := newSetupDependencies()
	config := dependencies.config.(*fakeSetupConfig)
	config.values = map[string]string{"org": "team"}
	config.confluenceValues = map[string]string{"base_url": "https://wiki.example", "space": "DOC", "secret": "secret:confluence.pat"}
	dependencies.secrets.(*fakeSetupSecrets).values["confluence.pat"] = "stored-pat"
	dependencies.picker.(*fakeSetupPicker).providers = [][]string{{}}
	var output bytes.Buffer
	if err := executeWorkspaceSetup(context.Background(), "workspace", &output, &bytes.Buffer{}, dependencies); err != nil {
		t.Fatalf("executeWorkspaceSetup() error = %v", err)
	}
	pickerFake := dependencies.picker.(*fakeSetupPicker)
	if pickerFake.providerCalls != 1 || len(pickerFake.providerOptions) != 1 {
		t.Fatalf("provider calls/options = %d/%#v", pickerFake.providerCalls, pickerFake.providerOptions)
	}
	want := []picker.Option{{Value: "github", Label: "GitHub      configured"}, {Value: "confluence", Label: "Confluence  configured"}}
	if !reflect.DeepEqual(pickerFake.providerOptions[0], want) {
		t.Fatalf("provider options = %#v, want %#v", pickerFake.providerOptions[0], want)
	}
	if len(config.setValues) != 0 || dependencies.github.(*fakeSetupGitHub).validateCalls != 0 || dependencies.confluence.(*fakeSetupConfluence).validateCalls != 0 {
		t.Fatal("cancelled provider selection changed configuration")
	}
	if !strings.Contains(output.String(), "Workspace setup cancelled.") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestExecuteWorkspaceSetupConfiguresConfluenceWithNewPATAndRoot(t *testing.T) {
	dependencies := newSetupDependencies()
	pickerFake := dependencies.picker.(*fakeSetupPicker)
	pickerFake.providers = [][]string{{"confluence"}}
	pickerFake.inputs = []string{"bad URL", "https://Wiki.Example.com/confluence///", "", "DOC", "bad", "123"}
	pickerFake.secrets = []string{"new-private-pat"}
	pickerFake.confirms = []bool{true}
	var output, feedback bytes.Buffer
	if err := executeWorkspaceSetup(context.Background(), "workspace", &output, &feedback, dependencies); err != nil {
		t.Fatalf("executeWorkspaceSetup() error = %v", err)
	}
	client := dependencies.confluence.(*fakeSetupConfluence)
	if client.validateCalls != 1 || client.pat != "new-private-pat" || client.validated.BaseURL != "https://Wiki.Example.com/confluence" || client.validated.Space != "DOC" || client.validated.RootPage != "123" {
		t.Fatalf("validated = %#v PAT=%q calls=%d", client.validated, client.pat, client.validateCalls)
	}
	secrets := dependencies.secrets.(*fakeSetupSecrets)
	if secrets.sets["confluence.pat"] != "new-private-pat" {
		t.Fatalf("stored secrets = %#v", secrets.sets)
	}
	config := dependencies.config.(*fakeSetupConfig)
	for key, value := range map[string]string{
		confluenceintegration.BaseURLKey:  "https://Wiki.Example.com/confluence",
		confluenceintegration.SpaceKey:    "DOC",
		confluenceintegration.SecretKey:   "secret:confluence.pat",
		confluenceintegration.RootPageKey: "123",
	} {
		if config.setValues[key] != value {
			t.Errorf("config %s = %q, want %q", key, config.setValues[key], value)
		}
	}
	for _, text := range []string{"Invalid Confluence configuration", "Expected a decimal page ID.", "Confluence checks:", "URL: OK", "Auth: PAT", "Secret: configured", "Root: 123", "Status: ready", "devdash wiki refresh workspace"} {
		if !strings.Contains(output.String(), text) {
			t.Errorf("output = %q, want %q", output.String(), text)
		}
	}
	if strings.Contains(output.String(), "new-private-pat") || !strings.Contains(feedback.String(), "Checking Confluence") {
		t.Fatalf("output/feedback leaked or missed progress: %q / %q", output.String(), feedback.String())
	}
}

func TestExecuteWorkspaceSetupReusesExistingConfluencePATAndClearsDefaults(t *testing.T) {
	dependencies := newSetupDependencies()
	config := dependencies.config.(*fakeSetupConfig)
	config.confluenceValues = map[string]string{
		"base_url":  "https://wiki.example/confluence",
		"space":     "DOC",
		"secret":    "secret:team.pat",
		"auth_type": "pat",
		"root_page": "123",
	}
	dependencies.secrets.(*fakeSetupSecrets).values["team.pat"] = "existing-pat"
	pickerFake := dependencies.picker.(*fakeSetupPicker)
	pickerFake.providers = [][]string{{"confluence"}}
	pickerFake.confirms = []bool{true, false}
	if err := executeWorkspaceSetup(context.Background(), "workspace", &bytes.Buffer{}, &bytes.Buffer{}, dependencies); err != nil {
		t.Fatalf("executeWorkspaceSetup() error = %v", err)
	}
	client := dependencies.confluence.(*fakeSetupConfluence)
	if client.pat != "existing-pat" || client.validated.RootPage != "" || len(pickerFake.secretPrompts) != 0 {
		t.Fatalf("reuse = PAT %q config %#v secret prompts %v", client.pat, client.validated, pickerFake.secretPrompts)
	}
	wantUnset := []string{confluenceintegration.AuthTypeKey, confluenceintegration.RootPageKey}
	if !reflect.DeepEqual(config.unsetValues, wantUnset) {
		t.Fatalf("unset values = %v, want %v", config.unsetValues, wantUnset)
	}
}

func TestExecuteWorkspaceSetupConfluenceValidationFailurePersistsNothing(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want string
	}{
		{name: "authentication", err: confluenceintegration.ErrAuthentication, want: "Confluence authentication failed for https://wiki.example."},
		{name: "forbidden", err: confluenceintegration.ErrForbidden, want: "Confluence authentication failed for https://wiki.example."},
		{name: "space", err: confluenceintegration.ErrSpaceNotFound, want: `Confluence space "DOC" was not found or is not accessible.`},
	} {
		t.Run(test.name, func(t *testing.T) {
			dependencies := newSetupDependencies()
			dependencies.confluence.(*fakeSetupConfluence).validateErr = test.err
			pickerFake := dependencies.picker.(*fakeSetupPicker)
			pickerFake.providers = [][]string{{"confluence"}}
			pickerFake.inputs = []string{"https://wiki.example", "DOC"}
			pickerFake.secrets = []string{"candidate-pat"}
			err := executeWorkspaceSetup(context.Background(), "workspace", &bytes.Buffer{}, &bytes.Buffer{}, dependencies)
			if !errors.Is(err, test.err) || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("executeWorkspaceSetup() error = %v, want %q preserving %v", err, test.want, test.err)
			}
			if len(dependencies.config.(*fakeSetupConfig).setValues) != 0 || len(dependencies.secrets.(*fakeSetupSecrets).sets) != 0 {
				t.Fatal("failed validation persisted configuration or PAT")
			}
		})
	}
}

func TestExecuteWorkspaceSetupProcessesProvidersInFixedOrder(t *testing.T) {
	dependencies := newSetupDependencies()
	dependencies.github.(*fakeSetupGitHub).owners = []githubintegration.Owner{{Login: "team"}}
	pickerFake := dependencies.picker.(*fakeSetupPicker)
	pickerFake.providers = [][]string{{"confluence", "github"}}
	pickerFake.one = []string{"github.com", "team"}
	pickerFake.inputs = []string{"https://wiki.example", "DOC"}
	pickerFake.secrets = []string{"pat"}
	var order []string
	dependencies.github.(*fakeSetupGitHub).beforeValidate = func() { order = append(order, "github") }
	dependencies.confluence.(*fakeSetupConfluence).beforeValidate = func() { order = append(order, "confluence") }
	if err := executeWorkspaceSetup(context.Background(), "workspace", &bytes.Buffer{}, &bytes.Buffer{}, dependencies); err != nil {
		t.Fatalf("executeWorkspaceSetup() error = %v", err)
	}
	if !reflect.DeepEqual(order, []string{"github", "confluence"}) {
		t.Fatalf("validation order = %v", order)
	}
}

func newSetupDependencies() workspaceSetupDependencies {
	return workspaceSetupDependencies{
		workspaces:   &fakeSetupWorkspace{item: workspace.Workspace{ID: "workspace-id", Name: "workspace", LocalPath: "/work/workspace"}},
		config:       &fakeSetupConfig{setValues: make(map[string]string)},
		github:       &fakeSetupGitHub{},
		confluence:   &fakeSetupConfluence{},
		secrets:      &fakeSetupSecrets{values: make(map[string]string)},
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
	values           map[string]string
	confluenceValues map[string]string
	setValues        map[string]string
	unsetValues      []string
}

func (f *fakeSetupConfig) Namespace(_ context.Context, _ string, namespace string) (map[string]string, error) {
	source := f.values
	if namespace == "confluence" {
		source = f.confluenceValues
	}
	values := make(map[string]string, len(source))
	for key, value := range source {
		values[key] = value
	}
	return values, nil
}

func (f *fakeSetupConfig) SetUser(_ context.Context, _ string, key, value string) (workspace.Workspace, error) {
	f.setValues[key] = value
	return workspace.Workspace{}, nil
}

func (f *fakeSetupConfig) UnsetUser(_ context.Context, _ string, key string) (workspace.Workspace, error) {
	f.unsetValues = append(f.unsetValues, key)
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

type fakeSetupConfluence struct {
	validated      confluenceintegration.Config
	pat            string
	validateErr    error
	validateCalls  int
	beforeValidate func()
}

func (f *fakeSetupConfluence) Validate(_ context.Context, config confluenceintegration.Config, pat string) error {
	if f.beforeValidate != nil {
		f.beforeValidate()
	}
	f.validateCalls++
	f.validated = config
	f.pat = pat
	return f.validateErr
}

type fakeSetupSecrets struct {
	values map[string]string
	sets   map[string]string
	err    error
}

func (f *fakeSetupSecrets) Get(_ context.Context, key string) (secret.Secret, error) {
	if f.err != nil {
		return secret.Secret{}, f.err
	}
	value, exists := f.values[key]
	if !exists {
		return secret.Secret{}, secret.ErrNotFound
	}
	return secret.Secret{Key: key, Value: value}, nil
}

func (f *fakeSetupSecrets) Set(_ context.Context, key, value string) error {
	if f.sets == nil {
		f.sets = make(map[string]string)
	}
	f.sets[key] = value
	return nil
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
	one             []string
	oneErrors       []error
	many            [][]string
	manyErrors      []error
	providers       [][]string
	providerErrors  []error
	confirms        []bool
	confirmErrs     []error
	inputs          []string
	inputErrors     []error
	secrets         []string
	secretErrors    []error
	confirmPrompts  []string
	confirmDefaults []bool
	inputPrompts    []string
	inputDefaults   []string
	secretPrompts   []string
	oneOptions      [][]picker.Option
	oneDefaults     []string
	manyOptions     [][]picker.Option
	providerOptions [][]picker.Option
	manyCalls       int
	providerCalls   int
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

func (f *fakeSetupPicker) PickMany(_ context.Context, title string, options []picker.Option) ([]string, error) {
	if title == "Integrations" {
		f.providerCalls++
		f.providerOptions = append(f.providerOptions, append([]picker.Option(nil), options...))
		values := []string{"github"}
		if len(f.providers) > 0 {
			values, f.providers = f.providers[0], f.providers[1:]
		}
		var err error
		if len(f.providerErrors) > 0 {
			err, f.providerErrors = f.providerErrors[0], f.providerErrors[1:]
		}
		return append([]string(nil), values...), err
	}
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

func (f *fakeSetupPicker) Confirm(prompt string, defaultValue bool) (bool, error) {
	f.confirmPrompts = append(f.confirmPrompts, prompt)
	f.confirmDefaults = append(f.confirmDefaults, defaultValue)
	var value bool
	if len(f.confirms) > 0 {
		value, f.confirms = f.confirms[0], f.confirms[1:]
	} else {
		value = defaultValue
	}
	var err error
	if len(f.confirmErrs) > 0 {
		err, f.confirmErrs = f.confirmErrs[0], f.confirmErrs[1:]
	}
	return value, err
}

func (f *fakeSetupPicker) Input(prompt, defaultValue string) (string, error) {
	f.inputPrompts = append(f.inputPrompts, prompt)
	f.inputDefaults = append(f.inputDefaults, defaultValue)
	value := defaultValue
	if len(f.inputs) > 0 {
		value, f.inputs = f.inputs[0], f.inputs[1:]
	}
	var err error
	if len(f.inputErrors) > 0 {
		err, f.inputErrors = f.inputErrors[0], f.inputErrors[1:]
	}
	return value, err
}

func (f *fakeSetupPicker) Secret(prompt string) (string, error) {
	f.secretPrompts = append(f.secretPrompts, prompt)
	var value string
	if len(f.secrets) > 0 {
		value, f.secrets = f.secrets[0], f.secrets[1:]
	}
	var err error
	if len(f.secretErrors) > 0 {
		err, f.secretErrors = f.secretErrors[0], f.secretErrors[1:]
	}
	return value, err
}

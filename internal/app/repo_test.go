package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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

func TestExecuteRepoRefresh(t *testing.T) {
	service := &fakeRepoService{refreshCount: 24}
	var output bytes.Buffer
	if err := executeRepo(context.Background(), []string{"refresh", "workspace"}, &output, service, &fakeSetupPicker{}); err != nil {
		t.Fatalf("executeRepo(refresh) error = %v", err)
	}
	if got := output.String(); got != "Repositories refreshed: 24\n" {
		t.Errorf("refresh output = %q", got)
	}
}

func TestExecuteRepoList(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		var output bytes.Buffer
		if err := executeRepo(context.Background(), []string{"list", "workspace"}, &output, &fakeRepoService{}, &fakeSetupPicker{}); err != nil {
			t.Fatalf("executeRepo(list) error = %v", err)
		}
		if output.String() != "No repositories found.\n" {
			t.Errorf("empty list output = %q", output.String())
		}
	})

	t.Run("states", func(t *testing.T) {
		service := &fakeRepoService{listed: []repositorydomain.Listed{
			{Repository: repositorydomain.Repository{ExternalKey: "team/api", CheckoutPath: "/workspace/repos/api"}, State: repositorydomain.StateCloned},
			{Repository: repositorydomain.Repository{ExternalKey: "team/web"}, State: repositorydomain.StateNotCloned},
		}}
		var output bytes.Buffer
		if err := executeRepo(context.Background(), []string{"list", "workspace"}, &output, service, &fakeSetupPicker{}); err != nil {
			t.Fatalf("executeRepo(list) error = %v", err)
		}
		fields := strings.Fields(output.String())
		want := []string{"REPOSITORY", "STATUS", "PATH", "team/api", "cloned", "/workspace/repos/api", "team/web", "not-cloned", "-"}
		if !reflect.DeepEqual(fields, want) {
			t.Errorf("list fields = %v, want %v", fields, want)
		}
	})
}

func TestExecuteRepoClonePrintsResultsAndReturnsFailure(t *testing.T) {
	cloneErr := errors.New("repository clone failed for 1 repository(s)")
	service := &fakeRepoService{
		cloneErr: cloneErr,
		cloneResults: []repositorydomain.CloneResult{
			{Repository: "team/api", Status: "cloned"},
			{Repository: "team/web", Error: errors.New("destination conflict")},
		},
	}
	var output bytes.Buffer
	err := executeRepo(context.Background(), []string{"clone", "workspace", "--all"}, &output, service, &fakeSetupPicker{})
	if !errors.Is(err, cloneErr) {
		t.Fatalf("executeRepo(clone) error = %v, want clone error", err)
	}
	fields := strings.Fields(output.String())
	want := []string{"team/api", "cloned", "team/web", "failed:", "destination", "conflict"}
	if !reflect.DeepEqual(fields, want) {
		t.Errorf("clone fields = %v, want %v", fields, want)
	}
	if !service.cloneAll || service.cloneWorkspace != "workspace" || service.cloneSelectors != nil {
		t.Errorf("clone request = all %v workspace %q selectors %v", service.cloneAll, service.cloneWorkspace, service.cloneSelectors)
	}
}

func TestExecuteRepoPickRefreshesBeforePrompt(t *testing.T) {
	refreshErr := errors.New("refresh failed")
	service := &fakeRepoService{refreshErr: refreshErr}
	pickerFake := &fakeSetupPicker{many: [][]string{{"team/api"}}}
	err := executeRepo(context.Background(), []string{"pick", "workspace"}, &bytes.Buffer{}, service, pickerFake)
	if !errors.Is(err, refreshErr) {
		t.Fatalf("executeRepo(pick) error = %v, want refresh error", err)
	}
	if pickerFake.manyCalls != 0 || service.listCalls != 0 || service.cloneKnownCalls != 0 {
		t.Fatalf("pick continued after refresh error: picker=%d list=%d clone=%d", pickerFake.manyCalls, service.listCalls, service.cloneKnownCalls)
	}
}

func TestExecuteRepoPickPresentsIncompleteConfigGuidance(t *testing.T) {
	service := &fakeRepoService{refreshErr: fmt.Errorf("refresh: %w", githubintegration.ErrIncompleteConfig)}
	pickerFake := &fakeSetupPicker{}
	err := executeRepo(context.Background(), []string{"pick", "workspace"}, &bytes.Buffer{}, service, pickerFake)
	want := "GitHub configuration is incomplete for workspace \"workspace\".\n\nMissing:\n  github.org\n\nConfigure with:\n  devdash workspace setup workspace"
	if !errors.Is(err, githubintegration.ErrIncompleteConfig) || err.Error() != want {
		t.Fatalf("executeRepo(pick) error = %v, want exact setup guidance", err)
	}
	if pickerFake.manyCalls != 0 {
		t.Fatal("pick prompted before configuration succeeded")
	}
}

func TestExecuteRepoPickCancellationAndEmptySelection(t *testing.T) {
	for _, test := range []struct {
		name   string
		values [][]string
		errors []error
	}{
		{name: "cancelled", errors: []error{picker.ErrCancelled}},
		{name: "empty", values: [][]string{{}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &fakeRepoService{}
			pickerFake := &fakeSetupPicker{many: test.values, manyErrors: test.errors}
			var output bytes.Buffer
			if err := executeRepo(context.Background(), []string{"pick", "workspace"}, &output, service, pickerFake); err != nil {
				t.Fatalf("executeRepo(pick) error = %v", err)
			}
			if output.String() != "No repositories selected.\n" {
				t.Errorf("output = %q, want no-selection message", output.String())
			}
			if service.cloneKnownCalls != 0 {
				t.Fatal("pick cloned after cancellation or empty selection")
			}
		})
	}
}

func TestExecuteRepoPickMapsExternalKeysAndPrintsIndependentResults(t *testing.T) {
	cloneErr := errors.New("aggregate clone failure")
	service := &fakeRepoService{
		listed: []repositorydomain.Listed{
			{Repository: repositorydomain.Repository{ExternalKey: "team/api"}, State: repositorydomain.StateCloned},
			{Repository: repositorydomain.Repository{ExternalKey: "team/web"}, State: repositorydomain.StateMissing},
		},
		cloneKnownResults: []repositorydomain.CloneResult{
			{Repository: "team/api", Status: "already cloned"},
			{Repository: "team/web", Status: "restored"},
			{Repository: "team/broken", Error: errors.New("clone failed")},
		},
		cloneKnownErr: cloneErr,
	}
	pickerFake := &fakeSetupPicker{many: [][]string{{"team/web", "team/api"}}}
	var output bytes.Buffer

	err := executeRepo(context.Background(), []string{"pick", "workspace"}, &output, service, pickerFake)
	if !errors.Is(err, cloneErr) {
		t.Fatalf("executeRepo(pick) error = %v, want clone error", err)
	}
	if service.refreshCalls != 1 || service.listCalls != 1 || service.cloneKnownCalls != 1 || service.cloneKnownWorkspace != "workspace" {
		t.Fatalf("pick calls = refresh %d list %d clone %d workspace %q", service.refreshCalls, service.listCalls, service.cloneKnownCalls, service.cloneKnownWorkspace)
	}
	wantSelectors := []string{"team/web", "team/api"}
	if !reflect.DeepEqual(service.cloneKnownSelectors, wantSelectors) {
		t.Errorf("CloneKnown selectors = %#v, want %#v", service.cloneKnownSelectors, wantSelectors)
	}
	if len(pickerFake.manyOptions) != 1 || pickerFake.manyOptions[0][0].Value != "team/api" || !strings.Contains(pickerFake.manyOptions[0][1].Label, "missing") {
		t.Errorf("picker options = %#v, want state labels with external-key values", pickerFake.manyOptions)
	}
	for _, text := range []string{"team/api", "already cloned", "team/web", "restored", "team/broken", "failed: clone failed"} {
		if !strings.Contains(output.String(), text) {
			t.Errorf("output = %q, want containing %q", output.String(), text)
		}
	}
}

func TestRepoCommandsValidateBeforeDatabaseOpen(t *testing.T) {
	for _, test := range []struct {
		args    []string
		wantErr string
	}{
		{args: []string{"repo"}, wantErr: "repo command required: refresh, list, clone, or pick"},
		{args: []string{"repo", "unknown"}, wantErr: "unknown repo command: unknown"},
		{args: []string{"repo", "refresh"}, wantErr: "usage: devdash repo refresh <workspace>"},
		{args: []string{"repo", "refresh", "ws", "extra"}, wantErr: "usage: devdash repo refresh <workspace>"},
		{args: []string{"repo", "list"}, wantErr: "usage: devdash repo list <workspace>"},
		{args: []string{"repo", "pick"}, wantErr: "usage: devdash repo pick <workspace>"},
		{args: []string{"repo", "pick", "ws", "extra"}, wantErr: "usage: devdash repo pick <workspace>"},
		{args: []string{"repo", "clone", "ws"}, wantErr: "usage: devdash repo clone <workspace> --all|<repo> [<repo>...]"},
		{args: []string{"repo", "clone", "ws", "--all", "api"}, wantErr: "usage: devdash repo clone <workspace> --all|<repo> [<repo>...]"},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			databasePath := filepath.Join(t.TempDir(), "devdash.db")
			t.Setenv("DEVDASH_DB", databasePath)
			var output bytes.Buffer
			err := run(context.Background(), test.args, strings.NewReader(""), &output)
			if err == nil || err.Error() != test.wantErr {
				t.Fatalf("run(%v) error = %v, want %q", test.args, err, test.wantErr)
			}
			if _, statErr := os.Stat(databasePath); !os.IsNotExist(statErr) {
				t.Fatalf("database stat error = %v, want missing", statErr)
			}
		})
	}
}

func TestRunRepoRejectsMissingAndInvalidConfigBeforeGitHub(t *testing.T) {
	for _, test := range []struct {
		name      string
		configure []string
		want      error
	}{
		{name: "missing", want: githubintegration.ErrIncompleteConfig},
		{name: "invalid", configure: []string{"workspace", "config", "set", "workspace", "github.base_url", "foo"}, want: githubintegration.ErrInvalidConfig},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("DEVDASH_DB", filepath.Join(t.TempDir(), "devdash.db"))
			runAppCommand(t, context.Background(), "workspace", "add", "workspace", t.TempDir())
			if len(test.configure) > 0 {
				runAppCommand(t, context.Background(), test.configure...)
			}
			if test.name == "invalid" {
				runAppCommand(t, context.Background(), "workspace", "config", "set", "workspace", "github.org", "team")
			}
			_, err := runAppCommandError(context.Background(), "repo", "refresh", "workspace")
			if !errors.Is(err, test.want) {
				t.Fatalf("repo refresh error = %v, want %v", err, test.want)
			}
		})
	}
}

type fakeRepoService struct {
	refreshCount        int
	refreshErr          error
	refreshCalls        int
	listErr             error
	listCalls           int
	listed              []repositorydomain.Listed
	cloneResults        []repositorydomain.CloneResult
	cloneErr            error
	cloneAll            bool
	cloneWorkspace      string
	cloneSelectors      []string
	cloneKnownResults   []repositorydomain.CloneResult
	cloneKnownErr       error
	cloneKnownCalls     int
	cloneKnownWorkspace string
	cloneKnownSelectors []string
}

func (s *fakeRepoService) Refresh(context.Context, string) (workspace.Workspace, int, error) {
	s.refreshCalls++
	return workspace.Workspace{}, s.refreshCount, s.refreshErr
}
func (s *fakeRepoService) List(context.Context, string) (workspace.Workspace, []repositorydomain.Listed, error) {
	s.listCalls++
	return workspace.Workspace{}, append([]repositorydomain.Listed(nil), s.listed...), s.listErr
}
func (s *fakeRepoService) Clone(_ context.Context, workspaceIdentifier string, selectors []string, all bool) (workspace.Workspace, []repositorydomain.CloneResult, error) {
	s.cloneWorkspace = workspaceIdentifier
	s.cloneSelectors = append([]string(nil), selectors...)
	s.cloneAll = all
	return workspace.Workspace{}, append([]repositorydomain.CloneResult(nil), s.cloneResults...), s.cloneErr
}
func (s *fakeRepoService) CloneKnown(_ context.Context, workspaceIdentifier string, selectors []string) (workspace.Workspace, []repositorydomain.CloneResult, error) {
	s.cloneKnownCalls++
	s.cloneKnownWorkspace = workspaceIdentifier
	s.cloneKnownSelectors = append([]string(nil), selectors...)
	return workspace.Workspace{}, append([]repositorydomain.CloneResult(nil), s.cloneKnownResults...), s.cloneKnownErr
}

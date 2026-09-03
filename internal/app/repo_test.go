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
	"github.com/ngochc/dev-dash/internal/workspace"
)

func TestExecuteRepoRefresh(t *testing.T) {
	service := &fakeRepoService{refreshCount: 24}
	var output bytes.Buffer
	if err := executeRepo(context.Background(), []string{"refresh", "workspace"}, &output, service); err != nil {
		t.Fatalf("executeRepo(refresh) error = %v", err)
	}
	if got := output.String(); got != "Repositories refreshed: 24\n" {
		t.Errorf("refresh output = %q", got)
	}
}

func TestExecuteRepoList(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		var output bytes.Buffer
		if err := executeRepo(context.Background(), []string{"list", "workspace"}, &output, &fakeRepoService{}); err != nil {
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
		if err := executeRepo(context.Background(), []string{"list", "workspace"}, &output, service); err != nil {
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
	err := executeRepo(context.Background(), []string{"clone", "workspace", "--all"}, &output, service)
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

func TestRepoCommandsValidateBeforeDatabaseOpen(t *testing.T) {
	for _, test := range []struct {
		args    []string
		wantErr string
	}{
		{args: []string{"repo"}, wantErr: "repo command required: refresh, list, or clone"},
		{args: []string{"repo", "unknown"}, wantErr: "unknown repo command: unknown"},
		{args: []string{"repo", "refresh"}, wantErr: "usage: devdash repo refresh <workspace>"},
		{args: []string{"repo", "refresh", "ws", "extra"}, wantErr: "usage: devdash repo refresh <workspace>"},
		{args: []string{"repo", "list"}, wantErr: "usage: devdash repo list <workspace>"},
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
	refreshCount   int
	listed         []repositorydomain.Listed
	cloneResults   []repositorydomain.CloneResult
	cloneErr       error
	cloneAll       bool
	cloneWorkspace string
	cloneSelectors []string
}

func (s *fakeRepoService) Refresh(context.Context, string) (workspace.Workspace, int, error) {
	return workspace.Workspace{}, s.refreshCount, nil
}
func (s *fakeRepoService) List(context.Context, string) (workspace.Workspace, []repositorydomain.Listed, error) {
	return workspace.Workspace{}, append([]repositorydomain.Listed(nil), s.listed...), nil
}
func (s *fakeRepoService) Clone(_ context.Context, workspaceIdentifier string, selectors []string, all bool) (workspace.Workspace, []repositorydomain.CloneResult, error) {
	s.cloneWorkspace = workspaceIdentifier
	s.cloneSelectors = append([]string(nil), selectors...)
	s.cloneAll = all
	return workspace.Workspace{}, append([]repositorydomain.CloneResult(nil), s.cloneResults...), s.cloneErr
}

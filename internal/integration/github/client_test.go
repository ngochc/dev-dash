package github

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"testing"
)

func TestClientValidateAuthentication(t *testing.T) {
	runner := &fakeCommandRunner{}
	client := NewClient(runner)
	config := Config{Host: "github.com"}
	if err := client.Validate(context.Background(), config); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	call := runner.calls[0]
	if call.environment != nil {
		t.Errorf("github.com environment = %v, want nil", call.environment)
	}
	wantArgs := []string{"auth", "status", "--hostname", "github.com"}
	if !reflect.DeepEqual(call.args, wantArgs) {
		t.Errorf("Validate() args = %v, want %v", call.args, wantArgs)
	}
}

func TestClientValidateAuthenticationFailure(t *testing.T) {
	runner := &fakeCommandRunner{results: []fakeCommandResult{{stderr: "not logged in", err: errors.New("exit 1")}}}
	err := NewClient(runner).Validate(context.Background(), Config{Host: "git.example.com"})
	if !errors.Is(err, ErrAuthentication) {
		t.Fatalf("Validate() error = %v, want ErrAuthentication", err)
	}
	want := "GitHub CLI is not authenticated for git.example.com.\n\nAuthenticate with:\n  gh auth login --hostname git.example.com: not logged in"
	if err.Error() != want {
		t.Errorf("Validate() error = %q, want %q", err, want)
	}
	if got := runner.calls[0].environment["GH_HOST"]; got != "git.example.com" {
		t.Errorf("GH_HOST = %q, want git.example.com", got)
	}
}

func TestClientValidateUnavailable(t *testing.T) {
	runner := &fakeCommandRunner{results: []fakeCommandResult{{err: exec.ErrNotFound}}}
	err := NewClient(runner).Validate(context.Background(), Config{Host: "github.com"})
	if !errors.Is(err, ErrCLIUnavailable) {
		t.Fatalf("Validate() error = %v, want ErrCLIUnavailable", err)
	}
}

func TestClientDiscoverRepositories(t *testing.T) {
	payload := `[
		{"id":"R2","name":"web","nameWithOwner":"team/web","url":"https://git.example.com/team/web","sshUrl":"git@git.example.com:team/web.git","isArchived":false,"isFork":false,"defaultBranchRef":{"name":"main"}},
		{"id":"R1","name":"api","nameWithOwner":"team/api","url":"https://git.example.com/team/api","sshUrl":"git@git.example.com:team/api.git","isArchived":false,"isFork":true,"defaultBranchRef":null}
	]`
	runner := &fakeCommandRunner{results: []fakeCommandResult{{stdout: payload}}}
	config := Config{Host: "git.example.com", Organization: "team"}
	repositories, err := NewClient(runner).Discover(context.Background(), config)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(repositories) != 2 || repositories[0].NameWithOwner != "team/api" || repositories[1].NameWithOwner != "team/web" {
		t.Errorf("Discover() = %#v, want sorted repositories", repositories)
	}
	call := runner.calls[0]
	wantArgs := []string{
		"repo", "list", "team", "--limit", "1000", "--no-archived", "--json",
		"id,name,nameWithOwner,url,sshUrl,isArchived,isFork,defaultBranchRef",
	}
	if !reflect.DeepEqual(call.args, wantArgs) {
		t.Errorf("Discover() args = %v, want %v", call.args, wantArgs)
	}
	if call.environment["GH_HOST"] != "git.example.com" {
		t.Errorf("Discover() environment = %v, want GH_HOST", call.environment)
	}
}

func TestClientCloneRepository(t *testing.T) {
	runner := &fakeCommandRunner{}
	config := Config{Host: "github.com"}
	if err := NewClient(runner).Clone(context.Background(), config, "team/api", "/workspace/repos/api"); err != nil {
		t.Fatalf("Clone() error = %v", err)
	}
	want := []string{"repo", "clone", "team/api", "/workspace/repos/api"}
	if !reflect.DeepEqual(runner.calls[0].args, want) {
		t.Errorf("Clone() args = %v, want %v", runner.calls[0].args, want)
	}
}

type fakeCommandRunner struct {
	results []fakeCommandResult
	calls   []fakeCommandCall
}

type fakeCommandResult struct {
	stdout string
	stderr string
	err    error
}

type fakeCommandCall struct {
	environment map[string]string
	args        []string
}

func (r *fakeCommandRunner) Run(_ context.Context, environment map[string]string, args ...string) (CommandResult, error) {
	copiedEnvironment := make(map[string]string, len(environment))
	for key, value := range environment {
		copiedEnvironment[key] = value
	}
	if environment == nil {
		copiedEnvironment = nil
	}
	r.calls = append(r.calls, fakeCommandCall{
		environment: copiedEnvironment,
		args:        append([]string(nil), args...),
	})
	if len(r.results) == 0 {
		return CommandResult{}, nil
	}
	result := r.results[0]
	r.results = r.results[1:]
	return CommandResult{Stdout: []byte(result.stdout), Stderr: []byte(result.stderr)}, result.err
}

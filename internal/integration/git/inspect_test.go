package git

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ngochc/dev-dash/internal/repository"
)

func TestInspectorMissingPath(t *testing.T) {
	runner := &fakeGitRunner{}
	inspection, err := NewInspector(runner).Inspect(context.Background(), filepath.Join(t.TempDir(), "missing"), "https://github.com/org/repo")
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if inspection != (repository.CheckoutInspection{}) {
		t.Errorf("Inspect() = %#v, want missing", inspection)
	}
	if len(runner.calls) != 0 {
		t.Fatal("Inspect() invoked git for a missing path")
	}
}

func TestInspectorValidCheckout(t *testing.T) {
	path := t.TempDir()
	runner := &fakeGitRunner{outputs: []fakeGitOutput{
		{output: path + "\n"},
		{output: "git@github.com:org/repo.git\n"},
	}}
	inspection, err := NewInspector(runner).Inspect(context.Background(), path, "https://github.com/org/repo")
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if inspection != (repository.CheckoutInspection{Exists: true, Valid: true}) {
		t.Errorf("Inspect() = %#v, want valid", inspection)
	}
	wantCalls := [][]string{
		{"-C", path, "rev-parse", "--show-toplevel"},
		{"-C", path, "remote", "get-url", "origin"},
	}
	if !reflect.DeepEqual(runner.calls, wantCalls) {
		t.Errorf("git calls = %v, want %v", runner.calls, wantCalls)
	}
}

func TestInspectorAcceptsSymlinkedCheckoutPath(t *testing.T) {
	realPath := t.TempDir()
	linkedPath := filepath.Join(t.TempDir(), "checkout")
	if err := os.Symlink(realPath, linkedPath); err != nil {
		t.Fatalf("create checkout symlink: %v", err)
	}
	runner := &fakeGitRunner{outputs: []fakeGitOutput{
		{output: realPath + "\n"},
		{output: "https://github.com/org/repo\n"},
	}}
	inspection, err := NewInspector(runner).Inspect(context.Background(), linkedPath, "https://github.com/org/repo")
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if inspection != (repository.CheckoutInspection{Exists: true, Valid: true}) {
		t.Errorf("Inspect() = %#v, want valid symlinked checkout", inspection)
	}
}

func TestInspectorInvalidCheckout(t *testing.T) {
	path := t.TempDir()
	otherPath := t.TempDir()
	for _, outputs := range [][]fakeGitOutput{
		{{err: errors.New("not a repository")}},
		{{output: path}, {output: "https://github.com/other/repo"}},
		{{output: otherPath}},
	} {
		runner := &fakeGitRunner{outputs: outputs}
		inspection, err := NewInspector(runner).Inspect(context.Background(), path, "https://github.com/org/repo")
		if err != nil {
			t.Fatalf("Inspect() error = %v", err)
		}
		if inspection != (repository.CheckoutInspection{Exists: true}) {
			t.Errorf("Inspect() = %#v, want invalid", inspection)
		}
	}
}

func TestInspectorReportsMissingGit(t *testing.T) {
	path := t.TempDir()
	runner := &fakeGitRunner{outputs: []fakeGitOutput{{err: exec.ErrNotFound}}}
	_, err := NewInspector(runner).Inspect(context.Background(), path, "https://github.com/org/repo")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Inspect() error = %v, want ErrUnavailable", err)
	}
}

type fakeGitRunner struct {
	outputs []fakeGitOutput
	calls   [][]string
}

type fakeGitOutput struct {
	output string
	err    error
}

func (r *fakeGitRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	r.calls = append(r.calls, append([]string(nil), args...))
	if len(r.outputs) == 0 {
		return nil, nil
	}
	output := r.outputs[0]
	r.outputs = r.outputs[1:]
	return []byte(output.output), output.err
}

package platform

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditTemporaryFileUsesVisualAndWiresProcess(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("TMPDIR", tempDir)
	t.Setenv("GO_WANT_EDITOR_HELPER", "1")
	t.Setenv("VISUAL", editorHelperCommand("edit", "--wait", "session"))
	t.Setenv("EDITOR", editorHelperCommand("fail"))

	var output bytes.Buffer
	edited, err := EditTemporaryFile(t.Context(), "devdash-workspace-*.conf", []byte("initial\n"), strings.NewReader("editor input"), &output)
	if err != nil {
		t.Fatalf("EditTemporaryFile() error = %v", err)
	}
	if got := string(edited); got != "edited=initial\n" {
		t.Errorf("EditTemporaryFile() = %q, want edited bytes", got)
	}
	for _, want := range []string{"stdout args=--wait,session", "stderr input=editor input", "initial=initial\n", "mode=0600"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("editor output = %q, want containing %q", output.String(), want)
		}
	}
	assertEditorTempDirEmpty(t, tempDir)
}

func TestEditTemporaryFileFallsBackToEditor(t *testing.T) {
	t.Setenv("GO_WANT_EDITOR_HELPER", "1")
	t.Setenv("VISUAL", " \t ")
	t.Setenv("EDITOR", editorHelperCommand("edit", "fallback"))

	var output bytes.Buffer
	if _, err := EditTemporaryFile(t.Context(), "devdash-workspace-*.conf", nil, strings.NewReader(""), &output); err != nil {
		t.Fatalf("EditTemporaryFile() error = %v", err)
	}
	if !strings.Contains(output.String(), "stdout args=fallback") {
		t.Errorf("editor output = %q, want EDITOR helper output", output.String())
	}
}

func TestEditTemporaryFileRequiresConfiguredEditorBeforeCreatingFile(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("TMPDIR", tempDir)
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")

	_, err := EditTemporaryFile(t.Context(), "devdash-workspace-*.conf", nil, strings.NewReader(""), io.Discard)
	if err == nil || err.Error() != "no editor configured; set $VISUAL or $EDITOR" {
		t.Fatalf("EditTemporaryFile() error = %v, want missing-editor error", err)
	}
	assertEditorTempDirEmpty(t, tempDir)
}

func TestEditTemporaryFileReturnsEditorFailureAndCleansUp(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("TMPDIR", tempDir)
	t.Setenv("GO_WANT_EDITOR_HELPER", "1")
	t.Setenv("VISUAL", editorHelperCommand("fail"))
	t.Setenv("EDITOR", "")

	_, err := EditTemporaryFile(t.Context(), "devdash-workspace-*.conf", []byte("initial"), strings.NewReader(""), io.Discard)
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) || exitError.ExitCode() != 12 {
		t.Fatalf("EditTemporaryFile() error = %v, want wrapped exit status 12", err)
	}
	if !strings.HasPrefix(err.Error(), "run editor: ") {
		t.Errorf("EditTemporaryFile() error = %q, want run context", err)
	}
	assertEditorTempDirEmpty(t, tempDir)
}

func TestEditTemporaryFilePropagatesCanceledContext(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("TMPDIR", tempDir)
	t.Setenv("GO_WANT_EDITOR_HELPER", "1")
	t.Setenv("VISUAL", editorHelperCommand("edit"))
	t.Setenv("EDITOR", "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := EditTemporaryFile(ctx, "devdash-workspace-*.conf", nil, strings.NewReader(""), io.Discard)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("EditTemporaryFile() error = %v, want context.Canceled", err)
	}
	assertEditorTempDirEmpty(t, tempDir)
}

func TestEditorHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_EDITOR_HELPER") != "1" {
		return
	}
	separator := -1
	for i, argument := range os.Args {
		if argument == "--" {
			separator = i
			break
		}
	}
	if separator == -1 || len(os.Args) < separator+3 {
		os.Exit(20)
	}
	arguments := os.Args[separator+1:]
	mode := arguments[0]
	path := arguments[len(arguments)-1]
	if mode == "fail" {
		os.Exit(12)
	}
	if mode != "edit" {
		os.Exit(21)
	}

	info, err := os.Stat(path)
	if err != nil {
		os.Exit(22)
	}
	initial, err := os.ReadFile(path)
	if err != nil {
		os.Exit(23)
	}
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(24)
	}
	fmt.Printf("stdout args=%s\n", strings.Join(arguments[1:len(arguments)-1], ","))
	fmt.Printf("initial=%s", initial)
	fmt.Printf("mode=%04o\n", info.Mode().Perm())
	fmt.Fprintf(os.Stderr, "stderr input=%s\n", input)
	if err := os.WriteFile(path, []byte("edited=initial\n"), 0o600); err != nil {
		os.Exit(25)
	}
	os.Exit(0)
}

func editorHelperCommand(mode string, arguments ...string) string {
	parts := []string{os.Args[0], "-test.run=^TestEditorHelperProcess$", "--", mode}
	parts = append(parts, arguments...)
	return strings.Join(parts, " ")
}

func assertEditorTempDirEmpty(t *testing.T, directory string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Clean(directory))
	if err != nil {
		t.Fatalf("read temp directory: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("temp directory contains %v, want empty", entries)
	}
}

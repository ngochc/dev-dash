package platform

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateRunsInstallerForResolvedExecutable(t *testing.T) {
	realDir := filepath.Join(t.TempDir(), "real", "bin")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatalf("create real executable directory: %v", err)
	}
	realExecutable := filepath.Join(realDir, "devdash")
	if err := os.WriteFile(realExecutable, []byte("fixture"), 0o755); err != nil {
		t.Fatalf("create real executable: %v", err)
	}

	linkDir := filepath.Join(t.TempDir(), "link")
	if err := os.MkdirAll(linkDir, 0o755); err != nil {
		t.Fatalf("create link directory: %v", err)
	}
	linkedExecutable := filepath.Join(linkDir, "devdash")
	if err := os.Symlink(realExecutable, linkedExecutable); err != nil {
		t.Fatalf("create executable symlink: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			t.Errorf("request method = %q, want GET", request.Method)
		}
		io.WriteString(response, `#!/bin/sh
printf 'install=%s\n' "$DEVDASH_INSTALL_DIR"
printf 'version=%s\n' "$DEVDASH_VERSION"
printf 'unrelated=%s\n' "$DEVDASH_UPDATE_TEST"
printf 'installer stderr\n' >&2
`)
	}))
	defer server.Close()

	t.Setenv("DEVDASH_INSTALL_DIR", "wrong-install-dir")
	t.Setenv("DEVDASH_VERSION", "v0.0.0")
	t.Setenv("DEVDASH_UPDATE_TEST", "preserved")

	resolvedExecutable, err := filepath.EvalSymlinks(realExecutable)
	if err != nil {
		t.Fatalf("resolve fixture executable: %v", err)
	}
	wantOutput := "Updating Devdash...\n" +
		"install=" + filepath.Dir(resolvedExecutable) + "\n" +
		"version=latest\n" +
		"unrelated=preserved\n" +
		"installer stderr\n"

	var output bytes.Buffer
	if err := update(t.Context(), &output, linkedExecutable, server.URL, server.Client()); err != nil {
		t.Fatalf("update() error = %v", err)
	}
	if got := output.String(); got != wantOutput {
		t.Errorf("update() output = %q, want %q", got, wantOutput)
	}
}

func TestUpdateRejectsMissingExecutable(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "installed")
	t.Setenv("DEVDASH_UPDATE_MARKER", marker)

	_, installerURL := installerServer(t, http.StatusOK, `touch "$DEVDASH_UPDATE_MARKER"`)
	missingTarget := filepath.Join(t.TempDir(), "missing")
	linkedExecutable := filepath.Join(t.TempDir(), "devdash")
	if err := os.Symlink(missingTarget, linkedExecutable); err != nil {
		t.Fatalf("create broken executable symlink: %v", err)
	}

	err := update(t.Context(), &bytes.Buffer{}, linkedExecutable, installerURL, http.DefaultClient)
	if err == nil || !strings.HasPrefix(err.Error(), "resolve devdash executable ") {
		t.Fatalf("update() error = %v, want executable resolution error", err)
	}
	assertFileMissing(t, marker)
}

func TestUpdateRejectsHTTPFailure(t *testing.T) {
	executable := createFixtureExecutable(t)
	marker := filepath.Join(t.TempDir(), "installed")
	t.Setenv("DEVDASH_UPDATE_MARKER", marker)

	client, installerURL := installerServer(t, http.StatusServiceUnavailable, `touch "$DEVDASH_UPDATE_MARKER"`)
	var output bytes.Buffer
	err := update(t.Context(), &output, executable, installerURL, client)
	if err == nil || err.Error() != "download installer: unexpected HTTP status 503 Service Unavailable" {
		t.Fatalf("update() error = %v, want HTTP status error", err)
	}
	if got := output.String(); got != "Updating Devdash...\n" {
		t.Errorf("update() output = %q, want progress line", got)
	}
	assertFileMissing(t, marker)
}

func TestUpdateRejectsOversizedInstaller(t *testing.T) {
	executable := createFixtureExecutable(t)
	marker := filepath.Join(t.TempDir(), "installed")
	t.Setenv("DEVDASH_UPDATE_MARKER", marker)
	installer := "touch \"$DEVDASH_UPDATE_MARKER\"\n" + strings.Repeat("#", int(maxInstallerSize))

	client, installerURL := installerServer(t, http.StatusOK, installer)
	err := update(t.Context(), &bytes.Buffer{}, executable, installerURL, client)
	if err == nil || err.Error() != "installer exceeds 1048576 bytes" {
		t.Fatalf("update() error = %v, want installer size error", err)
	}
	assertFileMissing(t, marker)
}

func TestUpdateReturnsInstallerFailure(t *testing.T) {
	executable := createFixtureExecutable(t)
	client, installerURL := installerServer(t, http.StatusOK, "printf 'installer failed\\n' >&2\nexit 12\n")

	var output bytes.Buffer
	err := update(t.Context(), &output, executable, installerURL, client)
	if err == nil || err.Error() != "run installer: exit status 12" {
		t.Fatalf("update() error = %v, want installer exit error", err)
	}
	if got := output.String(); got != "Updating Devdash...\ninstaller failed\n" {
		t.Errorf("update() output = %q, want progress and installer stderr", got)
	}
}

func createFixtureExecutable(t *testing.T) string {
	t.Helper()
	executable := filepath.Join(t.TempDir(), "devdash")
	if err := os.WriteFile(executable, []byte("fixture"), 0o755); err != nil {
		t.Fatalf("create fixture executable: %v", err)
	}
	return executable
}

func installerServer(t *testing.T, status int, installer string) (*http.Client, string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(status)
		fmt.Fprint(response, installer)
	}))
	t.Cleanup(server.Close)
	return server.Client(), server.URL
}

func assertFileMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file %q exists or stat error = %v, want missing", path, err)
	}
}

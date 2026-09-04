package platform

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
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

	installer := `#!/bin/sh
printf 'install=%s\n' "$DEVDASH_INSTALL_DIR"
printf 'version=%s\n' "$DEVDASH_VERSION"
printf 'unrelated=%s\n' "$DEVDASH_UPDATE_TEST"
printf 'installer stderr\n' >&2
`
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			t.Errorf("request method = %q, want GET", request.Method)
		}
		io.WriteString(response, installer)
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
		"  Executable: " + resolvedExecutable + "\n" +
		"  Install directory: " + filepath.Dir(resolvedExecutable) + "\n" +
		"  Release: latest\n" +
		"  Installer: " + server.URL + "\n" +
		"Downloading installer...\n" +
		"Downloading installer: done\n" +
		fmt.Sprintf("Installer downloaded: %d bytes\n", len(installer)) +
		"Installing latest release...\n" +
		"install=" + filepath.Dir(resolvedExecutable) + "\n" +
		"version=latest\n" +
		"unrelated=preserved\n" +
		"installer stderr\n" +
		"Update complete.\n"

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
	resolvedExecutable := resolveFixtureExecutable(t, executable)
	marker := filepath.Join(t.TempDir(), "installed")
	t.Setenv("DEVDASH_UPDATE_MARKER", marker)

	client, installerURL := installerServer(t, http.StatusServiceUnavailable, `touch "$DEVDASH_UPDATE_MARKER"`)
	var output bytes.Buffer
	err := update(t.Context(), &output, executable, installerURL, client)
	if err == nil || err.Error() != "download installer: unexpected HTTP status 503 Service Unavailable" {
		t.Fatalf("update() error = %v, want HTTP status error", err)
	}
	wantOutput := "Updating Devdash...\n" +
		"  Executable: " + resolvedExecutable + "\n" +
		"  Install directory: " + filepath.Dir(resolvedExecutable) + "\n" +
		"  Release: latest\n" +
		"  Installer: " + installerURL + "\n" +
		"Downloading installer...\n" +
		"Downloading installer: failed\n"
	if got := output.String(); got != wantOutput {
		t.Errorf("update() output = %q, want %q", got, wantOutput)
	}
	assertFileMissing(t, marker)
}

func TestUpdateRejectsOversizedInstaller(t *testing.T) {
	executable := createFixtureExecutable(t)
	resolvedExecutable := resolveFixtureExecutable(t, executable)
	marker := filepath.Join(t.TempDir(), "installed")
	t.Setenv("DEVDASH_UPDATE_MARKER", marker)
	installer := "touch \"$DEVDASH_UPDATE_MARKER\"\n" + strings.Repeat("#", int(maxInstallerSize))

	client, installerURL := installerServer(t, http.StatusOK, installer)
	var output bytes.Buffer
	err := update(t.Context(), &output, executable, installerURL, client)
	if err == nil || err.Error() != "installer exceeds 1048576 bytes" {
		t.Fatalf("update() error = %v, want installer size error", err)
	}
	wantOutput := "Updating Devdash...\n" +
		"  Executable: " + resolvedExecutable + "\n" +
		"  Install directory: " + filepath.Dir(resolvedExecutable) + "\n" +
		"  Release: latest\n" +
		"  Installer: " + installerURL + "\n" +
		"Downloading installer...\n" +
		"Downloading installer: failed\n"
	if got := output.String(); got != wantOutput {
		t.Errorf("update() output = %q, want %q", got, wantOutput)
	}
	assertFileMissing(t, marker)
}

func TestUpdateReturnsInstallerFailure(t *testing.T) {
	executable := createFixtureExecutable(t)
	resolvedExecutable := resolveFixtureExecutable(t, executable)
	installer := "printf 'installer failed\\n' >&2\nexit 12\n"
	client, installerURL := installerServer(t, http.StatusOK, installer)

	var output bytes.Buffer
	err := update(t.Context(), &output, executable, installerURL, client)
	if err == nil || err.Error() != "run installer: exit status 12" {
		t.Fatalf("update() error = %v, want installer exit error", err)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 12 {
		t.Fatalf("update() error = %v, want wrapped exit status 12", err)
	}
	wantOutput := "Updating Devdash...\n" +
		"  Executable: " + resolvedExecutable + "\n" +
		"  Install directory: " + filepath.Dir(resolvedExecutable) + "\n" +
		"  Release: latest\n" +
		"  Installer: " + installerURL + "\n" +
		"Downloading installer...\n" +
		"Downloading installer: done\n" +
		fmt.Sprintf("Installer downloaded: %d bytes\n", len(installer)) +
		"Installing latest release...\n" +
		"installer failed\n"
	if got := output.String(); got != wantOutput {
		t.Errorf("update() output = %q, want %q", got, wantOutput)
	}
}

func TestUpdatePreservesDownloadAndContextErrors(t *testing.T) {
	executable := createFixtureExecutable(t)
	downloadErr := errors.New("transport failed")
	cancelledContext, cancel := context.WithCancel(context.Background())
	cancel()

	for _, test := range []struct {
		name      string
		ctx       context.Context
		cause     error
		transport roundTripFunc
	}{
		{
			name:  "HTTP transport",
			ctx:   context.Background(),
			cause: downloadErr,
			transport: func(*http.Request) (*http.Response, error) {
				return nil, downloadErr
			},
		},
		{
			name:  "cancelled context",
			ctx:   cancelledContext,
			cause: context.Canceled,
			transport: func(request *http.Request) (*http.Response, error) {
				return nil, request.Context().Err()
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				if !strings.HasSuffix(output.String(), "Downloading installer...\n") {
					t.Errorf("update() output at RoundTrip = %q, want download progress started", output.String())
				}
				return test.transport(request)
			})}
			err := update(test.ctx, &output, executable, "https://example.test/install.sh", client)
			if !errors.Is(err, test.cause) {
				t.Fatalf("update() error = %v, want wrapped %v", err, test.cause)
			}
			if !strings.HasSuffix(output.String(), "Downloading installer...\nDownloading installer: failed\n") {
				t.Errorf("update() output = %q, want failed download progress", output.String())
			}
		})
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

func resolveFixtureExecutable(t *testing.T, executable string) string {
	t.Helper()
	resolvedExecutable, err := filepath.EvalSymlinks(executable)
	if err != nil {
		t.Fatalf("resolve fixture executable: %v", err)
	}
	return resolvedExecutable
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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func assertFileMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file %q exists or stat error = %v, want missing", path, err)
	}
}

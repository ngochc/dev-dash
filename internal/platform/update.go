package platform

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const defaultInstallerURL = "https://raw.githubusercontent.com/ngochc/dev-dash/main/install.sh"
const maxInstallerSize int64 = 1 << 20

// Update reinstalls the latest Devdash release over the running executable.
func Update(ctx context.Context, output io.Writer) error {
	executablePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate devdash executable: %w", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	return update(ctx, output, executablePath, defaultInstallerURL, client)
}

func update(
	ctx context.Context,
	output io.Writer,
	executablePath string,
	installerURL string,
	client *http.Client,
) error {
	resolvedExecutablePath, err := filepath.EvalSymlinks(executablePath)
	if err != nil {
		return fmt.Errorf("resolve devdash executable %q: %w", executablePath, err)
	}
	installDir := filepath.Dir(resolvedExecutablePath)

	fmt.Fprintln(output, "Updating Devdash...")

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, installerURL, nil)
	if err != nil {
		return fmt.Errorf("create installer request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("download installer: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download installer: unexpected HTTP status %s", response.Status)
	}

	installer, err := io.ReadAll(io.LimitReader(response.Body, maxInstallerSize+1))
	if err != nil {
		return fmt.Errorf("read installer: %w", err)
	}
	if int64(len(installer)) > maxInstallerSize {
		return fmt.Errorf("installer exceeds %d bytes", maxInstallerSize)
	}

	command := exec.CommandContext(ctx, "/bin/sh")
	command.Stdin = bytes.NewReader(installer)
	command.Stdout = output
	command.Stderr = output
	command.Env = updateEnvironment(os.Environ(), installDir)
	if err := command.Run(); err != nil {
		return fmt.Errorf("run installer: %w", err)
	}

	return nil
}

func updateEnvironment(environment []string, installDir string) []string {
	updated := make([]string, 0, len(environment)+2)
	for _, variable := range environment {
		if strings.HasPrefix(variable, "DEVDASH_INSTALL_DIR=") ||
			strings.HasPrefix(variable, "DEVDASH_VERSION=") {
			continue
		}
		updated = append(updated, variable)
	}

	return append(
		updated,
		"DEVDASH_INSTALL_DIR="+installDir,
		"DEVDASH_VERSION=latest",
	)
}

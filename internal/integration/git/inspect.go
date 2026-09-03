package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ngochc/dev-dash/internal/repository"
)

var ErrUnavailable = errors.New("git unavailable")

type CommandRunner interface {
	Run(context.Context, ...string) ([]byte, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "git", args...)
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Run(); err != nil {
		return output.Bytes(), err
	}
	return output.Bytes(), nil
}

type Inspector struct {
	runner CommandRunner
}

func NewInspector(runner CommandRunner) *Inspector {
	return &Inspector{runner: runner}
}

func NewCLIInspector() *Inspector {
	return NewInspector(ExecRunner{})
}

func (i *Inspector) Inspect(ctx context.Context, path, expectedRemote string) (repository.CheckoutInspection, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return repository.CheckoutInspection{}, nil
		}
		return repository.CheckoutInspection{}, fmt.Errorf("stat checkout %q: %w", path, err)
	}

	topLevel, err := i.runner.Run(ctx, "-C", path, "rev-parse", "--show-toplevel")
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return repository.CheckoutInspection{}, ctxErr
		}
		if errors.Is(err, exec.ErrNotFound) {
			return repository.CheckoutInspection{}, fmt.Errorf("git is not installed: %w", ErrUnavailable)
		}
		return repository.CheckoutInspection{Exists: true}, nil
	}
	actualTopLevel, err := canonicalCheckoutPath(strings.TrimSpace(string(topLevel)))
	if err != nil {
		return repository.CheckoutInspection{}, fmt.Errorf("resolve checkout root: %w", err)
	}
	expectedTopLevel, err := canonicalCheckoutPath(path)
	if err != nil {
		return repository.CheckoutInspection{}, fmt.Errorf("resolve expected checkout root: %w", err)
	}
	if actualTopLevel != expectedTopLevel {
		return repository.CheckoutInspection{Exists: true}, nil
	}

	origin, err := i.runner.Run(ctx, "-C", path, "remote", "get-url", "origin")
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return repository.CheckoutInspection{}, ctxErr
		}
		if errors.Is(err, exec.ErrNotFound) {
			return repository.CheckoutInspection{}, fmt.Errorf("git is not installed: %w", ErrUnavailable)
		}
		return repository.CheckoutInspection{Exists: true}, nil
	}
	actualIdentity, err := NormalizeRemote(strings.TrimSpace(string(origin)))
	if err != nil {
		return repository.CheckoutInspection{Exists: true}, nil
	}
	expectedIdentity, err := NormalizeRemote(expectedRemote)
	if err != nil {
		return repository.CheckoutInspection{}, fmt.Errorf("normalize expected repository remote: %w", err)
	}
	return repository.CheckoutInspection{
		Exists: true,
		Valid:  actualIdentity == expectedIdentity,
	}, nil
}

func canonicalCheckoutPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

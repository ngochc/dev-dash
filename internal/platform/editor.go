package platform

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// EditTemporaryFile opens initial content in the configured visual editor.
func EditTemporaryFile(ctx context.Context, pattern string, initial []byte, input io.Reader, output io.Writer) ([]byte, error) {
	editor := strings.TrimSpace(os.Getenv("VISUAL"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if editor == "" {
		return nil, errors.New("no editor configured; set $VISUAL or $EDITOR")
	}

	parts := strings.Fields(editor)
	file, err := os.CreateTemp("", pattern)
	if err != nil {
		return nil, fmt.Errorf("create editor file: %w", err)
	}
	path := file.Name()
	defer os.Remove(path)

	if _, err := file.Write(initial); err != nil {
		file.Close()
		return nil, fmt.Errorf("write editor file: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close editor file: %w", err)
	}

	command := exec.CommandContext(ctx, parts[0], append(parts[1:], path)...)
	command.Stdin = input
	command.Stdout = output
	command.Stderr = output
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("run editor: %w", err)
	}

	edited, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read editor file: %w", err)
	}
	return edited, nil
}

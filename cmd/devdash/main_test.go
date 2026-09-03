package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunExitCode(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		if code := run(context.Background(), []string{"help"}, &bytes.Buffer{}); code != 0 {
			t.Fatalf("run() code = %d, want 0", code)
		}
	})

	t.Run("error", func(t *testing.T) {
		var errorOutput bytes.Buffer
		if code := run(context.Background(), []string{"unknown"}, &errorOutput); code != 1 {
			t.Fatalf("run() code = %d, want 1", code)
		}
		if !strings.Contains(errorOutput.String(), "devdash: unknown command: unknown") {
			t.Errorf("run() error output = %q", errorOutput.String())
		}
	})
}

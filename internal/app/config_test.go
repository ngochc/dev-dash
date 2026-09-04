package app

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestRunConfigKeysGitHub(t *testing.T) {
	var output bytes.Buffer
	if err := run(context.Background(), []string{"config", "keys", "github"}, strings.NewReader(""), &output, &bytes.Buffer{}); err != nil {
		t.Fatalf("run(config keys github) error = %v", err)
	}
	fields := strings.Fields(output.String())
	want := []string{
		"KEY", "REQUIRED", "DEFAULT", "DESCRIPTION",
		"github.base_url", "no", "https://github.com", "GitHub", "or", "GHES", "base", "URL",
		"github.org", "yes", "-", "GitHub", "organization",
	}
	if !reflect.DeepEqual(fields, want) {
		t.Errorf("config keys fields = %v, want %v", fields, want)
	}
}

func TestRunConfigKeysAll(t *testing.T) {
	var output bytes.Buffer
	if err := runConfig([]string{"keys"}, &output); err != nil {
		t.Fatalf("runConfig(keys) error = %v", err)
	}
	if !strings.Contains(output.String(), "github.base_url") || !strings.Contains(output.String(), "github.org") {
		t.Errorf("config keys output = %q, want GitHub definitions", output.String())
	}
}

func TestRunConfigErrors(t *testing.T) {
	for _, test := range []struct {
		args    []string
		wantErr string
	}{
		{wantErr: "config command required: keys"},
		{args: []string{"unknown"}, wantErr: "unknown config command: unknown"},
		{args: []string{"keys", "github", "extra"}, wantErr: "usage: devdash config keys [provider]"},
		{args: []string{"keys", "unknown"}, wantErr: "unknown config provider: unknown"},
	} {
		var output bytes.Buffer
		err := runConfig(test.args, &output)
		if err == nil || err.Error() != test.wantErr {
			t.Errorf("runConfig(%v) error = %v, want %q", test.args, err, test.wantErr)
		}
		if output.Len() != 0 {
			t.Errorf("runConfig(%v) output = %q, want empty", test.args, output.String())
		}
	}
}

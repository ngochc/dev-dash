package github

import (
	"errors"
	"reflect"
	"testing"

	"github.com/ngochc/dev-dash/internal/configdef"
)

func TestDefinitions(t *testing.T) {
	want := []configdef.Definition{
		{Name: "github.base_url", Default: "https://github.com", Description: "GitHub or GHES base URL"},
		{Name: "github.org", Required: true, Description: "GitHub organization"},
	}
	if !reflect.DeepEqual(Definitions, want) {
		t.Errorf("Definitions = %#v, want %#v", Definitions, want)
	}
}

func TestResolveConfigAppliesDefault(t *testing.T) {
	config, err := ResolveConfig("mqms", map[string]string{"org": "example-org"})
	if err != nil {
		t.Fatalf("ResolveConfig() error = %v", err)
	}
	want := Config{BaseURL: "https://github.com", Host: "github.com", Organization: "example-org"}
	if config != want {
		t.Errorf("ResolveConfig() = %#v, want %#v", config, want)
	}
}

func TestResolveConfigUsesStoredOverride(t *testing.T) {
	config, err := ResolveConfig("mqms", map[string]string{
		"base_url": "https://Git.Example.com/",
		"org":      "team",
	})
	if err != nil {
		t.Fatalf("ResolveConfig() error = %v", err)
	}
	want := Config{BaseURL: "https://Git.Example.com", Host: "git.example.com", Organization: "team"}
	if config != want {
		t.Errorf("ResolveConfig() = %#v, want %#v", config, want)
	}
}

func TestResolveConfigMissingOrganization(t *testing.T) {
	_, err := ResolveConfig("mqms", nil)
	if !errors.Is(err, ErrIncompleteConfig) {
		t.Fatalf("ResolveConfig() error = %v, want ErrIncompleteConfig", err)
	}
	want := "GitHub configuration is incomplete for workspace \"mqms\".\n\nMissing:\n  github.org\n\nConfigure it with:\n  devdash workspace config set mqms github.org <organization>\n\nShow supported GitHub keys:\n  devdash config keys github"
	if err.Error() != want {
		t.Errorf("ResolveConfig() error = %q, want %q", err, want)
	}
}

func TestResolveConfigRejectsInvalidBaseURL(t *testing.T) {
	for _, baseURL := range []string{"foo", "ftp://github.example", "https:///missing-host"} {
		t.Run(baseURL, func(t *testing.T) {
			_, err := ResolveConfig("mqms", map[string]string{"base_url": baseURL, "org": "team"})
			if !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("ResolveConfig() error = %v, want ErrInvalidConfig", err)
			}
			want := "Invalid GitHub configuration:\n  github.base_url = \"" + baseURL + "\"\n\nExpected an HTTP or HTTPS URL."
			if err.Error() != want {
				t.Errorf("ResolveConfig() error = %q, want %q", err, want)
			}
		})
	}
}

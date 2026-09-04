package confluence

import (
	"errors"
	"reflect"
	"testing"

	"github.com/ngochc/dev-dash/internal/configdef"
)

func TestDefinitions(t *testing.T) {
	want := []configdef.Definition{
		{Name: BaseURLKey, Required: true, Description: "Confluence base URL"},
		{Name: SpaceKey, Required: true, Description: "Confluence space"},
		{Name: SecretKey, Required: true, Description: "Secret reference containing PAT"},
		{Name: AuthTypeKey, Default: DefaultAuthType, Description: "Authentication mode"},
		{Name: RootPageKey, Description: "Optional root page ID"},
	}
	if !reflect.DeepEqual(Definitions, want) {
		t.Fatalf("Definitions = %#v, want %#v", Definitions, want)
	}
}

func TestResolveConfigAppliesPATDefaultInMemory(t *testing.T) {
	config, err := ResolveConfig("mqms", validValues())
	if err != nil {
		t.Fatalf("ResolveConfig() error = %v", err)
	}
	want := Config{
		BaseURL:         "https://wiki.example.com/confluence",
		Space:           "MQMS",
		SecretReference: "secret:confluence.pat",
		SecretName:      "confluence.pat",
		AuthType:        DefaultAuthType,
	}
	if config != want {
		t.Fatalf("ResolveConfig() = %#v, want %#v", config, want)
	}
}

func TestResolveConfigReportsAllMissingRequiredKeys(t *testing.T) {
	_, err := ResolveConfig("mqms", map[string]string{"base_url": " ", "space": "", "secret": "\t"})
	if !errors.Is(err, ErrIncompleteConfig) {
		t.Fatalf("ResolveConfig() error = %v, want ErrIncompleteConfig", err)
	}
	want := "Confluence configuration is incomplete for workspace \"mqms\".\n\nMissing:\n  confluence.base_url\n  confluence.space\n  confluence.secret\n\nConfigure with:\n  devdash workspace setup mqms\n\nShow supported keys:\n  devdash config keys confluence"
	if err.Error() != want {
		t.Fatalf("ResolveConfig() error = %q, want %q", err, want)
	}
}

func TestResolveBaseURLPreservesContextPath(t *testing.T) {
	got, err := ResolveBaseURL("  https://Wiki.Example.com/confluence///  ")
	if err != nil {
		t.Fatalf("ResolveBaseURL() error = %v", err)
	}
	if want := "https://Wiki.Example.com/confluence"; got != want {
		t.Fatalf("ResolveBaseURL() = %q, want %q", got, want)
	}
}

func TestResolveConfigRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   string
		wantKey string
		want    string
	}{
		{name: "relative URL", key: "base_url", value: "wiki.example.com", wantKey: BaseURLKey, want: "Expected an HTTP or HTTPS URL."},
		{name: "URL userinfo", key: "base_url", value: "https://pat@wiki.example.com", wantKey: BaseURLKey, want: "Expected an HTTP or HTTPS URL."},
		{name: "URL query", key: "base_url", value: "https://wiki.example.com?x=1", wantKey: BaseURLKey, want: "Expected an HTTP or HTTPS URL."},
		{name: "URL fragment", key: "base_url", value: "https://wiki.example.com#x", wantKey: BaseURLKey, want: "Expected an HTTP or HTTPS URL."},
		{name: "secret scheme", key: "secret", value: "env:PAT", wantKey: SecretKey, want: "Expected secret:<key>."},
		{name: "secret key", key: "secret", value: "secret:bad key", wantKey: SecretKey, want: "Expected secret:<key>."},
		{name: "auth type", key: "auth_type", value: "basic", wantKey: AuthTypeKey, want: "Supported:\n  pat"},
		{name: "root page", key: "root_page", value: "12x", wantKey: RootPageKey, want: "Expected a decimal page ID."},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			values := validValues()
			values[test.key] = test.value
			_, err := ResolveConfig("mqms", values)
			if !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("ResolveConfig() error = %v, want ErrInvalidConfig", err)
			}
			want := "Invalid Confluence configuration:\n  " + test.wantKey + " = \"" + test.value + "\"\n\n" + test.want
			if err.Error() != want {
				t.Fatalf("ResolveConfig() error = %q, want %q", err, want)
			}
		})
	}
}

func TestResolveConfigAcceptsRootPage(t *testing.T) {
	values := validValues()
	values["root_page"] = "123456"
	config, err := ResolveConfig("mqms", values)
	if err != nil {
		t.Fatalf("ResolveConfig() error = %v", err)
	}
	if config.RootPage != "123456" {
		t.Fatalf("RootPage = %q, want 123456", config.RootPage)
	}
}

func validValues() map[string]string {
	return map[string]string{
		"base_url": "https://wiki.example.com/confluence/",
		"space":    " MQMS ",
		"secret":   "secret:confluence.pat",
	}
}

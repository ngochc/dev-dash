package github

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/ngochc/dev-dash/internal/configdef"
)

const (
	BaseURLKey      = "github.base_url"
	OrganizationKey = "github.org"
	DefaultBaseURL  = "https://github.com"
)

var (
	ErrIncompleteConfig = errors.New("incomplete GitHub configuration")
	ErrInvalidConfig    = errors.New("invalid GitHub configuration")

	Definitions = []configdef.Definition{
		{
			Name:        BaseURLKey,
			Default:     DefaultBaseURL,
			Description: "GitHub or GHES base URL",
		},
		{
			Name:        OrganizationKey,
			Required:    true,
			Description: "GitHub organization",
		},
	}
)

// Config is the effective GitHub configuration for one workspace.
type Config struct {
	BaseURL      string
	Host         string
	Organization string
}

// ResolveHostConfig applies the default GitHub host and validates its URL.
func ResolveHostConfig(values map[string]string) (Config, error) {
	baseURL := strings.TrimSpace(values["base_url"])
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.Hostname() == "" {
		return Config{}, configError{
			message: fmt.Sprintf("Invalid GitHub configuration:\n  %s = %q\n\nExpected an HTTP or HTTPS URL.", BaseURLKey, baseURL),
			cause:   ErrInvalidConfig,
		}
	}
	return Config{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Host:    strings.ToLower(parsed.Hostname()),
	}, nil
}

// ResolveConfig applies defaults and validates GitHub configuration before external work.
func ResolveConfig(workspaceName string, values map[string]string) (Config, error) {
	config, err := ResolveHostConfig(values)
	if err != nil {
		return Config{}, err
	}
	organization := strings.TrimSpace(values["org"])
	if organization == "" {
		return Config{}, configError{
			message: fmt.Sprintf("GitHub configuration is incomplete for workspace %q.\n\nMissing:\n  %s\n\nConfigure it with:\n  devdash workspace config set %s %s <organization>\n\nShow supported GitHub keys:\n  devdash config keys github", workspaceName, OrganizationKey, workspaceName, OrganizationKey),
			cause:   ErrIncompleteConfig,
		}
	}
	config.Organization = organization
	return config, nil
}

type configError struct {
	message string
	cause   error
}

func (e configError) Error() string { return e.message }
func (e configError) Unwrap() error { return e.cause }

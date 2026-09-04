package confluence

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/ngochc/dev-dash/internal/configdef"
	"github.com/ngochc/dev-dash/internal/secret"
)

const (
	BaseURLKey      = "confluence.base_url"
	SpaceKey        = "confluence.space"
	SecretKey       = "confluence.secret"
	AuthTypeKey     = "confluence.auth_type"
	RootPageKey     = "confluence.root_page"
	DefaultAuthType = "pat"
)

var (
	ErrIncompleteConfig = errors.New("incomplete Confluence configuration")
	ErrInvalidConfig    = errors.New("invalid Confluence configuration")

	Definitions = []configdef.Definition{
		{Name: BaseURLKey, Required: true, Description: "Confluence base URL"},
		{Name: SpaceKey, Required: true, Description: "Confluence space"},
		{Name: SecretKey, Required: true, Description: "Secret reference containing PAT"},
		{Name: AuthTypeKey, Default: DefaultAuthType, Description: "Authentication mode"},
		{Name: RootPageKey, Description: "Optional root page ID"},
	}

	pageIDPattern = regexp.MustCompile(`^[0-9]+$`)
)

// Config is the effective Confluence configuration for one workspace.
type Config struct {
	BaseURL         string
	Space           string
	SecretReference string
	SecretName      string
	AuthType        string
	RootPage        string
}

// ResolveBaseURL normalizes and validates a Confluence Data Center base URL.
func ResolveBaseURL(value string) (string, error) {
	baseURL := strings.TrimSpace(value)
	parsed, err := url.Parse(baseURL)
	if err != nil || !parsed.IsAbs() || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" {
		return "", invalidValueError(BaseURLKey, value, "Expected an HTTP or HTTPS URL.")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = strings.TrimRight(parsed.RawPath, "/")
	return parsed.String(), nil
}

// ResolveConfig applies in-memory defaults and validates Confluence configuration.
func ResolveConfig(workspaceName string, values map[string]string) (Config, error) {
	missing := make([]string, 0, 3)
	for _, key := range []struct {
		full  string
		local string
	}{
		{BaseURLKey, "base_url"},
		{SpaceKey, "space"},
		{SecretKey, "secret"},
	} {
		if strings.TrimSpace(values[key.local]) == "" {
			missing = append(missing, key.full)
		}
	}
	if len(missing) != 0 {
		return Config{}, configError{
			message: fmt.Sprintf("Confluence configuration is incomplete for workspace %q.\n\nMissing:\n  %s\n\nConfigure with:\n  devdash workspace setup %s\n\nShow supported keys:\n  devdash config keys confluence", workspaceName, strings.Join(missing, "\n  "), workspaceName),
			cause:   ErrIncompleteConfig,
		}
	}

	baseURL, err := ResolveBaseURL(values["base_url"])
	if err != nil {
		return Config{}, err
	}
	space := strings.TrimSpace(values["space"])
	secretReference := strings.TrimSpace(values["secret"])
	secretName, ok := strings.CutPrefix(secretReference, "secret:")
	if !ok || secret.ValidateKey(secretName) != nil {
		return Config{}, invalidValueError(SecretKey, values["secret"], "Expected secret:<key>.")
	}
	authType := strings.TrimSpace(values["auth_type"])
	if authType == "" {
		authType = DefaultAuthType
	}
	if authType != DefaultAuthType {
		return Config{}, invalidValueError(AuthTypeKey, values["auth_type"], "Supported:\n  pat")
	}
	rootPage := strings.TrimSpace(values["root_page"])
	if rootPage != "" && !pageIDPattern.MatchString(rootPage) {
		return Config{}, invalidValueError(RootPageKey, values["root_page"], "Expected a decimal page ID.")
	}

	return Config{
		BaseURL:         baseURL,
		Space:           space,
		SecretReference: secretReference,
		SecretName:      secretName,
		AuthType:        authType,
		RootPage:        rootPage,
	}, nil
}

func invalidValueError(key, value, guidance string) error {
	return configError{
		message: fmt.Sprintf("Invalid Confluence configuration:\n  %s = %q\n\n%s", key, value, guidance),
		cause:   ErrInvalidConfig,
	}
}

type configError struct {
	message string
	cause   error
}

func (e configError) Error() string { return e.message }
func (e configError) Unwrap() error { return e.cause }

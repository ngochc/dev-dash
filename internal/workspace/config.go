package workspace

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	ErrInvalidConfigKey = errors.New("invalid workspace config key")
	namespacePattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)
	configKeyPattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

// ConfigEntry is one provider-neutral workspace configuration value.
type ConfigEntry struct {
	Namespace string
	Key       string
	Value     string
}

func (e ConfigEntry) FullKey() string {
	return e.Namespace + "." + e.Key
}

// ParseConfigKey validates and separates a dotted key at its first dot.
func ParseConfigKey(fullKey string) (namespace, key string, err error) {
	namespace, key, found := strings.Cut(fullKey, ".")
	if !found || !namespacePattern.MatchString(namespace) || !configKeyPattern.MatchString(key) {
		return "", "", fmt.Errorf("%w %q", ErrInvalidConfigKey, fullKey)
	}
	return namespace, key, nil
}

package configdef

import (
	"sort"
	"strings"
)

// Definition describes one configuration key understood by an integration.
type Definition struct {
	Name        string
	Required    bool
	Default     string
	Description string
}

// Registry provides discoverable integration configuration definitions.
type Registry struct {
	definitions []Definition
}

func NewRegistry(definitions ...Definition) Registry {
	copied := append([]Definition(nil), definitions...)
	sort.Slice(copied, func(i, j int) bool { return copied[i].Name < copied[j].Name })
	return Registry{definitions: copied}
}

// List returns sorted definitions for one provider, or every definition when provider is empty.
func (r Registry) List(provider string) []Definition {
	if provider == "" {
		return append([]Definition(nil), r.definitions...)
	}
	prefix := provider + "."
	definitions := make([]Definition, 0)
	for _, definition := range r.definitions {
		if strings.HasPrefix(definition.Name, prefix) {
			definitions = append(definitions, definition)
		}
	}
	return definitions
}

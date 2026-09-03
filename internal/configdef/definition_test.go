package configdef

import (
	"reflect"
	"testing"
)

func TestRegistryListsSortedDefinitionsByProvider(t *testing.T) {
	registry := NewRegistry(
		Definition{Name: "jira.project"},
		Definition{Name: "github.org"},
		Definition{Name: "github.base_url"},
	)

	github := registry.List("github")
	wantGitHub := []Definition{{Name: "github.base_url"}, {Name: "github.org"}}
	if !reflect.DeepEqual(github, wantGitHub) {
		t.Errorf("List(github) = %#v, want %#v", github, wantGitHub)
	}
	all := registry.List("")
	wantAll := []Definition{{Name: "github.base_url"}, {Name: "github.org"}, {Name: "jira.project"}}
	if !reflect.DeepEqual(all, wantAll) {
		t.Errorf("List(all) = %#v, want %#v", all, wantAll)
	}
	all[0].Name = "mutated"
	if registry.List("")[0].Name != "github.base_url" {
		t.Fatal("List() exposed registry storage")
	}
}

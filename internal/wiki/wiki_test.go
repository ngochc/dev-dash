package wiki

import (
	"errors"
	"reflect"
	"testing"
)

func TestSelectResolvesIDsTitlesOrderingAndDeduplication(t *testing.T) {
	pages := []Page{
		{ResourceID: "r2", PageID: "2", Title: "Shared"},
		{ResourceID: "r1", PageID: "1", Title: "One"},
		{ResourceID: "r3", PageID: "3", Title: "Shared"},
		{ResourceID: "r4", PageID: "Shared", Title: "ID wins"},
	}
	selected, err := Select(pages, []string{"One", "2", "One", "Shared"}, false)
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	want := []Page{pages[1], pages[0], pages[3]}
	if !reflect.DeepEqual(selected, want) {
		t.Fatalf("Select() = %#v, want %#v", selected, want)
	}
}

func TestSelectRejectsUnknownAndAmbiguousTitles(t *testing.T) {
	pages := []Page{{ResourceID: "r1", PageID: "1", Title: "Same"}, {ResourceID: "r2", PageID: "2", Title: "Same"}}
	for _, test := range []struct {
		selector string
		want     string
	}{
		{"missing", `wiki page "missing" not found`},
		{"Same", `wiki page "Same" is ambiguous; use page ID`},
	} {
		_, err := Select(pages, []string{test.selector}, false)
		if err == nil || err.Error() != test.want {
			t.Errorf("Select(%q) error = %v, want %q", test.selector, err, test.want)
		}
	}
}

func TestSelectAllUsesDeterministicOrder(t *testing.T) {
	pages := []Page{
		{ResourceID: "r3", PageID: "3", Title: "Beta"},
		{ResourceID: "r2", PageID: "2", Title: "Alpha"},
		{ResourceID: "r1", PageID: "1", Title: "Alpha"},
	}
	selected, err := Select(pages, nil, true)
	if err != nil {
		t.Fatalf("Select(all) error = %v", err)
	}
	if got := []string{selected[0].PageID, selected[1].PageID, selected[2].PageID}; !reflect.DeepEqual(got, []string{"1", "2", "3"}) {
		t.Fatalf("Select(all) IDs = %v", got)
	}
}

func TestListDerivesStatesAndSorts(t *testing.T) {
	inspector := fakeInspector{inspections: map[string]FileInspection{
		"/wiki/fetched.md": {Exists: true, Regular: true},
		"/wiki/missing.md": {},
	}}
	pages := []Page{
		{ResourceID: "r3", PageID: "3", Title: "Zulu", MaterializedPath: "/wiki/missing.md"},
		{ResourceID: "r2", PageID: "2", Title: "Alpha", MaterializedPath: "/wiki/fetched.md"},
		{ResourceID: "r1", PageID: "1", Title: "Alpha"},
	}
	listed, err := List(pages, inspector)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	got := []struct {
		id    string
		state State
	}{{listed[0].Page.PageID, listed[0].State}, {listed[1].Page.PageID, listed[1].State}, {listed[2].Page.PageID, listed[2].State}}
	want := []struct {
		id    string
		state State
	}{{"1", StateNotFetched}, {"2", StateFetched}, {"3", StateMissing}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("List() = %#v, want %#v", got, want)
	}
}

func TestListRejectsInspectionErrorsAndNonRegularPaths(t *testing.T) {
	boom := errors.New("boom")
	_, err := List([]Page{{PageID: "1", MaterializedPath: "/wiki/one.md"}}, fakeInspector{err: boom})
	if !errors.Is(err, boom) {
		t.Fatalf("List() error = %v, want boom", err)
	}
	_, err = List([]Page{{PageID: "1", MaterializedPath: "/wiki/one.md"}}, fakeInspector{inspections: map[string]FileInspection{"/wiki/one.md": {Exists: true}}})
	if err == nil || err.Error() != `wiki page "1" path "/wiki/one.md" is not a regular file` {
		t.Fatalf("List() error = %v", err)
	}
}

type fakeInspector struct {
	inspections map[string]FileInspection
	err         error
}

func (f fakeInspector) Inspect(path string) (FileInspection, error) {
	if f.err != nil {
		return FileInspection{}, f.err
	}
	return f.inspections[path], nil
}

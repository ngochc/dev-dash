package wiki

import (
	"fmt"
	"sort"
)

// State is the local materialization state of a wiki page.
type State string

const (
	StateNotFetched State = "not-fetched"
	StateFetched    State = "fetched"
	StateMissing    State = "missing"
)

// Page is provider-neutral metadata for one wiki page.
type Page struct {
	ResourceID       string
	PageID           string
	ExternalKey      string
	Title            string
	URL              string
	MaterializedPath string
}

// Listed combines a page with its derived local state.
type Listed struct {
	Page  Page
	State State
}

// FetchResult describes one requested page fetch.
type FetchResult struct {
	PageID string
	Title  string
	Path   string
	Status string
	Error  error
}

// FileInspection reports whether a path exists as a regular file.
type FileInspection struct {
	Exists  bool
	Regular bool
}

// Inspector inspects a materialized page path.
type Inspector interface {
	Inspect(string) (FileInspection, error)
}

// Materializer owns safe page-file operations.
type Materializer interface {
	Inspector
	EnsureRoot(string) error
	WriteAtomic(string, []byte) error
	Remove(string) error
}

// List derives local state and returns pages sorted by title then page ID.
func List(pages []Page, inspector Inspector) ([]Listed, error) {
	ordered := sortedPages(pages)
	listed := make([]Listed, 0, len(ordered))
	for _, page := range ordered {
		state := StateNotFetched
		if page.MaterializedPath != "" {
			inspection, err := inspector.Inspect(page.MaterializedPath)
			if err != nil {
				return nil, fmt.Errorf("inspect wiki page %q: %w", page.PageID, err)
			}
			switch {
			case !inspection.Exists:
				state = StateMissing
			case !inspection.Regular:
				return nil, fmt.Errorf("wiki page %q path %q is not a regular file", page.PageID, page.MaterializedPath)
			default:
				state = StateFetched
			}
		}
		listed = append(listed, Listed{Page: page, State: state})
	}
	return listed, nil
}

// Select resolves exact page IDs before unique, case-sensitive titles.
func Select(pages []Page, selectors []string, all bool) ([]Page, error) {
	if all {
		return sortedPages(pages), nil
	}
	selected := make([]Page, 0, len(selectors))
	seen := make(map[string]struct{}, len(selectors))
	for _, selector := range selectors {
		var matches []Page
		for _, page := range pages {
			if page.PageID == selector {
				matches = []Page{page}
				break
			}
		}
		if len(matches) == 0 {
			for _, page := range pages {
				if page.Title == selector {
					matches = append(matches, page)
				}
			}
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("wiki page %q not found", selector)
		}
		if len(matches) > 1 {
			return nil, fmt.Errorf("wiki page %q is ambiguous; use page ID", selector)
		}
		page := matches[0]
		if _, duplicate := seen[page.ResourceID]; duplicate {
			continue
		}
		seen[page.ResourceID] = struct{}{}
		selected = append(selected, page)
	}
	return selected, nil
}

func sortedPages(pages []Page) []Page {
	ordered := append([]Page(nil), pages...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Title == ordered[j].Title {
			return ordered[i].PageID < ordered[j].PageID
		}
		return ordered[i].Title < ordered[j].Title
	})
	return ordered
}

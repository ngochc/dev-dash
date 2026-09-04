package confluence

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ngochc/dev-dash/internal/resource"
	"github.com/ngochc/dev-dash/internal/secret"
	"github.com/ngochc/dev-dash/internal/wiki"
	"github.com/ngochc/dev-dash/internal/workspace"
)

func TestServiceInvalidConfigAndMissingSecretMakeNoClientCalls(t *testing.T) {
	tests := []struct {
		name      string
		values    map[string]string
		secretErr error
		want      error
	}{
		{name: "missing config", values: nil, want: ErrIncompleteConfig},
		{name: "invalid URL", values: map[string]string{"base_url": "bad", "space": "DOC", "secret": "secret:pat"}, want: ErrInvalidConfig},
		{name: "missing secret", values: serviceConfigValues(), secretErr: secret.ErrNotFound, want: secret.ErrNotFound},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &fakePageClient{}
			store := &fakeSyncStore{}
			service := newConfluenceTestService(t.TempDir(), test.values, &fakeSecretReader{err: test.secretErr}, store, client, &fakeMaterializer{})
			_, _, err := service.Check(context.Background(), "workspace")
			if !errors.Is(err, test.want) {
				t.Fatalf("Check() error = %v, want %v", err, test.want)
			}
			if client.validateCalls != 0 || client.discoverCalls != 0 || client.fetchCalls != 0 || store.upsertCalls != 0 {
				t.Fatalf("invalid preparation invoked dependencies: client=%d/%d/%d store=%d", client.validateCalls, client.discoverCalls, client.fetchCalls, store.upsertCalls)
			}
			if strings.Contains(err.Error(), "secret-value") {
				t.Fatalf("Check() leaked secret: %q", err)
			}
		})
	}
}

func TestServiceMissingSecretGuidancePreservesSentinel(t *testing.T) {
	service := newConfluenceTestService(t.TempDir(), serviceConfigValues(), &fakeSecretReader{err: secret.ErrNotFound}, &fakeSyncStore{}, &fakePageClient{}, &fakeMaterializer{})
	_, _, err := service.Check(context.Background(), "workspace")
	want := "Confluence secret \"confluence.pat\" was not found for workspace \"workspace\".\n\nConfigure with:\n  devdash workspace setup workspace"
	if !errors.Is(err, secret.ErrNotFound) || err.Error() != want {
		t.Fatalf("Check() error = %q, want %q preserving ErrNotFound", err, want)
	}
}

func TestServiceRefreshMapsMetadataWithoutFetchingBodies(t *testing.T) {
	client := &fakePageClient{pages: []Page{
		{ID: "2", Space: "DOC", Title: "Two", URL: "https://wiki/2"},
		{ID: "1", Space: "DOC", Title: "One", URL: "https://wiki/1", UpdatedAt: "2026-09-04"},
	}}
	store := &fakeSyncStore{}
	service := newConfluenceTestService(t.TempDir(), serviceConfigValues(), &fakeSecretReader{}, store, client, &fakeMaterializer{})
	item, count, err := service.Refresh(context.Background(), "workspace")
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if item.ID != "workspace-id" || count != 2 || client.validateCalls != 1 || client.discoverCalls != 1 || client.fetchCalls != 0 {
		t.Fatalf("Refresh() = %#v/%d calls=%d/%d/%d", item, count, client.validateCalls, client.discoverCalls, client.fetchCalls)
	}
	wantSource := resource.Source{Provider: "confluence", Name: "https://wiki.example/confluence", BaseURL: "https://wiki.example/confluence"}
	if store.source != wantSource || store.resourceType != pageResourceType || store.workspaceID != "workspace-id" {
		t.Errorf("stored source/type/workspace = %#v/%q/%q", store.source, store.resourceType, store.workspaceID)
	}
	want := []resource.Discovered{
		{ProviderID: "2", ExternalKey: "DOC/2", Name: "Two", URL: "https://wiki/2"},
		{ProviderID: "1", ExternalKey: "DOC/1", Name: "One", URL: "https://wiki/1", Metadata: `{"confluence_updated_at":"2026-09-04"}`},
	}
	if !reflect.DeepEqual(store.discovered, want) {
		t.Errorf("discovered = %#v, want %#v", store.discovered, want)
	}
}

func TestServiceListIsOfflineAndDerivesState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wiki", "one.md")
	store := &fakeSyncStore{items: []resource.Located{{
		Resource:     resource.Resource{ID: "r1", ProviderID: "1", ExternalKey: "DOC/1", Name: "One", URL: "https://wiki/1"},
		LocationPath: path,
	}}}
	client := &fakePageClient{}
	materializer := &fakeMaterializer{inspections: map[string]wiki.FileInspection{path: {Exists: true, Regular: true}}}
	service := newConfluenceTestService(t.TempDir(), nil, &fakeSecretReader{err: secret.ErrNotFound}, store, client, materializer)
	_, listed, err := service.List(context.Background(), "workspace")
	if err != nil || len(listed) != 1 || listed[0].State != wiki.StateFetched {
		t.Fatalf("List() = %#v, %v", listed, err)
	}
	if client.validateCalls != 0 || client.discoverCalls != 0 || client.fetchCalls != 0 || store.upsertCalls != 0 {
		t.Fatalf("List() made remote calls")
	}
}

func TestServiceFetchSelectedRefreshesAndPreservesSelectorOrder(t *testing.T) {
	workspacePath := t.TempDir()
	client := &fakePageClient{pages: []Page{
		{ID: "1", Space: "DOC", Title: "One", URL: "https://wiki/1"},
		{ID: "2", Space: "DOC", Title: "Two", URL: "https://wiki/2"},
	}}
	store := &fakeSyncStore{}
	materializer := &fakeMaterializer{}
	service := newConfluenceTestService(workspacePath, serviceConfigValues(), &fakeSecretReader{}, store, client, materializer)
	_, results, err := service.FetchSelected(context.Background(), "workspace", []string{"2", "One"})
	if err != nil {
		t.Fatalf("FetchSelected() error = %v", err)
	}
	if len(results) != 2 || results[0].PageID != "2" || results[1].PageID != "1" || results[0].Status != "fetched" || results[1].Status != "fetched" {
		t.Fatalf("FetchSelected() results = %#v", results)
	}
	if !reflect.DeepEqual(client.fetchedIDs, []string{"2", "1"}) || store.setLocationCalls != 2 {
		t.Fatalf("fetch/location calls = %v/%d", client.fetchedIDs, store.setLocationCalls)
	}
	if results[0].Path != filepath.Join(workspacePath, "wiki", "two-2.md") || results[1].Path != filepath.Join(workspacePath, "wiki", "one-1.md") {
		t.Errorf("paths = %#v", results)
	}
}

func TestServiceFetchAllSortsAndContinuesAfterFailure(t *testing.T) {
	client := &fakePageClient{
		pages: []Page{
			{ID: "2", Space: "DOC", Title: "Zulu", URL: "https://wiki/2"},
			{ID: "1", Space: "DOC", Title: "Alpha", URL: "https://wiki/1"},
		},
		fetchErrors: map[string]error{"1": errors.New("fetch failed")},
	}
	store := &fakeSyncStore{}
	service := newConfluenceTestService(t.TempDir(), serviceConfigValues(), &fakeSecretReader{}, store, client, &fakeMaterializer{})
	_, results, err := service.FetchAll(context.Background(), "workspace")
	if !errors.Is(err, ErrFetchFailed) || err.Error() != "wiki fetch failed for 1 page(s)" {
		t.Fatalf("FetchAll() error = %v", err)
	}
	if len(results) != 2 || results[0].PageID != "1" || results[0].Error == nil || results[1].PageID != "2" || results[1].Status != "fetched" {
		t.Fatalf("FetchAll() results = %#v", results)
	}
	if !reflect.DeepEqual(client.fetchedIDs, []string{"1", "2"}) || store.setLocationCalls != 1 {
		t.Errorf("calls = fetch %v location %d", client.fetchedIDs, store.setLocationCalls)
	}
}

func TestServiceFetchSelectionErrorsFetchNoBodies(t *testing.T) {
	client := &fakePageClient{pages: []Page{{ID: "1", Space: "DOC", Title: "Same"}, {ID: "2", Space: "DOC", Title: "Same"}}}
	service := newConfluenceTestService(t.TempDir(), serviceConfigValues(), &fakeSecretReader{}, &fakeSyncStore{}, client, &fakeMaterializer{})
	_, _, err := service.FetchSelected(context.Background(), "workspace", []string{"Same"})
	if err == nil || !strings.Contains(err.Error(), "ambiguous") || client.fetchCalls != 0 {
		t.Fatalf("FetchSelected() error/calls = %v/%d", err, client.fetchCalls)
	}
}

func TestServiceFetchRefusesConflictAndUnsafeTrackedPath(t *testing.T) {
	workspacePath := t.TempDir()
	page := Page{ID: "1", Space: "DOC", Title: "One", URL: "https://wiki/1"}
	for _, test := range []struct {
		name          string
		location      string
		inspections   map[string]wiki.FileInspection
		wantSubstring string
	}{
		{
			name: "untracked conflict",
			inspections: map[string]wiki.FileInspection{
				filepath.Join(workspacePath, "wiki", "one-1.md"): {Exists: true, Regular: true},
			},
			wantSubstring: "already exists",
		},
		{name: "tracked outside root", location: filepath.Join(workspacePath, "outside.md"), wantSubstring: "outside the workspace wiki directory"},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &fakeSyncStore{items: []resource.Located{{Resource: resource.Resource{ID: "r1", ProviderID: "1", ExternalKey: "DOC/1", Name: "One", URL: page.URL}, LocationPath: test.location}}}
			client := &fakePageClient{pages: []Page{page}}
			service := newConfluenceTestService(workspacePath, serviceConfigValues(), &fakeSecretReader{}, store, client, &fakeMaterializer{inspections: test.inspections})
			_, results, err := service.FetchAll(context.Background(), "workspace")
			if !errors.Is(err, ErrFetchFailed) || len(results) != 1 || !strings.Contains(results[0].Error.Error(), test.wantSubstring) {
				t.Fatalf("FetchAll() = %#v, %v", results, err)
			}
			if client.fetchCalls != 0 || store.setLocationCalls != 0 {
				t.Fatalf("unsafe path fetched or registered: %d/%d", client.fetchCalls, store.setLocationCalls)
			}
		})
	}
}

func TestServiceFetchRollsBackNewFileWhenLocationFails(t *testing.T) {
	client := &fakePageClient{pages: []Page{{ID: "1", Space: "DOC", Title: "One", URL: "https://wiki/1"}}}
	store := &fakeSyncStore{setLocationErr: errors.New("location failed")}
	materializer := &fakeMaterializer{}
	service := newConfluenceTestService(t.TempDir(), serviceConfigValues(), &fakeSecretReader{}, store, client, materializer)
	_, results, err := service.FetchAll(context.Background(), "workspace")
	if !errors.Is(err, ErrFetchFailed) || len(results) != 1 || results[0].Error == nil {
		t.Fatalf("FetchAll() = %#v, %v", results, err)
	}
	if !reflect.DeepEqual(materializer.removed, []string{results[0].Path}) {
		t.Fatalf("removed = %v, want %q", materializer.removed, results[0].Path)
	}
}

func TestServiceFetchRetainsTrackedFilenameAndRestoresMissingFile(t *testing.T) {
	workspacePath := t.TempDir()
	tracked := filepath.Join(workspacePath, "wiki", "old-title-1.md")
	store := &fakeSyncStore{items: []resource.Located{{Resource: resource.Resource{ID: "r1", ProviderID: "1", ExternalKey: "DOC/1", Name: "Old title"}, LocationPath: tracked}}}
	client := &fakePageClient{pages: []Page{{ID: "1", Space: "DOC", Title: "New title", URL: "https://wiki/1"}}}
	materializer := &fakeMaterializer{}
	service := newConfluenceTestService(workspacePath, serviceConfigValues(), &fakeSecretReader{}, store, client, materializer)
	_, results, err := service.FetchAll(context.Background(), "workspace")
	if err != nil || len(results) != 1 || results[0].Path != tracked || results[0].Status != "fetched" {
		t.Fatalf("FetchAll() = %#v, %v", results, err)
	}
	if _, written := materializer.writes[tracked]; !written {
		t.Fatalf("tracked path was not restored: writes=%v", materializer.writes)
	}
}

func TestServiceFetchWriteAndConversionFailuresDoNotRegisterLocation(t *testing.T) {
	for _, test := range []struct {
		name      string
		configure func(*fakePageClient, *fakeMaterializer, string)
	}{
		{
			name: "write failure",
			configure: func(_ *fakePageClient, materializer *fakeMaterializer, path string) {
				materializer.writeErrors = map[string]error{path: errors.New("write failed")}
			},
		},
		{
			name: "conversion failure",
			configure: func(client *fakePageClient, _ *fakeMaterializer, _ string) {
				client.fetchContents = map[string]PageContent{"1": {Page: Page{ID: "invalid", Space: "DOC", Title: "One", URL: "https://wiki/1"}}}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			workspacePath := t.TempDir()
			path := filepath.Join(workspacePath, "wiki", "one-1.md")
			client := &fakePageClient{pages: []Page{{ID: "1", Space: "DOC", Title: "One", URL: "https://wiki/1"}}}
			store := &fakeSyncStore{}
			materializer := &fakeMaterializer{}
			test.configure(client, materializer, path)
			service := newConfluenceTestService(workspacePath, serviceConfigValues(), &fakeSecretReader{}, store, client, materializer)
			_, results, err := service.FetchAll(context.Background(), "workspace")
			if !errors.Is(err, ErrFetchFailed) || len(results) != 1 || results[0].Error == nil {
				t.Fatalf("FetchAll() = %#v, %v", results, err)
			}
			if store.setLocationCalls != 0 || len(materializer.writes) != 0 {
				t.Fatalf("failed fetch registered/wrote: %d/%v", store.setLocationCalls, materializer.writes)
			}
		})
	}
}

func TestServiceRepeatedFetchOverwritesTrackedGeneratedFile(t *testing.T) {
	workspacePath := t.TempDir()
	client := &fakePageClient{pages: []Page{{ID: "1", Space: "DOC", Title: "One", URL: "https://wiki/1"}}}
	store := &fakeSyncStore{}
	materializer := &fakeMaterializer{}
	service := newConfluenceTestService(workspacePath, serviceConfigValues(), &fakeSecretReader{}, store, client, materializer)
	for attempt := range 2 {
		if _, _, err := service.FetchAll(context.Background(), "workspace"); err != nil {
			t.Fatalf("FetchAll() attempt %d error = %v", attempt+1, err)
		}
	}
	path := filepath.Join(workspacePath, "wiki", "one-1.md")
	if materializer.writeCalls[path] != 2 || store.setLocationCalls != 2 {
		t.Fatalf("write/location calls = %d/%d", materializer.writeCalls[path], store.setLocationCalls)
	}
}

func newConfluenceTestService(path string, values map[string]string, secrets *fakeSecretReader, store *fakeSyncStore, client *fakePageClient, materializer *fakeMaterializer) *Service {
	return NewService(
		&confluenceFakeWorkspaceRepository{item: workspace.Workspace{ID: "workspace-id", Name: "workspace", LocalPath: path}},
		&confluenceFakeConfigRepository{values: values},
		store,
		secrets,
		client,
		materializer,
	)
}

func serviceConfigValues() map[string]string {
	return map[string]string{"base_url": "https://wiki.example/confluence/", "space": "DOC", "secret": "secret:confluence.pat"}
}

type confluenceFakeWorkspaceRepository struct{ item workspace.Workspace }

func (r *confluenceFakeWorkspaceRepository) Create(context.Context, workspace.Workspace) error {
	return nil
}
func (r *confluenceFakeWorkspaceRepository) List(context.Context) ([]workspace.Workspace, error) {
	return []workspace.Workspace{r.item}, nil
}
func (r *confluenceFakeWorkspaceRepository) GetByID(_ context.Context, id string) (workspace.Workspace, error) {
	if id == r.item.ID {
		return r.item, nil
	}
	return workspace.Workspace{}, workspace.ErrNotFound
}
func (r *confluenceFakeWorkspaceRepository) GetByName(_ context.Context, name string) (workspace.Workspace, error) {
	if name == r.item.Name {
		return r.item, nil
	}
	return workspace.Workspace{}, workspace.ErrNotFound
}
func (r *confluenceFakeWorkspaceRepository) Delete(context.Context, string) error { return nil }

type confluenceFakeConfigRepository struct{ values map[string]string }

func (r *confluenceFakeConfigRepository) Set(context.Context, string, workspace.ConfigEntry) error {
	return nil
}
func (r *confluenceFakeConfigRepository) Get(context.Context, string, string, string) (workspace.ConfigEntry, error) {
	return workspace.ConfigEntry{}, workspace.ErrConfigNotFound
}
func (r *confluenceFakeConfigRepository) List(context.Context, string) ([]workspace.ConfigEntry, error) {
	return nil, nil
}
func (r *confluenceFakeConfigRepository) ListNamespace(_ context.Context, _, namespace string) ([]workspace.ConfigEntry, error) {
	entries := make([]workspace.ConfigEntry, 0, len(r.values))
	for key, value := range r.values {
		entries = append(entries, workspace.ConfigEntry{Namespace: namespace, Key: key, Value: value})
	}
	return entries, nil
}
func (r *confluenceFakeConfigRepository) Unset(context.Context, string, string, string) error {
	return nil
}
func (r *confluenceFakeConfigRepository) ReplaceAll(context.Context, string, []workspace.ConfigEntry) error {
	return nil
}
func (r *confluenceFakeConfigRepository) ReplaceUser(context.Context, string, []workspace.ConfigEntry) error {
	return nil
}

type fakeSecretReader struct{ err error }

func (r *fakeSecretReader) Get(context.Context, string) (secret.Secret, error) {
	if r.err != nil {
		return secret.Secret{}, r.err
	}
	return secret.Secret{Key: "confluence.pat", Value: "secret-value"}, nil
}

type fakePageClient struct {
	pages         []Page
	fetchErrors   map[string]error
	fetchContents map[string]PageContent
	validateErr   error
	discoverErr   error
	validateCalls int
	discoverCalls int
	fetchCalls    int
	fetchedIDs    []string
}

func (c *fakePageClient) Validate(context.Context, Config, string) error {
	c.validateCalls++
	return c.validateErr
}
func (c *fakePageClient) Discover(context.Context, Config, string) ([]Page, error) {
	c.discoverCalls++
	return append([]Page(nil), c.pages...), c.discoverErr
}
func (c *fakePageClient) Fetch(_ context.Context, _ Config, _ string, pageID string) (PageContent, error) {
	c.fetchCalls++
	c.fetchedIDs = append(c.fetchedIDs, pageID)
	if err := c.fetchErrors[pageID]; err != nil {
		return PageContent{}, err
	}
	if content, exists := c.fetchContents[pageID]; exists {
		return content, nil
	}
	for _, page := range c.pages {
		if page.ID == pageID {
			return PageContent{Page: page, StorageHTML: "<p>Body</p>"}, nil
		}
	}
	return PageContent{}, errors.New("page missing")
}

type fakeSyncStore struct {
	items            []resource.Located
	source           resource.Source
	workspaceID      string
	resourceType     string
	discovered       []resource.Discovered
	upsertCalls      int
	listCalls        int
	setLocationCalls int
	setLocationErr   error
}

func (s *fakeSyncStore) UpsertDiscovered(_ context.Context, source resource.Source, workspaceID, resourceType string, discovered []resource.Discovered) error {
	s.upsertCalls++
	s.source = source
	s.workspaceID = workspaceID
	s.resourceType = resourceType
	s.discovered = append([]resource.Discovered(nil), discovered...)
	byProviderID := make(map[string]int, len(s.items))
	for i, item := range s.items {
		byProviderID[item.Resource.ProviderID] = i
	}
	for _, item := range discovered {
		if index, exists := byProviderID[item.ProviderID]; exists {
			s.items[index].Resource.ExternalKey = item.ExternalKey
			s.items[index].Resource.Name = item.Name
			s.items[index].Resource.URL = item.URL
			s.items[index].Resource.Metadata = item.Metadata
			continue
		}
		s.items = append(s.items, resource.Located{Resource: resource.Resource{ID: "r" + item.ProviderID, Type: resourceType, ProviderID: item.ProviderID, ExternalKey: item.ExternalKey, Name: item.Name, URL: item.URL, Metadata: item.Metadata}})
	}
	return nil
}
func (s *fakeSyncStore) ListByWorkspace(context.Context, string, string, string) ([]resource.Located, error) {
	s.listCalls++
	return append([]resource.Located(nil), s.items...), nil
}
func (s *fakeSyncStore) SetLocation(_ context.Context, _, resourceID, _, path string) error {
	s.setLocationCalls++
	if s.setLocationErr != nil {
		return s.setLocationErr
	}
	for i := range s.items {
		if s.items[i].Resource.ID == resourceID {
			s.items[i].LocationPath = path
		}
	}
	return nil
}

type fakeMaterializer struct {
	inspections map[string]wiki.FileInspection
	writes      map[string][]byte
	writeErrors map[string]error
	removed     []string
	ensureErr   error
	writeCalls  map[string]int
}

func (m *fakeMaterializer) Inspect(path string) (wiki.FileInspection, error) {
	return m.inspections[path], nil
}
func (m *fakeMaterializer) EnsureRoot(string) error { return m.ensureErr }
func (m *fakeMaterializer) WriteAtomic(path string, content []byte) error {
	if err := m.writeErrors[path]; err != nil {
		return err
	}
	if m.writes == nil {
		m.writes = make(map[string][]byte)
	}
	if m.writeCalls == nil {
		m.writeCalls = make(map[string]int)
	}
	m.writeCalls[path]++
	m.writes[path] = append([]byte(nil), content...)
	if m.inspections == nil {
		m.inspections = make(map[string]wiki.FileInspection)
	}
	m.inspections[path] = wiki.FileInspection{Exists: true, Regular: true}
	return nil
}
func (m *fakeMaterializer) Remove(path string) error {
	m.removed = append(m.removed, path)
	delete(m.writes, path)
	delete(m.inspections, path)
	return nil
}

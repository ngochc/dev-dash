package github

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	repositorydomain "github.com/ngochc/dev-dash/internal/repository"
	"github.com/ngochc/dev-dash/internal/workspace"
)

func TestServiceRefreshValidatesConfigBeforeGitHub(t *testing.T) {
	for _, test := range []struct {
		name   string
		values map[string]string
		want   error
	}{
		{name: "missing organization", want: ErrIncompleteConfig},
		{name: "invalid base URL", values: map[string]string{"org": "team", "base_url": "foo"}, want: ErrInvalidConfig},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := &fakeRepositoryClient{}
			store := &fakeRepositoryStore{}
			service := newTestService(test.values, store, client, &fakeCheckoutInspector{}, &fakeDirectoryManager{})
			_, _, err := service.Refresh(context.Background(), "workspace")
			if !errors.Is(err, test.want) {
				t.Fatalf("Refresh() error = %v, want %v", err, test.want)
			}
			if client.validateCalls != 0 || client.discoverCalls != 0 || store.upsertCalls != 0 {
				t.Fatalf("invalid config invoked dependencies: client=%d/%d store=%d", client.validateCalls, client.discoverCalls, store.upsertCalls)
			}
		})
	}
}

func TestServiceRefreshDiscoversAndStoresRepositories(t *testing.T) {
	client := &fakeRepositoryClient{repositories: []Repository{
		{ID: "R2", Name: "web", NameWithOwner: "team/web", URL: "https://git.example.com/team/web"},
		{ID: "R1", Name: "api", NameWithOwner: "team/api", URL: "https://git.example.com/team/api"},
	}}
	store := &fakeRepositoryStore{}
	service := newTestService(
		map[string]string{"org": "team", "base_url": "https://git.example.com"},
		store,
		client,
		&fakeCheckoutInspector{},
		&fakeDirectoryManager{},
	)
	item, count, err := service.Refresh(context.Background(), "workspace")
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if item.ID != "workspace-id" || count != 2 {
		t.Errorf("Refresh() = (%#v, %d), want workspace and count 2", item, count)
	}
	wantConfig := Config{BaseURL: "https://git.example.com", Host: "git.example.com", Organization: "team"}
	if client.validatedConfig != wantConfig || client.discoveredConfig != wantConfig {
		t.Errorf("client configs = %#v / %#v, want %#v", client.validatedConfig, client.discoveredConfig, wantConfig)
	}
	wantSource := repositorydomain.Source{Provider: "github", Name: "git.example.com", BaseURL: "https://git.example.com"}
	if store.source != wantSource || store.workspaceID != "workspace-id" {
		t.Errorf("store source/workspace = %#v/%q, want %#v/workspace-id", store.source, store.workspaceID, wantSource)
	}
	wantRemotes := []repositorydomain.Remote{
		{ProviderID: "R2", ExternalKey: "team/web", Name: "web", URL: "https://git.example.com/team/web"},
		{ProviderID: "R1", ExternalKey: "team/api", Name: "api", URL: "https://git.example.com/team/api"},
	}
	if !reflect.DeepEqual(store.remotes, wantRemotes) {
		t.Errorf("stored remotes = %#v, want %#v", store.remotes, wantRemotes)
	}
}

func TestServiceCloneAllHandlesDerivedStatesIndependently(t *testing.T) {
	workspacePath := filepath.Join(t.TempDir(), "workspace")
	items := []repositorydomain.Repository{
		{ResourceID: "1", ExternalKey: "team/new", Name: "new", URL: "https://github.com/team/new"},
		{ResourceID: "2", ExternalKey: "team/cloned", Name: "cloned", URL: "https://github.com/team/cloned", CheckoutPath: filepath.Join(workspacePath, "repos", "cloned")},
		{ResourceID: "3", ExternalKey: "team/missing", Name: "missing", URL: "https://github.com/team/missing", CheckoutPath: filepath.Join(workspacePath, "repos", "missing")},
		{ResourceID: "4", ExternalKey: "team/invalid", Name: "invalid", URL: "https://github.com/team/invalid", CheckoutPath: filepath.Join(workspacePath, "repos", "invalid")},
	}
	store := &fakeRepositoryStore{items: items}
	inspector := &fakeCheckoutInspector{results: map[string][]repositorydomain.CheckoutInspection{
		filepath.Join(workspacePath, "repos", "new"):     {{}, {Exists: true, Valid: true}},
		filepath.Join(workspacePath, "repos", "cloned"):  {{Exists: true, Valid: true}},
		filepath.Join(workspacePath, "repos", "missing"): {{}, {Exists: true, Valid: true}},
		filepath.Join(workspacePath, "repos", "invalid"): {{Exists: true}},
	}}
	client := &fakeRepositoryClient{}
	directories := &fakeDirectoryManager{}
	service := newTestServiceWithPath(workspacePath, map[string]string{"org": "team"}, store, client, inspector, directories)

	_, results, err := service.Clone(context.Background(), "workspace", nil, true)
	if !errors.Is(err, ErrCloneFailed) {
		t.Fatalf("Clone() error = %v, want ErrCloneFailed", err)
	}
	wantResults := []repositorydomain.CloneResult{
		{Repository: "team/cloned", Status: "already cloned"},
		{Repository: "team/invalid", Error: errors.New("destination exists but is not the expected repository")},
		{Repository: "team/missing", Status: "restored"},
		{Repository: "team/new", Status: "cloned"},
	}
	if len(results) != len(wantResults) {
		t.Fatalf("Clone() results = %#v, want %d results", results, len(wantResults))
	}
	for i := range wantResults {
		if results[i].Repository != wantResults[i].Repository || results[i].Status != wantResults[i].Status {
			t.Errorf("result %d = %#v, want %#v", i, results[i], wantResults[i])
		}
		if (results[i].Error == nil) != (wantResults[i].Error == nil) {
			t.Errorf("result %d error = %v, want error presence %v", i, results[i].Error, wantResults[i].Error != nil)
		}
	}
	wantClones := []string{"team/missing", "team/new"}
	if !reflect.DeepEqual(client.clones, wantClones) {
		t.Errorf("clone calls = %v, want %v", client.clones, wantClones)
	}
	if len(store.checkouts) != 2 {
		t.Errorf("registered checkouts = %#v, want missing and new", store.checkouts)
	}
}

func TestServiceCloneAdoptsExpectedDestination(t *testing.T) {
	workspacePath := filepath.Join(t.TempDir(), "workspace")
	destination := filepath.Join(workspacePath, "repos", "api")
	store := &fakeRepositoryStore{items: []repositorydomain.Repository{{
		ResourceID: "1", ExternalKey: "team/api", Name: "api", URL: "https://github.com/team/api",
	}}}
	inspector := &fakeCheckoutInspector{results: map[string][]repositorydomain.CheckoutInspection{
		destination: {{Exists: true, Valid: true}},
	}}
	client := &fakeRepositoryClient{}
	service := newTestServiceWithPath(workspacePath, map[string]string{"org": "team"}, store, client, inspector, &fakeDirectoryManager{})
	_, results, err := service.Clone(context.Background(), "workspace", []string{"api"}, false)
	if err != nil {
		t.Fatalf("Clone() error = %v", err)
	}
	if len(results) != 1 || results[0].Status != "already cloned" || len(client.clones) != 0 {
		t.Errorf("Clone() results/calls = %#v/%v, want adopted checkout", results, client.clones)
	}
	if len(store.checkouts) != 1 || store.checkouts[0].path != destination {
		t.Errorf("registered checkouts = %#v, want %q", store.checkouts, destination)
	}
}

func TestServiceCloneRejectsConflictAndCloneFailure(t *testing.T) {
	for _, test := range []struct {
		name          string
		inspections   []repositorydomain.CheckoutInspection
		cloneErr      error
		wantCloneCall bool
		wantPartial   bool
	}{
		{name: "conflicting destination", inspections: []repositorydomain.CheckoutInspection{{Exists: true}}},
		{name: "clone failure", inspections: []repositorydomain.CheckoutInspection{{}, {Exists: true}}, cloneErr: errors.New("clone failed"), wantCloneCall: true, wantPartial: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			workspacePath := filepath.Join(t.TempDir(), "workspace")
			destination := filepath.Join(workspacePath, "repos", "api")
			store := &fakeRepositoryStore{items: []repositorydomain.Repository{{ResourceID: "1", ExternalKey: "team/api", Name: "api", URL: "https://github.com/team/api"}}}
			inspector := &fakeCheckoutInspector{results: map[string][]repositorydomain.CheckoutInspection{destination: test.inspections}}
			client := &fakeRepositoryClient{cloneErrors: map[string]error{"team/api": test.cloneErr}}
			service := newTestServiceWithPath(workspacePath, map[string]string{"org": "team"}, store, client, inspector, &fakeDirectoryManager{})
			_, results, err := service.Clone(context.Background(), "workspace", []string{"api"}, false)
			if !errors.Is(err, ErrCloneFailed) || len(results) != 1 || results[0].Error == nil {
				t.Fatalf("Clone() = %#v, %v, want one failure", results, err)
			}
			if (len(client.clones) > 0) != test.wantCloneCall {
				t.Errorf("clone calls = %v, want called %v", client.clones, test.wantCloneCall)
			}
			if test.wantPartial && !strings.Contains(results[0].Error.Error(), "partial destination remains at "+destination) {
				t.Errorf("clone failure error = %q, want partial destination", results[0].Error)
			}
			if len(store.checkouts) != 0 {
				t.Errorf("failed clone registered checkout %#v", store.checkouts)
			}
		})
	}
}

func TestServiceCloneSelectionErrorsBeforeClone(t *testing.T) {
	items := []repositorydomain.Repository{
		{ResourceID: "1", ExternalKey: "one/common", Name: "common"},
		{ResourceID: "2", ExternalKey: "two/common", Name: "common"},
	}
	for _, test := range []struct {
		selector string
		want     string
	}{
		{selector: "common", want: `repository "common" is ambiguous; use owner/repository`},
		{selector: "missing", want: `repository "missing" not found`},
	} {
		store := &fakeRepositoryStore{items: items}
		client := &fakeRepositoryClient{}
		service := newTestService(map[string]string{"org": "team"}, store, client, &fakeCheckoutInspector{}, &fakeDirectoryManager{})
		_, _, err := service.Clone(context.Background(), "workspace", []string{test.selector}, false)
		if err == nil || err.Error() != test.want {
			t.Errorf("Clone(%q) error = %v, want %q", test.selector, err, test.want)
		}
		if len(client.clones) != 0 {
			t.Errorf("Clone(%q) invoked clone: %v", test.selector, client.clones)
		}
	}
}

func newTestService(values map[string]string, store *fakeRepositoryStore, client *fakeRepositoryClient, inspector *fakeCheckoutInspector, directories *fakeDirectoryManager) *Service {
	return newTestServiceWithPath("/workspace", values, store, client, inspector, directories)
}

func newTestServiceWithPath(path string, values map[string]string, store *fakeRepositoryStore, client *fakeRepositoryClient, inspector *fakeCheckoutInspector, directories *fakeDirectoryManager) *Service {
	return NewService(
		&fakeWorkspaceRepository{item: workspace.Workspace{ID: "workspace-id", Name: "workspace", LocalPath: path}},
		&fakeConfigRepository{values: values},
		store,
		client,
		inspector,
		directories,
	)
}

type fakeWorkspaceRepository struct {
	item workspace.Workspace
}

func (r *fakeWorkspaceRepository) Create(context.Context, workspace.Workspace) error { return nil }
func (r *fakeWorkspaceRepository) List(context.Context) ([]workspace.Workspace, error) {
	return []workspace.Workspace{r.item}, nil
}
func (r *fakeWorkspaceRepository) GetByID(_ context.Context, id string) (workspace.Workspace, error) {
	if id == r.item.ID {
		return r.item, nil
	}
	return workspace.Workspace{}, workspace.ErrNotFound
}
func (r *fakeWorkspaceRepository) GetByName(_ context.Context, name string) (workspace.Workspace, error) {
	if name == r.item.Name {
		return r.item, nil
	}
	return workspace.Workspace{}, workspace.ErrNotFound
}
func (r *fakeWorkspaceRepository) Delete(context.Context, string) error { return nil }

type fakeConfigRepository struct {
	values map[string]string
}

func (r *fakeConfigRepository) Set(context.Context, string, workspace.ConfigEntry) error { return nil }
func (r *fakeConfigRepository) Get(context.Context, string, string, string) (workspace.ConfigEntry, error) {
	return workspace.ConfigEntry{}, workspace.ErrConfigNotFound
}
func (r *fakeConfigRepository) List(context.Context, string) ([]workspace.ConfigEntry, error) {
	return nil, nil
}
func (r *fakeConfigRepository) ListNamespace(_ context.Context, _, namespace string) ([]workspace.ConfigEntry, error) {
	entries := make([]workspace.ConfigEntry, 0, len(r.values))
	for key, value := range r.values {
		entries = append(entries, workspace.ConfigEntry{Namespace: namespace, Key: key, Value: value})
	}
	return entries, nil
}
func (r *fakeConfigRepository) Unset(context.Context, string, string, string) error { return nil }
func (r *fakeConfigRepository) ReplaceAll(context.Context, string, []workspace.ConfigEntry) error {
	return nil
}
func (r *fakeConfigRepository) ReplaceUser(context.Context, string, []workspace.ConfigEntry) error {
	return nil
}

type fakeRepositoryClient struct {
	repositories     []Repository
	cloneErrors      map[string]error
	validateCalls    int
	discoverCalls    int
	validatedConfig  Config
	discoveredConfig Config
	clones           []string
}

func (c *fakeRepositoryClient) Validate(_ context.Context, config Config) error {
	c.validateCalls++
	c.validatedConfig = config
	return nil
}
func (c *fakeRepositoryClient) Discover(_ context.Context, config Config) ([]Repository, error) {
	c.discoverCalls++
	c.discoveredConfig = config
	return append([]Repository(nil), c.repositories...), nil
}
func (c *fakeRepositoryClient) Clone(_ context.Context, _ Config, repository, _ string) error {
	c.clones = append(c.clones, repository)
	return c.cloneErrors[repository]
}

type fakeRepositoryStore struct {
	items       []repositorydomain.Repository
	upsertCalls int
	source      repositorydomain.Source
	workspaceID string
	remotes     []repositorydomain.Remote
	checkouts   []fakeCheckout
}

type fakeCheckout struct {
	workspaceID string
	resourceID  string
	path        string
}

func (s *fakeRepositoryStore) UpsertDiscovered(_ context.Context, source repositorydomain.Source, workspaceID string, remotes []repositorydomain.Remote) error {
	s.upsertCalls++
	s.source = source
	s.workspaceID = workspaceID
	s.remotes = append([]repositorydomain.Remote(nil), remotes...)
	return nil
}
func (s *fakeRepositoryStore) ListByWorkspace(context.Context, string) ([]repositorydomain.Repository, error) {
	return append([]repositorydomain.Repository(nil), s.items...), nil
}
func (s *fakeRepositoryStore) SetCheckout(_ context.Context, workspaceID, resourceID, path string) error {
	s.checkouts = append(s.checkouts, fakeCheckout{workspaceID: workspaceID, resourceID: resourceID, path: path})
	return nil
}

type fakeCheckoutInspector struct {
	results map[string][]repositorydomain.CheckoutInspection
}

func (i *fakeCheckoutInspector) Inspect(_ context.Context, path, _ string) (repositorydomain.CheckoutInspection, error) {
	results := i.results[path]
	if len(results) == 0 {
		return repositorydomain.CheckoutInspection{}, nil
	}
	result := results[0]
	i.results[path] = results[1:]
	return result, nil
}

type fakeDirectoryManager struct {
	paths []string
}

func (m *fakeDirectoryManager) Ensure(path string) error {
	m.paths = append(m.paths, path)
	return nil
}

package workspace

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParseConfigKey(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		fullKey   string
		namespace string
		key       string
	}{
		{fullKey: "github.org", namespace: "github", key: "org"},
		{fullKey: "github.api.version", namespace: "github", key: "api.version"},
		{fullKey: "GitHub-1.API_V2", namespace: "GitHub-1", key: "API_V2"},
	} {
		t.Run(test.fullKey, func(t *testing.T) {
			namespace, key, err := ParseConfigKey(test.fullKey)
			if err != nil {
				t.Fatalf("ParseConfigKey() error = %v", err)
			}
			if namespace != test.namespace || key != test.key {
				t.Errorf("ParseConfigKey() = (%q, %q), want (%q, %q)", namespace, key, test.namespace, test.key)
			}
			if got := (ConfigEntry{Namespace: namespace, Key: key}).FullKey(); got != test.fullKey {
				t.Errorf("FullKey() = %q, want %q", got, test.fullKey)
			}
		})
	}
}

func TestParseConfigKeyRejectsInvalidKeys(t *testing.T) {
	t.Parallel()

	for _, fullKey := range []string{"", "github", ".org", "github.", "git hub.org", "github.my key", "github/my.key", "github.key/value"} {
		t.Run(fullKey, func(t *testing.T) {
			_, _, err := ParseConfigKey(fullKey)
			if !errors.Is(err, ErrInvalidConfigKey) {
				t.Fatalf("ParseConfigKey() error = %v, want ErrInvalidConfigKey", err)
			}
			want := `invalid workspace config key "` + fullKey + `"`
			if err.Error() != want {
				t.Errorf("ParseConfigKey() error = %q, want %q", err, want)
			}
		})
	}
}

func TestConfigServiceSetGetAndUnset(t *testing.T) {
	workspaceRepository := newConfigWorkspaceRepository(Workspace{ID: "workspace-id", Name: "devdash"})
	configRepository := newFakeConfigRepository()
	service := NewConfigService(workspaceRepository, configRepository)
	ctx := context.Background()

	item, err := service.SetUser(ctx, "devdash", "GitHub.api.version", "  https://example.test/a=b  ")
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if item.ID != "workspace-id" {
		t.Errorf("Set() workspace ID = %q, want workspace-id", item.ID)
	}
	entry := configRepository.entries["workspace-id"]["GitHub.api.version"]
	if entry.Value != "https://example.test/a=b" {
		t.Errorf("Set() stored value = %q, want trimmed value", entry.Value)
	}

	if _, err := service.SetUser(ctx, "workspace-id", "GitHub.api.version", "secret:github-work"); err != nil {
		t.Fatalf("updated Set() error = %v", err)
	}
	_, got, err := service.Get(ctx, "devdash", "GitHub.api.version")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Value != "secret:github-work" {
		t.Errorf("Get() value = %q, want literal secret reference", got.Value)
	}

	if _, err := service.UnsetUser(ctx, "devdash", "GitHub.api.version"); err != nil {
		t.Fatalf("Unset() error = %v", err)
	}
	if _, exists := configRepository.entries["workspace-id"]["GitHub.api.version"]; exists {
		t.Fatal("Unset() left the entry present")
	}
}

func TestConfigServiceValidatesBeforeRepositoryCalls(t *testing.T) {
	for _, test := range []struct {
		name    string
		key     string
		value   string
		wantErr string
	}{
		{name: "key", key: "invalid", value: "value", wantErr: `invalid workspace config key "invalid"`},
		{name: "empty value", key: "github.org", value: " \t ", wantErr: "workspace config value is required"},
		{name: "multiline value", key: "github.org", value: "one\ntwo", wantErr: "workspace config value must be a single line"},
	} {
		t.Run(test.name, func(t *testing.T) {
			workspaceRepository := newConfigWorkspaceRepository(Workspace{ID: "workspace-id", Name: "devdash"})
			configRepository := newFakeConfigRepository()
			_, err := NewConfigService(workspaceRepository, configRepository).SetUser(context.Background(), "devdash", test.key, test.value)
			if err == nil || err.Error() != test.wantErr {
				t.Fatalf("Set() error = %v, want %q", err, test.wantErr)
			}
			if workspaceRepository.getCalls != 0 || configRepository.calls != 0 {
				t.Fatalf("validation made repository calls: workspace=%d config=%d", workspaceRepository.getCalls, configRepository.calls)
			}
		})
	}
}

func TestConfigServiceListSortsDefensively(t *testing.T) {
	workspaceRepository := newConfigWorkspaceRepository(Workspace{ID: "workspace-id", Name: "devdash"})
	configRepository := newFakeConfigRepository()
	configRepository.listResult = []ConfigEntry{
		{Namespace: "jira", Key: "project", Value: "DD"},
		{Namespace: "github", Key: "org", Value: "acme"},
		{Namespace: "github", Key: "base_url", Value: "https://github.example"},
	}

	_, entries, err := NewConfigService(workspaceRepository, configRepository).ListUser(context.Background(), "devdash")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	got := []string{entries[0].FullKey(), entries[1].FullKey(), entries[2].FullKey()}
	want := []string{"github.base_url", "github.org", "jira.project"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("List() keys = %v, want %v", got, want)
	}

	configRepository.listResult = nil
	_, entries, err = NewConfigService(workspaceRepository, configRepository).ListUser(context.Background(), "devdash")
	if err != nil {
		t.Fatalf("empty List() error = %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("empty List() = %v, want empty", entries)
	}
}

func TestConfigServiceResolvesWorkspaceIDBeforeName(t *testing.T) {
	workspaceRepository := newConfigWorkspaceRepository(
		Workspace{ID: "target", Name: "by-id"},
		Workspace{ID: "other", Name: "target"},
	)
	configRepository := newFakeConfigRepository()

	item, err := NewConfigService(workspaceRepository, configRepository).SetUser(context.Background(), "target", "github.org", "acme")
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if item.Name != "by-id" || configRepository.lastWorkspaceID != "target" {
		t.Errorf("Set() resolved workspace = %#v and repository ID %q, want ID match", item, configRepository.lastWorkspaceID)
	}
}

func TestConfigServiceNotFoundErrors(t *testing.T) {
	t.Run("workspace", func(t *testing.T) {
		_, _, err := NewConfigService(newConfigWorkspaceRepository(), newFakeConfigRepository()).Get(context.Background(), "missing", "github.org")
		if !errors.Is(err, ErrNotFound) || err.Error() != `workspace "missing" not found` {
			t.Fatalf("Get() error = %v, want exact workspace not found error", err)
		}
	})

	for _, operation := range []string{"get", "unset"} {
		t.Run(operation, func(t *testing.T) {
			service := NewConfigService(
				newConfigWorkspaceRepository(Workspace{ID: "workspace-id", Name: "devdash"}),
				newFakeConfigRepository(),
			)
			var err error
			if operation == "get" {
				_, _, err = service.Get(context.Background(), "devdash", "github.org")
			} else {
				_, err = service.UnsetUser(context.Background(), "devdash", "github.org")
			}
			if !errors.Is(err, ErrConfigNotFound) || err.Error() != `workspace config "github.org" not found` {
				t.Fatalf("%s error = %v, want exact config not found error", operation, err)
			}
		})
	}
}

func TestConfigServiceReplaceAllValidatesAndSortsBeforeCalls(t *testing.T) {
	t.Run("normalizes sorted copy", func(t *testing.T) {
		workspaceRepository := newConfigWorkspaceRepository(Workspace{ID: "workspace-id", Name: "devdash"})
		configRepository := newFakeConfigRepository()
		input := []ConfigEntry{
			{Namespace: "jira", Key: "project", Value: " DD "},
			{Namespace: "github", Key: "org", Value: " acme "},
		}
		if _, err := NewConfigService(workspaceRepository, configRepository).ReplaceUser(context.Background(), "devdash", input); err != nil {
			t.Fatalf("ReplaceAll() error = %v", err)
		}
		want := []ConfigEntry{
			{Namespace: "github", Key: "org", Value: "acme"},
			{Namespace: "jira", Key: "project", Value: "DD"},
		}
		if !reflect.DeepEqual(configRepository.replaced, want) {
			t.Errorf("ReplaceAll() entries = %#v, want %#v", configRepository.replaced, want)
		}
		if input[0].Value != " DD " {
			t.Error("ReplaceAll() mutated caller input")
		}
	})

	for _, test := range []struct {
		name    string
		entries []ConfigEntry
		wantErr string
	}{
		{
			name: "duplicate",
			entries: []ConfigEntry{
				{Namespace: "github", Key: "org", Value: "one"},
				{Namespace: "github", Key: "org", Value: "two"},
			},
			wantErr: `duplicate workspace config "github.org"`,
		},
		{
			name: "invalid later entry",
			entries: []ConfigEntry{
				{Namespace: "github", Key: "org", Value: "one"},
				{Namespace: "bad namespace", Key: "key", Value: "two"},
			},
			wantErr: `invalid workspace config key "bad namespace.key"`,
		},
		{
			name: "invalid later value",
			entries: []ConfigEntry{
				{Namespace: "github", Key: "org", Value: "one"},
				{Namespace: "jira", Key: "project", Value: "two\nlines"},
			},
			wantErr: "workspace config value must be a single line",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			workspaceRepository := newConfigWorkspaceRepository(Workspace{ID: "workspace-id", Name: "devdash"})
			configRepository := newFakeConfigRepository()
			_, err := NewConfigService(workspaceRepository, configRepository).ReplaceUser(context.Background(), "devdash", test.entries)
			if err == nil || err.Error() != test.wantErr {
				t.Fatalf("ReplaceAll() error = %v, want %q", err, test.wantErr)
			}
			if workspaceRepository.getCalls != 0 || configRepository.calls != 0 {
				t.Fatalf("validation made repository calls: workspace=%d config=%d", workspaceRepository.getCalls, configRepository.calls)
			}
		})
	}
}

func TestConfigServiceNamespace(t *testing.T) {
	workspaceRepository := newConfigWorkspaceRepository(Workspace{ID: "workspace-id", Name: "devdash"})
	configRepository := newFakeConfigRepository()
	configRepository.namespaceResult = []ConfigEntry{
		{Namespace: "github", Key: "base_url", Value: "https://github.example"},
		{Namespace: "github", Key: "org", Value: "acme"},
	}
	service := NewConfigService(workspaceRepository, configRepository)

	values, err := service.Namespace(context.Background(), "devdash", "github")
	if err != nil {
		t.Fatalf("Namespace() error = %v", err)
	}
	want := map[string]string{"base_url": "https://github.example", "org": "acme"}
	if !reflect.DeepEqual(values, want) {
		t.Errorf("Namespace() = %#v, want %#v", values, want)
	}

	configRepository.namespaceResult = nil
	values, err = service.Namespace(context.Background(), "devdash", "empty")
	if err != nil {
		t.Fatalf("empty Namespace() error = %v", err)
	}
	if values == nil || len(values) != 0 {
		t.Errorf("empty Namespace() = %#v, want non-nil empty map", values)
	}

	workspaceCalls := workspaceRepository.getCalls
	_, err = service.Namespace(context.Background(), "devdash", "bad namespace")
	if err == nil || err.Error() != `invalid workspace config namespace "bad namespace"` {
		t.Fatalf("invalid Namespace() error = %v", err)
	}
	if workspaceRepository.getCalls != workspaceCalls {
		t.Fatal("invalid namespace resolved workspace")
	}
}

func TestConfigServiceWrapsRepositoryErrors(t *testing.T) {
	repositoryError := errors.New("repository failed")
	tests := []struct {
		name    string
		context string
		setErr  func(*fakeConfigRepository)
		run     func(*ConfigService) error
	}{
		{name: "set", context: `set workspace config "github.org"`, setErr: func(r *fakeConfigRepository) { r.setErr = repositoryError }, run: func(s *ConfigService) error {
			_, err := s.SetUser(context.Background(), "devdash", "github.org", "acme")
			return err
		}},
		{name: "get", context: `get workspace config "github.org"`, setErr: func(r *fakeConfigRepository) { r.getErr = repositoryError }, run: func(s *ConfigService) error {
			_, _, err := s.Get(context.Background(), "devdash", "github.org")
			return err
		}},
		{name: "list", context: "list workspace config", setErr: func(r *fakeConfigRepository) { r.listErr = repositoryError }, run: func(s *ConfigService) error { _, _, err := s.ListUser(context.Background(), "devdash"); return err }},
		{name: "unset", context: `unset workspace config "github.org"`, setErr: func(r *fakeConfigRepository) { r.unsetErr = repositoryError }, run: func(s *ConfigService) error {
			_, err := s.UnsetUser(context.Background(), "devdash", "github.org")
			return err
		}},
		{name: "replace", context: "replace workspace config", setErr: func(r *fakeConfigRepository) { r.replaceErr = repositoryError }, run: func(s *ConfigService) error {
			_, err := s.ReplaceUser(context.Background(), "devdash", nil)
			return err
		}},
		{name: "namespace", context: `list workspace config namespace "github"`, setErr: func(r *fakeConfigRepository) { r.namespaceErr = repositoryError }, run: func(s *ConfigService) error {
			_, err := s.Namespace(context.Background(), "devdash", "github")
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configRepository := newFakeConfigRepository()
			test.setErr(configRepository)
			service := NewConfigService(newConfigWorkspaceRepository(Workspace{ID: "workspace-id", Name: "devdash"}), configRepository)
			err := test.run(service)
			if !errors.Is(err, repositoryError) {
				t.Fatalf("operation error = %v, want repository error", err)
			}
			if !strings.HasPrefix(err.Error(), test.context+": ") {
				t.Errorf("operation error = %q, want context %q", err, test.context)
			}
		})
	}
}

func TestConfigServiceReservedKeyOwnership(t *testing.T) {
	workspaceRepository := newConfigWorkspaceRepository(Workspace{ID: "workspace-id", Name: "devdash"})
	configRepository := newFakeConfigRepository()
	service := NewConfigService(workspaceRepository, configRepository)
	ctx := context.Background()

	if _, err := service.SetUser(ctx, "devdash", "_repo.test", "value"); err == nil || err.Error() != `config key "_repo.test" is reserved for internal use` {
		t.Fatalf("SetUser() error = %v, want reserved-key error", err)
	}
	if _, err := service.UnsetUser(ctx, "devdash", "_repo.test"); err == nil || err.Error() != `config key "_repo.test" is reserved for internal use` {
		t.Fatalf("UnsetUser() error = %v, want reserved-key error", err)
	}
	if workspaceRepository.getCalls != 0 || configRepository.calls != 0 {
		t.Fatalf("reserved user mutations made repository calls: workspace=%d config=%d", workspaceRepository.getCalls, configRepository.calls)
	}

	if _, err := service.SetInternal(ctx, "devdash", "_repo.test", "internal"); err != nil {
		t.Fatalf("SetInternal() error = %v", err)
	}
	if got := configRepository.entries["workspace-id"]["_repo.test"].Value; got != "internal" {
		t.Errorf("SetInternal() stored value = %q, want internal", got)
	}
	if _, err := service.SetUser(ctx, "devdash", "custom.foo", "bar"); err != nil {
		t.Fatalf("SetUser(custom key) error = %v", err)
	}
}

func TestConfigServiceUserViewsHideInternalEntries(t *testing.T) {
	workspaceRepository := newConfigWorkspaceRepository(Workspace{ID: "workspace-id", Name: "devdash"})
	configRepository := newFakeConfigRepository()
	configRepository.listResult = []ConfigEntry{
		{Namespace: "_repo", Key: "last_refresh", Value: "internal"},
		{Namespace: "github", Key: "org", Value: "acme"},
	}
	service := NewConfigService(workspaceRepository, configRepository)

	_, userEntries, err := service.ListUser(context.Background(), "devdash")
	if err != nil {
		t.Fatalf("ListUser() error = %v", err)
	}
	if len(userEntries) != 1 || userEntries[0].FullKey() != "github.org" {
		t.Errorf("ListUser() = %#v, want only github.org", userEntries)
	}
	if cap(userEntries) != len(userEntries) {
		t.Errorf("ListUser() capacity = %d, want hidden storage inaccessible beyond length %d", cap(userEntries), len(userEntries))
	}
	_, allEntries, err := service.ListAll(context.Background(), "devdash")
	if err != nil {
		t.Fatalf("ListAll() error = %v", err)
	}
	if len(allEntries) != 2 || allEntries[0].FullKey() != "_repo.last_refresh" {
		t.Errorf("ListAll() = %#v, want internal and user entries", allEntries)
	}

	configCalls := configRepository.calls
	workspaceCalls := workspaceRepository.getCalls
	_, err = service.ReplaceUser(context.Background(), "devdash", []ConfigEntry{{Namespace: "_repo", Key: "test", Value: "value"}})
	if err == nil || err.Error() != `config key "_repo.test" is reserved for internal use` {
		t.Fatalf("ReplaceUser() error = %v, want reserved-key error", err)
	}
	if configRepository.calls != configCalls || workspaceRepository.getCalls != workspaceCalls {
		t.Fatal("invalid user replacement made repository calls")
	}
}

type configWorkspaceRepository struct {
	items    []Workspace
	getCalls int
}

func newConfigWorkspaceRepository(items ...Workspace) *configWorkspaceRepository {
	return &configWorkspaceRepository{items: append([]Workspace(nil), items...)}
}

func (r *configWorkspaceRepository) Create(context.Context, Workspace) error { return nil }
func (r *configWorkspaceRepository) List(context.Context) ([]Workspace, error) {
	return append([]Workspace(nil), r.items...), nil
}
func (r *configWorkspaceRepository) GetByID(_ context.Context, id string) (Workspace, error) {
	r.getCalls++
	for _, item := range r.items {
		if item.ID == id {
			return item, nil
		}
	}
	return Workspace{}, ErrNotFound
}
func (r *configWorkspaceRepository) GetByName(_ context.Context, name string) (Workspace, error) {
	r.getCalls++
	for _, item := range r.items {
		if item.Name == name {
			return item, nil
		}
	}
	return Workspace{}, ErrNotFound
}
func (r *configWorkspaceRepository) Delete(context.Context, string) error { return nil }

type fakeConfigRepository struct {
	entries         map[string]map[string]ConfigEntry
	listResult      []ConfigEntry
	namespaceResult []ConfigEntry
	replaced        []ConfigEntry
	lastWorkspaceID string
	calls           int
	setErr          error
	getErr          error
	listErr         error
	namespaceErr    error
	unsetErr        error
	replaceErr      error
}

func newFakeConfigRepository() *fakeConfigRepository {
	return &fakeConfigRepository{entries: make(map[string]map[string]ConfigEntry)}
}

func (r *fakeConfigRepository) Set(_ context.Context, workspaceID string, entry ConfigEntry) error {
	r.calls++
	r.lastWorkspaceID = workspaceID
	if r.setErr != nil {
		return r.setErr
	}
	if r.entries[workspaceID] == nil {
		r.entries[workspaceID] = make(map[string]ConfigEntry)
	}
	r.entries[workspaceID][entry.FullKey()] = entry
	return nil
}

func (r *fakeConfigRepository) Get(_ context.Context, workspaceID, namespace, key string) (ConfigEntry, error) {
	r.calls++
	r.lastWorkspaceID = workspaceID
	if r.getErr != nil {
		return ConfigEntry{}, r.getErr
	}
	entry, ok := r.entries[workspaceID][namespace+"."+key]
	if !ok {
		return ConfigEntry{}, ErrConfigNotFound
	}
	return entry, nil
}

func (r *fakeConfigRepository) List(_ context.Context, workspaceID string) ([]ConfigEntry, error) {
	r.calls++
	r.lastWorkspaceID = workspaceID
	if r.listErr != nil {
		return nil, r.listErr
	}
	return append([]ConfigEntry(nil), r.listResult...), nil
}

func (r *fakeConfigRepository) ListNamespace(_ context.Context, workspaceID, _ string) ([]ConfigEntry, error) {
	r.calls++
	r.lastWorkspaceID = workspaceID
	if r.namespaceErr != nil {
		return nil, r.namespaceErr
	}
	return append([]ConfigEntry(nil), r.namespaceResult...), nil
}

func (r *fakeConfigRepository) Unset(_ context.Context, workspaceID, namespace, key string) error {
	r.calls++
	r.lastWorkspaceID = workspaceID
	if r.unsetErr != nil {
		return r.unsetErr
	}
	fullKey := namespace + "." + key
	if _, ok := r.entries[workspaceID][fullKey]; !ok {
		return ErrConfigNotFound
	}
	delete(r.entries[workspaceID], fullKey)
	return nil
}

func (r *fakeConfigRepository) ReplaceAll(_ context.Context, workspaceID string, entries []ConfigEntry) error {
	r.calls++
	r.lastWorkspaceID = workspaceID
	if r.replaceErr != nil {
		return r.replaceErr
	}
	r.replaced = append([]ConfigEntry(nil), entries...)
	return nil
}

func (r *fakeConfigRepository) ReplaceUser(ctx context.Context, workspaceID string, entries []ConfigEntry) error {
	return r.ReplaceAll(ctx, workspaceID, entries)
}

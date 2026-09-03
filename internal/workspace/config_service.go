package workspace

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ConfigService provides provider-neutral workspace configuration operations.
type ConfigService struct {
	workspaceService *Service
	repository       ConfigRepository
}

func NewConfigService(workspaceRepository Repository, configRepository ConfigRepository) *ConfigService {
	return &ConfigService{
		workspaceService: NewService(workspaceRepository),
		repository:       configRepository,
	}
}

func (s *ConfigService) SetUser(ctx context.Context, workspaceIdentifier, fullKey, value string) (Workspace, error) {
	return s.set(ctx, workspaceIdentifier, fullKey, value, true)
}

func (s *ConfigService) SetInternal(ctx context.Context, workspaceIdentifier, fullKey, value string) (Workspace, error) {
	return s.set(ctx, workspaceIdentifier, fullKey, value, false)
}

func (s *ConfigService) set(ctx context.Context, workspaceIdentifier, fullKey, value string, user bool) (Workspace, error) {
	namespace, key, err := ParseConfigKey(fullKey)
	if err != nil {
		return Workspace{}, err
	}
	if user && strings.HasPrefix(namespace, "_") {
		return Workspace{}, fmt.Errorf("config key %q is reserved for internal use", fullKey)
	}
	value, err = normalizeConfigValue(value)
	if err != nil {
		return Workspace{}, err
	}
	item, err := s.resolveWorkspace(ctx, workspaceIdentifier)
	if err != nil {
		return Workspace{}, err
	}
	entry := ConfigEntry{Namespace: namespace, Key: key, Value: value}
	if err := s.repository.Set(ctx, item.ID, entry); err != nil {
		return Workspace{}, fmt.Errorf("set workspace config %q: %w", fullKey, err)
	}
	return item, nil
}

func (s *ConfigService) Get(ctx context.Context, workspaceIdentifier, fullKey string) (Workspace, ConfigEntry, error) {
	namespace, key, err := ParseConfigKey(fullKey)
	if err != nil {
		return Workspace{}, ConfigEntry{}, err
	}
	item, err := s.resolveWorkspace(ctx, workspaceIdentifier)
	if err != nil {
		return Workspace{}, ConfigEntry{}, err
	}
	entry, err := s.repository.Get(ctx, item.ID, namespace, key)
	if err != nil {
		if errors.Is(err, ErrConfigNotFound) {
			return Workspace{}, ConfigEntry{}, configContextError{
				message: fmt.Sprintf("workspace config %q not found", fullKey),
				cause:   ErrConfigNotFound,
			}
		}
		return Workspace{}, ConfigEntry{}, fmt.Errorf("get workspace config %q: %w", fullKey, err)
	}
	return item, entry, nil
}

func (s *ConfigService) ListUser(ctx context.Context, workspaceIdentifier string) (Workspace, []ConfigEntry, error) {
	item, entries, err := s.list(ctx, workspaceIdentifier)
	if err != nil {
		return Workspace{}, nil, err
	}
	userEntries := make([]ConfigEntry, 0, len(entries))
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Namespace, "_") {
			userEntries = append(userEntries, entry)
		}
	}
	return item, userEntries[:len(userEntries):len(userEntries)], nil
}

func (s *ConfigService) ListAll(ctx context.Context, workspaceIdentifier string) (Workspace, []ConfigEntry, error) {
	return s.list(ctx, workspaceIdentifier)
}

func (s *ConfigService) list(ctx context.Context, workspaceIdentifier string) (Workspace, []ConfigEntry, error) {
	item, err := s.resolveWorkspace(ctx, workspaceIdentifier)
	if err != nil {
		return Workspace{}, nil, err
	}
	entries, err := s.repository.List(ctx, item.ID)
	if err != nil {
		return Workspace{}, nil, fmt.Errorf("list workspace config: %w", err)
	}
	sortConfigEntries(entries)
	return item, entries, nil
}

func (s *ConfigService) UnsetUser(ctx context.Context, workspaceIdentifier, fullKey string) (Workspace, error) {
	namespace, key, err := ParseConfigKey(fullKey)
	if err != nil {
		return Workspace{}, err
	}
	if strings.HasPrefix(namespace, "_") {
		return Workspace{}, fmt.Errorf("config key %q is reserved for internal use", fullKey)
	}
	item, err := s.resolveWorkspace(ctx, workspaceIdentifier)
	if err != nil {
		return Workspace{}, err
	}
	if err := s.repository.Unset(ctx, item.ID, namespace, key); err != nil {
		if errors.Is(err, ErrConfigNotFound) {
			return Workspace{}, configContextError{
				message: fmt.Sprintf("workspace config %q not found", fullKey),
				cause:   ErrConfigNotFound,
			}
		}
		return Workspace{}, fmt.Errorf("unset workspace config %q: %w", fullKey, err)
	}
	return item, nil
}

func (s *ConfigService) ReplaceUser(ctx context.Context, workspaceIdentifier string, entries []ConfigEntry) (Workspace, error) {
	normalized := make([]ConfigEntry, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for i, entry := range entries {
		namespace, key, err := ParseConfigKey(entry.FullKey())
		if err != nil {
			return Workspace{}, err
		}
		fullKey := namespace + "." + key
		if strings.HasPrefix(namespace, "_") {
			return Workspace{}, fmt.Errorf("config key %q is reserved for internal use", fullKey)
		}
		value, err := normalizeConfigValue(entry.Value)
		if err != nil {
			return Workspace{}, err
		}
		if _, exists := seen[fullKey]; exists {
			return Workspace{}, fmt.Errorf("duplicate workspace config %q", fullKey)
		}
		seen[fullKey] = struct{}{}
		normalized[i] = ConfigEntry{Namespace: namespace, Key: key, Value: value}
	}
	sortConfigEntries(normalized)

	item, err := s.resolveWorkspace(ctx, workspaceIdentifier)
	if err != nil {
		return Workspace{}, err
	}
	if err := s.repository.ReplaceUser(ctx, item.ID, normalized); err != nil {
		return Workspace{}, fmt.Errorf("replace workspace config: %w", err)
	}
	return item, nil
}

func (s *ConfigService) Namespace(ctx context.Context, workspaceIdentifier, namespace string) (map[string]string, error) {
	if !namespacePattern.MatchString(namespace) {
		return nil, fmt.Errorf("invalid workspace config namespace %q", namespace)
	}
	item, err := s.resolveWorkspace(ctx, workspaceIdentifier)
	if err != nil {
		return nil, err
	}
	entries, err := s.repository.ListNamespace(ctx, item.ID, namespace)
	if err != nil {
		return nil, fmt.Errorf("list workspace config namespace %q: %w", namespace, err)
	}
	values := make(map[string]string, len(entries))
	for _, entry := range entries {
		values[entry.Key] = entry.Value
	}
	return values, nil
}

func (s *ConfigService) resolveWorkspace(ctx context.Context, identifier string) (Workspace, error) {
	item, err := s.workspaceService.Get(ctx, identifier)
	if errors.Is(err, ErrNotFound) {
		return Workspace{}, configContextError{
			message: fmt.Sprintf("workspace %q not found", identifier),
			cause:   ErrNotFound,
		}
	}
	return item, err
}

func normalizeConfigValue(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("workspace config value is required")
	}
	if strings.ContainsAny(value, "\r\n") {
		return "", errors.New("workspace config value must be a single line")
	}
	return value, nil
}

func sortConfigEntries(entries []ConfigEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Namespace == entries[j].Namespace {
			return entries[i].Key < entries[j].Key
		}
		return entries[i].Namespace < entries[j].Namespace
	})
}

type configContextError struct {
	message string
	cause   error
}

func (e configContextError) Error() string { return e.message }
func (e configContextError) Unwrap() error { return e.cause }

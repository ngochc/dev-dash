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

func (s *ConfigService) Set(ctx context.Context, workspaceIdentifier, fullKey, value string) (Workspace, error) {
	namespace, key, err := ParseConfigKey(fullKey)
	if err != nil {
		return Workspace{}, err
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

func (s *ConfigService) List(ctx context.Context, workspaceIdentifier string) (Workspace, []ConfigEntry, error) {
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

func (s *ConfigService) Unset(ctx context.Context, workspaceIdentifier, fullKey string) (Workspace, error) {
	namespace, key, err := ParseConfigKey(fullKey)
	if err != nil {
		return Workspace{}, err
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

func (s *ConfigService) ReplaceAll(ctx context.Context, workspaceIdentifier string, entries []ConfigEntry) (Workspace, error) {
	normalized := make([]ConfigEntry, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for i, entry := range entries {
		namespace, key, err := ParseConfigKey(entry.FullKey())
		if err != nil {
			return Workspace{}, err
		}
		value, err := normalizeConfigValue(entry.Value)
		if err != nil {
			return Workspace{}, err
		}
		fullKey := namespace + "." + key
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
	if err := s.repository.ReplaceAll(ctx, item.ID, normalized); err != nil {
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

package confluence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ngochc/dev-dash/internal/resource"
	"github.com/ngochc/dev-dash/internal/secret"
	"github.com/ngochc/dev-dash/internal/wiki"
	"github.com/ngochc/dev-dash/internal/workspace"
)

const (
	pageResourceType    = "confluence_page"
	pageLocationType    = "materialized_file"
	confluenceNamespace = "confluence"
)

var ErrFetchFailed = errors.New("wiki fetch failed")

// PageClient provides the Confluence operations needed by the wiki service.
type PageClient interface {
	Validate(context.Context, Config, string) error
	Discover(context.Context, Config, string) ([]Page, error)
	Fetch(context.Context, Config, string, string) (PageContent, error)
}

// SecretReader resolves a configured secret reference.
type SecretReader interface {
	Get(context.Context, string) (secret.Secret, error)
}

// Service orchestrates Confluence discovery and local wiki materialization.
type Service struct {
	workspaceService *workspace.Service
	configService    *workspace.ConfigService
	store            resource.SyncStore
	secrets          SecretReader
	client           PageClient
	materializer     wiki.Materializer
}

func NewService(
	workspaceRepository workspace.Repository,
	configRepository workspace.ConfigRepository,
	store resource.SyncStore,
	secrets SecretReader,
	client PageClient,
	materializer wiki.Materializer,
) *Service {
	return &Service{
		workspaceService: workspace.NewService(workspaceRepository),
		configService:    workspace.NewConfigService(workspaceRepository, configRepository),
		store:            store,
		secrets:          secrets,
		client:           client,
		materializer:     materializer,
	}
}

func (s *Service) Check(ctx context.Context, workspaceIdentifier string) (workspace.Workspace, Config, error) {
	item, config, _, err := s.prepareRemote(ctx, workspaceIdentifier)
	if err != nil {
		return workspace.Workspace{}, Config{}, err
	}
	return item, config, nil
}

func (s *Service) Refresh(ctx context.Context, workspaceIdentifier string) (workspace.Workspace, int, error) {
	item, _, _, count, err := s.refresh(ctx, workspaceIdentifier)
	return item, count, err
}

func (s *Service) List(ctx context.Context, workspaceIdentifier string) (workspace.Workspace, []wiki.Listed, error) {
	item, err := s.workspaceService.Get(ctx, workspaceIdentifier)
	if err != nil {
		return workspace.Workspace{}, nil, err
	}
	pages, err := s.listPages(ctx, item.ID)
	if err != nil {
		return workspace.Workspace{}, nil, err
	}
	listed, err := wiki.List(pages, s.materializer)
	if err != nil {
		return workspace.Workspace{}, nil, err
	}
	return item, listed, nil
}

func (s *Service) FetchSelected(ctx context.Context, workspaceIdentifier string, selectors []string) (workspace.Workspace, []wiki.FetchResult, error) {
	return s.fetch(ctx, workspaceIdentifier, selectors, false)
}

func (s *Service) FetchAll(ctx context.Context, workspaceIdentifier string) (workspace.Workspace, []wiki.FetchResult, error) {
	return s.fetch(ctx, workspaceIdentifier, nil, true)
}

func (s *Service) fetch(ctx context.Context, workspaceIdentifier string, selectors []string, all bool) (workspace.Workspace, []wiki.FetchResult, error) {
	item, config, pat, _, err := s.refresh(ctx, workspaceIdentifier)
	if err != nil {
		return workspace.Workspace{}, nil, err
	}
	pages, err := s.listPages(ctx, item.ID)
	if err != nil {
		return workspace.Workspace{}, nil, err
	}
	selected, err := wiki.Select(pages, selectors, all)
	if err != nil {
		return workspace.Workspace{}, nil, err
	}

	results := make([]wiki.FetchResult, 0, len(selected))
	failures := 0
	for _, page := range selected {
		result := s.fetchOne(ctx, item, config, pat, page)
		if result.Error != nil {
			failures++
		}
		results = append(results, result)
	}
	if failures != 0 {
		return item, results, fetchFailureError{count: failures}
	}
	return item, results, nil
}

func (s *Service) refresh(ctx context.Context, workspaceIdentifier string) (workspace.Workspace, Config, string, int, error) {
	item, config, pat, err := s.prepareRemote(ctx, workspaceIdentifier)
	if err != nil {
		return workspace.Workspace{}, Config{}, "", 0, err
	}
	pages, err := s.client.Discover(ctx, config, pat)
	if err != nil {
		return workspace.Workspace{}, Config{}, "", 0, err
	}
	discovered := make([]resource.Discovered, len(pages))
	for i, page := range pages {
		metadata := ""
		if page.UpdatedAt != "" {
			encoded, err := json.Marshal(struct {
				UpdatedAt string `json:"confluence_updated_at"`
			}{UpdatedAt: page.UpdatedAt})
			if err != nil {
				return workspace.Workspace{}, Config{}, "", 0, fmt.Errorf("encode Confluence page metadata: %w", err)
			}
			metadata = string(encoded)
		}
		discovered[i] = resource.Discovered{
			ProviderID:  page.ID,
			ExternalKey: config.Space + "/" + page.ID,
			Name:        page.Title,
			URL:         page.URL,
			Metadata:    metadata,
		}
	}
	source := resource.Source{Provider: "confluence", Name: config.BaseURL, BaseURL: config.BaseURL}
	if err := s.store.UpsertDiscovered(ctx, source, item.ID, pageResourceType, discovered); err != nil {
		return workspace.Workspace{}, Config{}, "", 0, fmt.Errorf("store discovered wiki pages: %w", err)
	}
	return item, config, pat, len(discovered), nil
}

func (s *Service) prepareRemote(ctx context.Context, workspaceIdentifier string) (workspace.Workspace, Config, string, error) {
	item, err := s.workspaceService.Get(ctx, workspaceIdentifier)
	if err != nil {
		return workspace.Workspace{}, Config{}, "", err
	}
	values, err := s.configService.Namespace(ctx, item.ID, confluenceNamespace)
	if err != nil {
		return workspace.Workspace{}, Config{}, "", err
	}
	config, err := ResolveConfig(item.Name, values)
	if err != nil {
		return workspace.Workspace{}, Config{}, "", err
	}
	storedSecret, err := s.secrets.Get(ctx, config.SecretName)
	if err != nil {
		if errors.Is(err, secret.ErrNotFound) {
			return workspace.Workspace{}, Config{}, "", secretNotFoundError{
				message: fmt.Sprintf("Confluence secret %q was not found for workspace %q.\n\nConfigure with:\n  devdash workspace setup %s", config.SecretName, item.Name, item.Name),
				cause:   secret.ErrNotFound,
			}
		}
		return workspace.Workspace{}, Config{}, "", fmt.Errorf("get Confluence secret %q: %w", config.SecretName, err)
	}
	if err := s.client.Validate(ctx, config, storedSecret.Value); err != nil {
		return workspace.Workspace{}, Config{}, "", err
	}
	return item, config, storedSecret.Value, nil
}

func (s *Service) listPages(ctx context.Context, workspaceID string) ([]wiki.Page, error) {
	located, err := s.store.ListByWorkspace(ctx, workspaceID, pageResourceType, pageLocationType)
	if err != nil {
		return nil, fmt.Errorf("list workspace wiki pages: %w", err)
	}
	pages := make([]wiki.Page, len(located))
	for i, item := range located {
		pages[i] = wiki.Page{
			ResourceID:       item.Resource.ID,
			PageID:           item.Resource.ProviderID,
			ExternalKey:      item.Resource.ExternalKey,
			Title:            item.Resource.Name,
			URL:              item.Resource.URL,
			MaterializedPath: item.LocationPath,
		}
	}
	return pages, nil
}

func (s *Service) fetchOne(ctx context.Context, workspaceItem workspace.Workspace, config Config, pat string, page wiki.Page) wiki.FetchResult {
	result := wiki.FetchResult{PageID: page.PageID, Title: page.Title, Path: page.MaterializedPath, Status: "failed"}
	wikiRoot, err := cleanWikiRoot(workspaceItem.LocalPath)
	if err != nil {
		result.Error = err
		return result
	}
	if err := s.materializer.EnsureRoot(wikiRoot); err != nil {
		result.Error = err
		return result
	}

	destination := page.MaterializedPath
	newFile := destination == ""
	if newFile {
		filename, err := Filename(page.Title, page.PageID)
		if err != nil {
			result.Error = err
			return result
		}
		destination = filepath.Join(wikiRoot, filename)
	} else {
		destination, err = validateTrackedPath(wikiRoot, destination)
		if err != nil {
			result.Error = err
			return result
		}
	}
	result.Path = destination
	inspection, err := s.materializer.Inspect(destination)
	if err != nil {
		result.Error = err
		return result
	}
	if newFile && inspection.Exists {
		result.Error = fmt.Errorf("wiki page destination %q already exists", destination)
		return result
	}
	if inspection.Exists && !inspection.Regular {
		result.Error = fmt.Errorf("wiki page destination %q is not a regular file", destination)
		return result
	}

	content, err := s.client.Fetch(ctx, config, pat, page.PageID)
	if err != nil {
		result.Error = err
		return result
	}
	markdown, err := RenderMarkdown(page.ResourceID, content)
	if err != nil {
		result.Error = err
		return result
	}
	if err := s.materializer.WriteAtomic(destination, markdown); err != nil {
		result.Error = err
		return result
	}
	if err := s.store.SetLocation(ctx, workspaceItem.ID, page.ResourceID, pageLocationType, destination); err != nil {
		result.Error = fmt.Errorf("register materialized wiki page: %w", err)
		if newFile {
			if removeErr := s.materializer.Remove(destination); removeErr != nil {
				result.Error = errors.Join(result.Error, fmt.Errorf("roll back materialized wiki page: %w", removeErr))
			}
		}
		return result
	}
	result.Status = "fetched"
	return result
}

func cleanWikiRoot(workspacePath string) (string, error) {
	root, err := filepath.Abs(filepath.Join(workspacePath, "wiki"))
	if err != nil {
		return "", fmt.Errorf("resolve workspace wiki path: %w", err)
	}
	return filepath.Clean(root), nil
}

func validateTrackedPath(wikiRoot, tracked string) (string, error) {
	if !filepath.IsAbs(tracked) {
		return "", fmt.Errorf("tracked wiki path %q is not absolute", tracked)
	}
	cleaned, err := filepath.Abs(filepath.Clean(tracked))
	if err != nil {
		return "", fmt.Errorf("resolve tracked wiki path %q: %w", tracked, err)
	}
	if filepath.Dir(cleaned) != wikiRoot || cleaned == wikiRoot || strings.TrimSpace(filepath.Base(cleaned)) == "" {
		return "", fmt.Errorf("tracked wiki path %q is outside the workspace wiki directory", tracked)
	}
	return cleaned, nil
}

type secretNotFoundError struct {
	message string
	cause   error
}

func (e secretNotFoundError) Error() string { return e.message }
func (e secretNotFoundError) Unwrap() error { return e.cause }

type fetchFailureError struct {
	count int
}

func (e fetchFailureError) Error() string {
	return fmt.Sprintf("wiki fetch failed for %d page(s)", e.count)
}

func (e fetchFailureError) Unwrap() error { return ErrFetchFailed }

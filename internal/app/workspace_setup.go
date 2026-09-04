package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/ngochc/dev-dash/internal/config"
	confluenceintegration "github.com/ngochc/dev-dash/internal/integration/confluence"
	gitintegration "github.com/ngochc/dev-dash/internal/integration/git"
	githubintegration "github.com/ngochc/dev-dash/internal/integration/github"
	"github.com/ngochc/dev-dash/internal/platform"
	repositorydomain "github.com/ngochc/dev-dash/internal/repository"
	"github.com/ngochc/dev-dash/internal/secret"
	"github.com/ngochc/dev-dash/internal/storage/sqlite"
	"github.com/ngochc/dev-dash/internal/ui/picker"
	"github.com/ngochc/dev-dash/internal/ui/progress"
	"github.com/ngochc/dev-dash/internal/workspace"
)

type workspaceLookup interface {
	Get(context.Context, string) (workspace.Workspace, error)
}

type workspaceSetupConfig interface {
	Namespace(context.Context, string, string) (map[string]string, error)
	SetUser(context.Context, string, string, string) (workspace.Workspace, error)
	UnsetUser(context.Context, string, string) (workspace.Workspace, error)
}

type workspaceSetupGitHub interface {
	Validate(context.Context, githubintegration.Config) error
	DiscoverOwners(context.Context, githubintegration.Config) ([]githubintegration.Owner, error)
}

type workspaceSetupConfluence interface {
	Validate(context.Context, confluenceintegration.Config, string) error
}

type workspaceSetupSecrets interface {
	Get(context.Context, string) (secret.Secret, error)
	Set(context.Context, string, string) error
}

type workspaceSetupRepositories interface {
	Refresh(context.Context, string) (workspace.Workspace, int, error)
	List(context.Context, string) (workspace.Workspace, []repositorydomain.Listed, error)
	CloneKnown(context.Context, string, []string) (workspace.Workspace, []repositorydomain.CloneResult, error)
}

type workspaceSetupDependencies struct {
	workspaces   workspaceLookup
	config       workspaceSetupConfig
	github       workspaceSetupGitHub
	confluence   workspaceSetupConfluence
	secrets      workspaceSetupSecrets
	repositories workspaceSetupRepositories
	picker       picker.Picker
}

func runWorkspaceSetup(ctx context.Context, workspaceIdentifier string, input io.Reader, output, feedback io.Writer) error {
	databasePath, err := config.DatabasePath()
	if err != nil {
		return fmt.Errorf("resolve database path: %w", err)
	}
	db, err := sqlite.Open(ctx, databasePath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	workspaceRepository := sqlite.NewWorkspaceRepository(db)
	configRepository := sqlite.NewWorkspaceConfigRepository(db)
	workspaceService := workspace.NewService(workspaceRepository)
	configService := workspace.NewConfigService(workspaceRepository, configRepository)
	secretService := secret.NewService(sqlite.NewSecretRepository(db))
	githubClient := githubintegration.NewCLIClient()
	repositoryService := githubintegration.NewService(
		workspaceRepository,
		configRepository,
		sqlite.NewRepositoryResourceStore(db),
		githubClient,
		gitintegration.NewCLIInspector(),
		platform.DirectoryManager{},
	)
	return executeWorkspaceSetup(ctx, workspaceIdentifier, output, feedback, workspaceSetupDependencies{
		workspaces:   workspaceService,
		config:       configService,
		github:       githubClient,
		confluence:   confluenceintegration.NewClient(nil),
		secrets:      secretService,
		repositories: repositoryService,
		picker:       picker.New(input, feedback),
	})
}

func executeWorkspaceSetup(ctx context.Context, workspaceIdentifier string, output, feedback io.Writer, dependencies workspaceSetupDependencies) error {
	item, err := dependencies.workspaces.Get(ctx, workspaceIdentifier)
	if err != nil {
		return err
	}
	githubValues, err := dependencies.config.Namespace(ctx, item.ID, "github")
	if err != nil {
		return err
	}
	confluenceValues, err := dependencies.config.Namespace(ctx, item.ID, "confluence")
	if err != nil {
		return err
	}

	fmt.Fprintf(output, "Workspace: %s\nPath: %s\n\n", item.Name, item.LocalPath)
	providers, err := dependencies.picker.PickMany(ctx, "Integrations", []picker.Option{
		{Value: "github", Label: "GitHub      " + githubSetupState(item.Name, githubValues)},
		{Value: "confluence", Label: "Confluence  " + confluenceSetupState(ctx, item.Name, confluenceValues, dependencies.secrets)},
	})
	if errors.Is(err, picker.ErrCancelled) || err == nil && len(providers) == 0 {
		fmt.Fprintln(output, "Workspace setup cancelled.")
		return nil
	}
	if err != nil {
		return err
	}
	selected := make(map[string]bool, len(providers))
	for _, provider := range providers {
		selected[provider] = true
	}

	var githubResult *githubSetupResult
	if selected["github"] {
		result, err := configureGitHub(ctx, item, githubValues, output, feedback, dependencies)
		if err != nil {
			return err
		}
		if result.cancelled {
			fmt.Fprintln(output, "Workspace setup cancelled.")
			return nil
		}
		githubResult = &result
	}
	var confluenceConfig *confluenceintegration.Config
	if selected["confluence"] {
		configured, err := configureConfluence(ctx, item, confluenceValues, output, feedback, dependencies)
		if err != nil {
			return err
		}
		confluenceConfig = &configured
	}
	printWorkspaceSetupSummary(output, item, githubResult, confluenceConfig)
	if githubResult != nil {
		return githubResult.cloneErr
	}
	return nil
}

type githubSetupResult struct {
	config       githubintegration.Config
	cloneResults []repositorydomain.CloneResult
	cloneErr     error
	cancelled    bool
}

func configureGitHub(ctx context.Context, item workspace.Workspace, values map[string]string, output, feedback io.Writer, dependencies workspaceSetupDependencies) (githubSetupResult, error) {
	githubConfig, cancelled, err := selectGitHubHost(ctx, item, values, output, feedback, dependencies)
	if err != nil || cancelled {
		return githubSetupResult{cancelled: cancelled}, err
	}
	if err := progress.Run(feedback, "Checking GitHub authentication", func() error {
		return dependencies.github.Validate(ctx, githubConfig)
	}); err != nil {
		return githubSetupResult{}, workspaceSetupValidationError(item.Name, githubConfig.Host, err)
	}

	organization, cancelled, err := selectGitHubOwner(ctx, item, values, githubConfig, output, feedback, dependencies)
	if err != nil || cancelled {
		return githubSetupResult{cancelled: cancelled}, err
	}
	if _, err := dependencies.config.SetUser(ctx, item.ID, githubintegration.OrganizationKey, organization); err != nil {
		return githubSetupResult{}, err
	}
	githubConfig.Organization = organization

	if err := progress.Run(feedback, "Refreshing repositories", func() error {
		_, _, refreshErr := dependencies.repositories.Refresh(ctx, item.ID)
		return refreshErr
	}); err != nil {
		return githubSetupResult{}, err
	}
	var repositories []repositorydomain.Listed
	err = progress.Run(feedback, "Inspecting repositories", func() error {
		var listErr error
		_, repositories, listErr = dependencies.repositories.List(ctx, item.ID)
		return listErr
	})
	if err != nil {
		return githubSetupResult{}, err
	}

	selected, pickErr := dependencies.picker.PickMany(ctx, "Repositories", repositoryPickerOptions(repositories))
	if pickErr != nil && !errors.Is(pickErr, picker.ErrCancelled) {
		return githubSetupResult{}, pickErr
	}
	if errors.Is(pickErr, picker.ErrCancelled) {
		selected = nil
	}

	var results []repositorydomain.CloneResult
	var cloneErr error
	if len(selected) > 0 {
		fmt.Fprintln(output, "Selected repositories:")
		for _, externalKey := range selected {
			fmt.Fprintf(output, "  %s\n", externalKey)
		}
		confirmed, err := dependencies.picker.Confirm(fmt.Sprintf("Clone %d repositories?", len(selected)), true)
		if err != nil {
			return githubSetupResult{}, err
		}
		if confirmed {
			cloneErr = progress.Run(feedback, "Cloning selected repositories", func() error {
				var err error
				_, results, err = dependencies.repositories.CloneKnown(ctx, item.ID, selected)
				return err
			})
			if err := printCloneResults(output, results); err != nil {
				return githubSetupResult{}, err
			}
		}
	}
	return githubSetupResult{config: githubConfig, cloneResults: results, cloneErr: cloneErr}, nil
}

func configureConfluence(ctx context.Context, item workspace.Workspace, values map[string]string, output, feedback io.Writer, dependencies workspaceSetupDependencies) (confluenceintegration.Config, error) {
	defaultURL := strings.TrimSpace(values["base_url"])
	var baseURL string
	for {
		entered, err := dependencies.picker.Input("Confluence URL:", defaultURL)
		if err != nil {
			return confluenceintegration.Config{}, err
		}
		baseURL, err = confluenceintegration.ResolveBaseURL(entered)
		if err == nil {
			break
		}
		fmt.Fprintln(output, err)
		defaultURL = entered
	}

	space := strings.TrimSpace(values["space"])
	for {
		entered, err := dependencies.picker.Input("Confluence space:", space)
		if err != nil {
			return confluenceintegration.Config{}, err
		}
		space = strings.TrimSpace(entered)
		if space != "" {
			break
		}
		fmt.Fprintln(output, "Confluence space is required.")
	}

	secretName := confluenceSecretName(values["secret"])
	storedSecret, secretErr := dependencies.secrets.Get(ctx, secretName)
	var pat string
	storePAT := false
	if secretErr == nil {
		fmt.Fprintln(output, "PAT: configured")
		keep, err := dependencies.picker.Confirm("Keep existing PAT?", true)
		if err != nil {
			return confluenceintegration.Config{}, err
		}
		if keep {
			pat = storedSecret.Value
		} else {
			pat, err = dependencies.picker.Secret("PAT:")
			if err != nil {
				return confluenceintegration.Config{}, err
			}
			storePAT = true
		}
	} else {
		if !errors.Is(secretErr, secret.ErrNotFound) {
			return confluenceintegration.Config{}, fmt.Errorf("get Confluence secret %q: %w", secretName, secretErr)
		}
		var err error
		pat, err = dependencies.picker.Secret("PAT:")
		if err != nil {
			return confluenceintegration.Config{}, err
		}
		storePAT = true
	}

	existingRoot := strings.TrimSpace(values["root_page"])
	restrict, err := dependencies.picker.Confirm("Restrict wiki discovery to a root page?", existingRoot != "")
	if err != nil {
		return confluenceintegration.Config{}, err
	}
	rootPage := ""
	if restrict {
		rootPage = existingRoot
		for {
			entered, err := dependencies.picker.Input("Root page ID:", rootPage)
			if err != nil {
				return confluenceintegration.Config{}, err
			}
			rootPage = strings.TrimSpace(entered)
			if decimalPageID(rootPage) {
				break
			}
			fmt.Fprintln(output, "Expected a decimal page ID.")
		}
	}

	candidate, err := confluenceintegration.ResolveConfig(item.Name, map[string]string{
		"base_url":  baseURL,
		"space":     space,
		"secret":    "secret:" + secretName,
		"auth_type": confluenceintegration.DefaultAuthType,
		"root_page": rootPage,
	})
	if err != nil {
		return confluenceintegration.Config{}, err
	}
	if err := progress.Run(feedback, "Checking Confluence", func() error {
		return dependencies.confluence.Validate(ctx, candidate, pat)
	}); err != nil {
		return confluenceintegration.Config{}, confluenceSetupValidationError(item.Name, candidate, err)
	}
	fmt.Fprintln(output, "Confluence checks:")
	fmt.Fprintln(output, "  URL: OK")
	fmt.Fprintln(output, "  Auth: OK")
	fmt.Fprintln(output, "  Space: OK")

	if storePAT {
		if err := dependencies.secrets.Set(ctx, secretName, pat); err != nil {
			return confluenceintegration.Config{}, err
		}
	}
	for _, setting := range []struct {
		key   string
		value string
	}{
		{confluenceintegration.BaseURLKey, candidate.BaseURL},
		{confluenceintegration.SpaceKey, candidate.Space},
		{confluenceintegration.SecretKey, "secret:" + secretName},
	} {
		if _, err := dependencies.config.SetUser(ctx, item.ID, setting.key, setting.value); err != nil {
			return confluenceintegration.Config{}, err
		}
	}
	if _, exists := values["auth_type"]; exists {
		if _, err := dependencies.config.UnsetUser(ctx, item.ID, confluenceintegration.AuthTypeKey); err != nil {
			return confluenceintegration.Config{}, err
		}
	}
	if candidate.RootPage != "" {
		if _, err := dependencies.config.SetUser(ctx, item.ID, confluenceintegration.RootPageKey, candidate.RootPage); err != nil {
			return confluenceintegration.Config{}, err
		}
	} else if _, exists := values["root_page"]; exists {
		if _, err := dependencies.config.UnsetUser(ctx, item.ID, confluenceintegration.RootPageKey); err != nil {
			return confluenceintegration.Config{}, err
		}
	}
	printConfluenceSetupSummary(output, candidate)
	return candidate, nil
}

func githubSetupState(workspaceName string, values map[string]string) string {
	if len(values) == 0 {
		return "not configured"
	}
	if _, err := githubintegration.ResolveConfig(workspaceName, values); err == nil {
		return "configured"
	}
	return "incomplete"
}

func confluenceSetupState(ctx context.Context, workspaceName string, values map[string]string, secrets workspaceSetupSecrets) string {
	if len(values) == 0 {
		return "not configured"
	}
	config, err := confluenceintegration.ResolveConfig(workspaceName, values)
	if err != nil {
		return "incomplete"
	}
	if _, err := secrets.Get(ctx, config.SecretName); err != nil {
		return "incomplete"
	}
	return "configured"
}

func confluenceSecretName(reference string) string {
	name, ok := strings.CutPrefix(strings.TrimSpace(reference), "secret:")
	if ok && secret.ValidateKey(name) == nil {
		return name
	}
	return "confluence.pat"
}

func decimalPageID(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func confluenceSetupValidationError(workspaceName string, config confluenceintegration.Config, err error) error {
	switch {
	case errors.Is(err, confluenceintegration.ErrAuthentication), errors.Is(err, confluenceintegration.ErrForbidden):
		return setupError{
			message: fmt.Sprintf("Confluence authentication failed for %s.\n\nCheck the PAT and try again:\n\n  devdash workspace setup %s", config.BaseURL, workspaceName),
			cause:   err,
		}
	case errors.Is(err, confluenceintegration.ErrSpaceNotFound):
		return setupError{message: fmt.Sprintf("Confluence space %q was not found or is not accessible.", config.Space), cause: err}
	case errors.Is(err, confluenceintegration.ErrRootPageNotFound):
		return setupError{message: fmt.Sprintf("Confluence root page %q was not found or is not accessible.", config.RootPage), cause: err}
	default:
		return setupError{message: fmt.Sprintf("Confluence validation failed for %s: %v", config.BaseURL, err), cause: err}
	}
}

func printConfluenceSetupSummary(output io.Writer, config confluenceintegration.Config) {
	root := config.RootPage
	if root == "" {
		root = "all pages"
	}
	fmt.Fprintf(output, "Confluence:\n  URL: %s\n  Space: %s\n  Auth: PAT\n  Secret: configured\n  Root: %s\n  Status: ready\n", config.BaseURL, config.Space, root)
}

func selectGitHubHost(
	ctx context.Context,
	item workspace.Workspace,
	values map[string]string,
	output io.Writer,
	feedback io.Writer,
	dependencies workspaceSetupDependencies,
) (githubintegration.Config, bool, error) {
	storedConfig, storedErr := githubintegration.ResolveHostConfig(values)
	_, hasBaseURL := values["base_url"]
	_, hasOrganization := values["org"]
	if (hasBaseURL || hasOrganization) && storedErr == nil {
		organization := strings.TrimSpace(values["org"])
		if organization == "" {
			organization = "-"
		}
		fmt.Fprintf(output, "Current GitHub configuration:\n  URL: %s\n  Organization: %s\n", storedConfig.BaseURL, organization)
		keep, err := dependencies.picker.Confirm("Use this host?", true)
		if err != nil {
			return githubintegration.Config{}, false, err
		}
		if keep {
			return storedConfig, false, nil
		}
	}
	if storedErr != nil && (hasBaseURL || hasOrganization) {
		fmt.Fprintf(output, "Stored GitHub host is invalid: %v\n", storedErr)
	}

	choice, err := dependencies.picker.PickOne(ctx, "GitHub host", []picker.Option{
		{Value: "github.com", Label: "github.com"},
		{Value: "custom", Label: "Custom GitHub URL"},
	}, "github.com")
	if errors.Is(err, picker.ErrCancelled) {
		return githubintegration.Config{}, true, nil
	}
	if err != nil {
		return githubintegration.Config{}, false, err
	}

	if choice == "github.com" {
		config, err := githubintegration.ResolveHostConfig(nil)
		if err != nil {
			return githubintegration.Config{}, false, err
		}
		if _, err := dependencies.config.SetUser(ctx, item.ID, githubintegration.BaseURLKey, githubintegration.DefaultBaseURL); err != nil {
			return githubintegration.Config{}, false, err
		}
		return config, false, nil
	}

	defaultURL := strings.TrimSpace(values["base_url"])
	for {
		baseURL, err := dependencies.picker.Input("GitHub URL:", defaultURL)
		if err != nil {
			return githubintegration.Config{}, false, err
		}
		config, resolveErr := githubintegration.ResolveHostConfig(map[string]string{"base_url": baseURL})
		if resolveErr != nil {
			fmt.Fprintln(output, resolveErr)
			defaultURL = baseURL
			continue
		}
		if _, err := dependencies.config.SetUser(ctx, item.ID, githubintegration.BaseURLKey, config.BaseURL); err != nil {
			return githubintegration.Config{}, false, err
		}
		return config, false, nil
	}
}

func selectGitHubOwner(
	ctx context.Context,
	item workspace.Workspace,
	values map[string]string,
	config githubintegration.Config,
	output io.Writer,
	feedback io.Writer,
	dependencies workspaceSetupDependencies,
) (string, bool, error) {
	var owners []githubintegration.Owner
	discoveryErr := progress.Run(feedback, "Discovering GitHub owners", func() error {
		var err error
		owners, err = dependencies.github.DiscoverOwners(ctx, config)
		return err
	})
	existingOrganization := strings.TrimSpace(values["org"])
	if discoveryErr != nil || len(owners) == 0 {
		if discoveryErr != nil {
			fmt.Fprintf(output, "Could not discover GitHub owners: %v\n", discoveryErr)
		} else {
			fmt.Fprintln(output, "No GitHub owners discovered.")
		}
		for {
			organization, err := dependencies.picker.Input("GitHub organization:", existingOrganization)
			if err != nil {
				return "", false, err
			}
			organization = strings.TrimSpace(organization)
			if organization != "" {
				return organization, false, nil
			}
			fmt.Fprintln(output, "GitHub organization is required.")
		}
	}

	if existingOrganization != "" && ownerExists(owners, existingOrganization) {
		fmt.Fprintf(output, "Current GitHub organization: %s\n", existingOrganization)
		keep, err := dependencies.picker.Confirm("Use this organization?", true)
		if err != nil {
			return "", false, err
		}
		if keep {
			return existingOrganization, false, nil
		}
	}

	options := make([]picker.Option, len(owners))
	defaultOwner := ""
	for index, owner := range owners {
		label := owner.Login
		if owner.Personal {
			label += " (personal)"
			if defaultOwner == "" {
				defaultOwner = owner.Login
			}
		}
		options[index] = picker.Option{Value: owner.Login, Label: label}
	}
	organization, err := dependencies.picker.PickOne(ctx, "GitHub owner", options, defaultOwner)
	if errors.Is(err, picker.ErrCancelled) {
		return "", true, nil
	}
	if err != nil {
		return "", false, err
	}
	return organization, false, nil
}

func ownerExists(owners []githubintegration.Owner, login string) bool {
	for _, owner := range owners {
		if strings.EqualFold(owner.Login, login) {
			return true
		}
	}
	return false
}

func repositoryPickerOptions(repositories []repositorydomain.Listed) []picker.Option {
	options := make([]picker.Option, len(repositories))
	for index, item := range repositories {
		options[index] = picker.Option{
			Value: item.Repository.ExternalKey,
			Label: fmt.Sprintf("%-10s %s", item.State, item.Repository.ExternalKey),
		}
	}
	return options
}

func printWorkspaceSetupSummary(output io.Writer, item workspace.Workspace, githubResult *githubSetupResult, confluenceConfig *confluenceintegration.Config) {
	fmt.Fprintf(output, "\nWorkspace setup complete.\nWorkspace: %s\n", item.Name)
	if githubResult != nil {
		cloned, existing, failed := 0, 0, 0
		for _, result := range githubResult.cloneResults {
			switch {
			case result.Error != nil:
				failed++
			case result.Status == "cloned" || result.Status == "restored":
				cloned++
			case result.Status == "already cloned":
				existing++
			}
		}
		fmt.Fprintf(output, "GitHub: %s / %s\nRepositories: cloned %d, existing %d, failed %d\n", githubResult.config.Host, githubResult.config.Organization, cloned, existing, failed)
	}
	if confluenceConfig != nil {
		fmt.Fprintf(output, "Confluence: %s / %s\n", confluenceConfig.BaseURL, confluenceConfig.Space)
	}
	fmt.Fprintln(output, "\nNext:")
	if githubResult != nil {
		fmt.Fprintf(output, "  devdash repo list %s\n", item.Name)
	}
	if confluenceConfig != nil {
		fmt.Fprintf(output, "  devdash wiki refresh %s\n", item.Name)
	}
}

func workspaceSetupValidationError(workspaceName, host string, err error) error {
	switch {
	case errors.Is(err, githubintegration.ErrCLIUnavailable):
		return setupError{
			message: fmt.Sprintf("GitHub CLI is required for GitHub setup.\n\nInstall `gh` and run:\n  devdash workspace setup %s", workspaceName),
			cause:   err,
		}
	case errors.Is(err, githubintegration.ErrAuthentication):
		return setupError{
			message: fmt.Sprintf("GitHub CLI is not authenticated for %s.\n\nAuthenticate with:\n  gh auth login --hostname %s\n\nThen run:\n  devdash workspace setup %s", host, host, workspaceName),
			cause:   err,
		}
	default:
		return err
	}
}

type setupError struct {
	message string
	cause   error
}

func (e setupError) Error() string { return e.message }
func (e setupError) Unwrap() error { return e.cause }

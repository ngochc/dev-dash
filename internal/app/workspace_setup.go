package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/ngochc/dev-dash/internal/config"
	gitintegration "github.com/ngochc/dev-dash/internal/integration/git"
	githubintegration "github.com/ngochc/dev-dash/internal/integration/github"
	"github.com/ngochc/dev-dash/internal/platform"
	repositorydomain "github.com/ngochc/dev-dash/internal/repository"
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
}

type workspaceSetupGitHub interface {
	Validate(context.Context, githubintegration.Config) error
	DiscoverOwners(context.Context, githubintegration.Config) ([]githubintegration.Owner, error)
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
		repositories: repositoryService,
		picker:       picker.New(input, feedback),
	})
}

func executeWorkspaceSetup(ctx context.Context, workspaceIdentifier string, output, feedback io.Writer, dependencies workspaceSetupDependencies) error {
	item, err := dependencies.workspaces.Get(ctx, workspaceIdentifier)
	if err != nil {
		return err
	}
	fmt.Fprintf(output, "Workspace: %s\nPath: %s\n\n", item.Name, item.LocalPath)

	values, err := dependencies.config.Namespace(ctx, item.ID, "github")
	if err != nil {
		return err
	}
	githubConfig, cancelled, err := selectGitHubHost(ctx, item, values, output, feedback, dependencies)
	if err != nil {
		return err
	}
	if cancelled {
		fmt.Fprintln(output, "Workspace setup cancelled.")
		return nil
	}
	if err := progress.Run(feedback, "Checking GitHub authentication", func() error {
		return dependencies.github.Validate(ctx, githubConfig)
	}); err != nil {
		return workspaceSetupValidationError(item.Name, githubConfig.Host, err)
	}

	organization, cancelled, err := selectGitHubOwner(ctx, item, values, githubConfig, output, feedback, dependencies)
	if err != nil {
		return err
	}
	if cancelled {
		fmt.Fprintln(output, "Workspace setup cancelled.")
		return nil
	}
	if _, err := dependencies.config.SetUser(ctx, item.ID, githubintegration.OrganizationKey, organization); err != nil {
		return err
	}
	githubConfig.Organization = organization

	if err := progress.Run(feedback, "Refreshing repositories", func() error {
		_, _, refreshErr := dependencies.repositories.Refresh(ctx, item.ID)
		return refreshErr
	}); err != nil {
		return err
	}
	var repositories []repositorydomain.Listed
	err = progress.Run(feedback, "Inspecting repositories", func() error {
		var listErr error
		_, repositories, listErr = dependencies.repositories.List(ctx, item.ID)
		return listErr
	})
	if err != nil {
		return err
	}

	selected, pickErr := dependencies.picker.PickMany(ctx, "Repositories", repositoryPickerOptions(repositories))
	if pickErr != nil && !errors.Is(pickErr, picker.ErrCancelled) {
		return pickErr
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
			return err
		}
		if confirmed {
			cloneErr = progress.Run(feedback, "Cloning selected repositories", func() error {
				var err error
				_, results, err = dependencies.repositories.CloneKnown(ctx, item.ID, selected)
				return err
			})
			if err := printCloneResults(output, results); err != nil {
				return err
			}
		}
	}

	printWorkspaceSetupSummary(output, item, githubConfig, results)
	return cloneErr
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

func printWorkspaceSetupSummary(output io.Writer, item workspace.Workspace, config githubintegration.Config, results []repositorydomain.CloneResult) {
	cloned, existing, failed := 0, 0, 0
	for _, result := range results {
		switch {
		case result.Error != nil:
			failed++
		case result.Status == "cloned" || result.Status == "restored":
			cloned++
		case result.Status == "already cloned":
			existing++
		}
	}
	fmt.Fprintf(output, "\nWorkspace setup complete.\nWorkspace: %s\nGitHub: %s / %s\nRepositories: cloned %d, existing %d, failed %d\n\nNext:\n  devdash repo list %s\n", item.Name, config.Host, config.Organization, cloned, existing, failed, item.Name)
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

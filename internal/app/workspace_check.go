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
	"github.com/ngochc/dev-dash/internal/ui/progress"
	"github.com/ngochc/dev-dash/internal/wiki"
	"github.com/ngochc/dev-dash/internal/workspace"
)

var (
	ErrWorkspaceIncomplete = errors.New("workspace setup incomplete")
	ErrWorkspaceDegraded   = errors.New("workspace readiness degraded")
)

type workspaceCheckStatus string

const (
	workspaceReady         workspaceCheckStatus = "ready"
	workspaceIncomplete    workspaceCheckStatus = "incomplete"
	workspaceDegraded      workspaceCheckStatus = "degraded"
	workspaceNotConfigured workspaceCheckStatus = "not configured"
)

type workspaceCheckConfig interface {
	Namespace(context.Context, string, string) (map[string]string, error)
}

type workspaceReadinessChecker interface {
	Check(context.Context, string) (workspace.Workspace, githubintegration.Config, error)
}

type confluenceReadinessChecker interface {
	Check(context.Context, string) (workspace.Workspace, confluenceintegration.Config, error)
}

type workspaceRepositoryLister interface {
	List(context.Context, string) (workspace.Workspace, []repositorydomain.Listed, error)
}

type workspaceWikiLister interface {
	List(context.Context, string) (workspace.Workspace, []wiki.Listed, error)
}

type workspaceDirectoryChecker interface {
	Exists(string) (bool, error)
}

type workspaceCheckDependencies struct {
	workspaces   workspaceLookup
	config       workspaceCheckConfig
	github       workspaceReadinessChecker
	confluence   confluenceReadinessChecker
	repositories workspaceRepositoryLister
	wiki         workspaceWikiLister
	directories  workspaceDirectoryChecker
}

type workspaceCheckReport struct {
	workspace workspace.Workspace
	root      string

	githubActive   bool
	githubStatus   workspaceCheckStatus
	baseURL        string
	organization   string
	cli            string
	authentication string

	confluenceActive          bool
	confluenceStatus          workspaceCheckStatus
	confluenceBaseURL         string
	confluenceSpace           string
	confluenceSecretReference string
	confluenceRoot            string
	confluenceAuth            string

	repositoryError string
	discovered      int
	cloned          int
	missing         int
	invalid         int
	notCloned       int

	wikiError      string
	wikiDiscovered int
	wikiFetched    int
	wikiMissing    int
	wikiNotFetched int

	status workspaceCheckStatus
}

func runWorkspaceCheck(ctx context.Context, workspaceIdentifier string, output, feedback io.Writer) error {
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
	githubClient := githubintegration.NewCLIClient()
	repositoryService := githubintegration.NewService(
		workspaceRepository,
		configRepository,
		sqlite.NewRepositoryResourceStore(db),
		githubClient,
		gitintegration.NewCLIInspector(),
		platform.DirectoryManager{},
	)
	confluenceService := confluenceintegration.NewService(
		workspaceRepository,
		configRepository,
		sqlite.NewResourceSyncStore(db),
		secret.NewService(sqlite.NewSecretRepository(db)),
		confluenceintegration.NewClient(nil),
		platform.PageMaterializer{},
	)
	return executeWorkspaceCheck(ctx, workspaceIdentifier, output, feedback, workspaceCheckDependencies{
		workspaces:   workspace.NewService(workspaceRepository),
		config:       workspace.NewConfigService(workspaceRepository, configRepository),
		github:       repositoryService,
		confluence:   confluenceService,
		repositories: repositoryService,
		wiki:         confluenceService,
		directories:  platform.DirectoryManager{},
	})
}

func executeWorkspaceCheck(ctx context.Context, workspaceIdentifier string, output, feedback io.Writer, dependencies workspaceCheckDependencies) error {
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

	report := workspaceCheckReport{workspace: item, root: "present", status: workspaceReady}
	var degradationErrors []error
	incomplete := false

	exists, pathErr := dependencies.directories.Exists(item.LocalPath)
	switch {
	case pathErr != nil:
		report.root = "ERROR: " + pathErr.Error()
		degradationErrors = append(degradationErrors, pathErr)
	case !exists:
		report.root = "MISSING"
		degradationErrors = append(degradationErrors, errors.New("workspace root is missing or is not a directory"))
	}

	if len(githubValues) == 0 {
		report.githubStatus = workspaceNotConfigured
	} else {
		report.githubActive = true
		report.githubStatus = workspaceReady
		report.cli = "not checked"
		report.authentication = "not checked"
		hostConfig, hostErr := githubintegration.ResolveHostConfig(githubValues)
		storedBaseURL := strings.TrimSpace(githubValues["base_url"])
		switch {
		case hostErr != nil:
			report.baseURL = fmt.Sprintf("%q (INVALID)", storedBaseURL)
			report.githubStatus = workspaceDegraded
			degradationErrors = append(degradationErrors, hostErr)
		case storedBaseURL == "":
			report.baseURL = hostConfig.BaseURL + " (default)"
		default:
			report.baseURL = hostConfig.BaseURL + " (configured)"
		}

		organization := strings.TrimSpace(githubValues["org"])
		if organization == "" {
			report.organization = "MISSING"
			report.githubStatus = workspaceIncomplete
			incomplete = true
		} else {
			report.organization = organization + " (configured)"
		}
		if report.githubStatus != workspaceIncomplete && hostErr == nil {
			checkErr := progress.Run(feedback, "Checking GitHub authentication", func() error {
				_, _, err := dependencies.github.Check(ctx, item.ID)
				return err
			})
			switch {
			case checkErr == nil:
				report.cli = "installed"
				report.authentication = "authenticated"
			case errors.Is(checkErr, githubintegration.ErrCLIUnavailable):
				report.cli = "missing"
				report.githubStatus = workspaceDegraded
				degradationErrors = append(degradationErrors, checkErr)
			case errors.Is(checkErr, githubintegration.ErrAuthentication):
				report.cli = "installed"
				report.authentication = "failed"
				report.githubStatus = workspaceDegraded
				degradationErrors = append(degradationErrors, checkErr)
			default:
				report.cli = "error"
				report.authentication = "failed"
				report.githubStatus = workspaceDegraded
				degradationErrors = append(degradationErrors, checkErr)
			}
		}
	}

	if len(confluenceValues) == 0 {
		report.confluenceStatus = workspaceNotConfigured
	} else {
		report.confluenceActive = true
		report.confluenceStatus = workspaceReady
		populateConfluenceReport(&report, confluenceValues)
		resolved, resolveErr := confluenceintegration.ResolveConfig(item.Name, confluenceValues)
		switch {
		case errors.Is(resolveErr, confluenceintegration.ErrIncompleteConfig):
			report.confluenceStatus = workspaceIncomplete
			incomplete = true
		case resolveErr != nil:
			report.confluenceStatus = workspaceDegraded
			degradationErrors = append(degradationErrors, resolveErr)
		default:
			report.confluenceBaseURL = resolved.BaseURL
			report.confluenceSpace = resolved.Space
			report.confluenceSecretReference = resolved.SecretReference
			report.confluenceRoot = resolved.RootPage
			checkErr := progress.Run(feedback, "Checking Confluence", func() error {
				_, _, err := dependencies.confluence.Check(ctx, item.ID)
				return err
			})
			if checkErr != nil {
				report.confluenceStatus = workspaceDegraded
				report.confluenceAuth = "PAT failed"
				degradationErrors = append(degradationErrors, checkErr)
			} else {
				report.confluenceAuth = "PAT OK"
			}
		}
	}

	var repositories []repositorydomain.Listed
	listErr := progress.Run(feedback, "Inspecting repositories", func() error {
		var err error
		_, repositories, err = dependencies.repositories.List(ctx, item.ID)
		return err
	})
	if listErr != nil {
		report.repositoryError = listErr.Error()
		degradationErrors = append(degradationErrors, listErr)
	} else {
		report.discovered = len(repositories)
		for _, repository := range repositories {
			switch repository.State {
			case repositorydomain.StateCloned:
				report.cloned++
			case repositorydomain.StateMissing:
				report.missing++
			case repositorydomain.StateInvalid:
				report.invalid++
			case repositorydomain.StateNotCloned:
				report.notCloned++
			}
		}
	}

	var wikiPages []wiki.Listed
	wikiErr := progress.Run(feedback, "Inspecting wiki pages", func() error {
		var err error
		_, wikiPages, err = dependencies.wiki.List(ctx, item.ID)
		return err
	})
	if wikiErr != nil {
		report.wikiError = wikiErr.Error()
		degradationErrors = append(degradationErrors, wikiErr)
	} else {
		report.wikiDiscovered = len(wikiPages)
		for _, page := range wikiPages {
			switch page.State {
			case wiki.StateFetched:
				report.wikiFetched++
			case wiki.StateMissing:
				report.wikiMissing++
			case wiki.StateNotFetched:
				report.wikiNotFetched++
			}
		}
	}

	switch {
	case incomplete:
		report.status = workspaceIncomplete
	case len(degradationErrors) != 0:
		report.status = workspaceDegraded
	default:
		report.status = workspaceReady
	}
	printWorkspaceCheckReport(output, report)

	if report.status == workspaceIncomplete {
		return workspaceStatusError{message: fmt.Sprintf("workspace %q setup is incomplete", item.Name), cause: ErrWorkspaceIncomplete}
	}
	if report.status == workspaceDegraded {
		causes := append([]error{ErrWorkspaceDegraded}, degradationErrors...)
		return workspaceStatusError{
			message: fmt.Sprintf("workspace %q readiness is degraded", item.Name),
			cause:   errors.Join(causes...),
		}
	}
	return nil
}

func populateConfluenceReport(report *workspaceCheckReport, values map[string]string) {
	baseURL := strings.TrimSpace(values["base_url"])
	if normalized, err := confluenceintegration.ResolveBaseURL(baseURL); err == nil {
		report.confluenceBaseURL = normalized
	} else if baseURL == "" {
		report.confluenceBaseURL = "MISSING"
	} else {
		report.confluenceBaseURL = fmt.Sprintf("%q (INVALID)", baseURL)
	}
	space := strings.TrimSpace(values["space"])
	if space == "" {
		report.confluenceSpace = "MISSING"
	} else {
		report.confluenceSpace = space
	}
	reference := strings.TrimSpace(values["secret"])
	name, ok := strings.CutPrefix(reference, "secret:")
	if reference == "" {
		report.confluenceSecretReference = "MISSING"
	} else if ok && secret.ValidateKey(name) == nil {
		report.confluenceSecretReference = reference
	} else {
		report.confluenceSecretReference = "INVALID"
	}
	report.confluenceRoot = strings.TrimSpace(values["root_page"])
	report.confluenceAuth = "not checked"
}

func printWorkspaceCheckReport(output io.Writer, report workspaceCheckReport) {
	fmt.Fprintf(output, "Workspace\n  Name: %s\n  Path: %s\n  Root: %s\n\n", report.workspace.Name, report.workspace.LocalPath, report.root)
	fmt.Fprintln(output, "GitHub")
	if !report.githubActive {
		fmt.Fprintf(output, "  status: %s\n\n", report.githubStatus)
	} else {
		fmt.Fprintf(output, "  github.base_url: %s\n  github.org: %s\n  gh: %s\n  Authentication: %s\n  status: %s\n\n", report.baseURL, report.organization, report.cli, report.authentication, report.githubStatus)
	}
	fmt.Fprintln(output, "Confluence")
	if !report.confluenceActive {
		fmt.Fprintf(output, "  status: %s\n\n", report.confluenceStatus)
	} else {
		fmt.Fprintf(output, "  confluence.base_url: %s\n  confluence.space: %s\n  confluence.secret: %s\n", report.confluenceBaseURL, report.confluenceSpace, report.confluenceSecretReference)
		if report.confluenceRoot != "" {
			fmt.Fprintf(output, "  confluence.root_page: %s\n", report.confluenceRoot)
		}
		fmt.Fprintf(output, "  auth: %s\n  status: %s\n\n", report.confluenceAuth, report.confluenceStatus)
	}
	fmt.Fprintln(output, "Repositories")
	if report.repositoryError != "" {
		fmt.Fprintf(output, "  Inspection: failed: %s\n", report.repositoryError)
	}
	fmt.Fprintf(output, "  Discovered: %d\n  Cloned: %d\n  Missing: %d\n  Invalid: %d\n  Not cloned: %d\n\n", report.discovered, report.cloned, report.missing, report.invalid, report.notCloned)
	fmt.Fprintln(output, "Wiki pages")
	if report.wikiError != "" {
		fmt.Fprintf(output, "  Inspection: failed: %s\n", report.wikiError)
	}
	fmt.Fprintf(output, "  Discovered: %d\n  Fetched: %d\n  Missing: %d\n  Not fetched: %d\n\n", report.wikiDiscovered, report.wikiFetched, report.wikiMissing, report.wikiNotFetched)
	fmt.Fprintf(output, "Status\n  Status: %s\n", report.status)
	if report.status != workspaceReady {
		fmt.Fprintf(output, "\nConfigure with:\n  devdash workspace setup %s\n", report.workspace.Name)
	}
}

type workspaceStatusError struct {
	message string
	cause   error
}

func (e workspaceStatusError) Error() string { return e.message }
func (e workspaceStatusError) Unwrap() error { return e.cause }

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
	"github.com/ngochc/dev-dash/internal/ui/progress"
	"github.com/ngochc/dev-dash/internal/workspace"
)

var (
	ErrWorkspaceIncomplete = errors.New("workspace setup incomplete")
	ErrWorkspaceDegraded   = errors.New("workspace readiness degraded")
)

type workspaceCheckStatus string

const (
	workspaceReady      workspaceCheckStatus = "ready"
	workspaceIncomplete workspaceCheckStatus = "incomplete"
	workspaceDegraded   workspaceCheckStatus = "degraded"
)

type workspaceCheckConfig interface {
	Namespace(context.Context, string, string) (map[string]string, error)
}

type workspaceReadinessChecker interface {
	Check(context.Context, string) (workspace.Workspace, githubintegration.Config, error)
}

type workspaceRepositoryLister interface {
	List(context.Context, string) (workspace.Workspace, []repositorydomain.Listed, error)
}

type workspaceDirectoryChecker interface {
	Exists(string) (bool, error)
}

type workspaceCheckDependencies struct {
	workspaces   workspaceLookup
	config       workspaceCheckConfig
	github       workspaceReadinessChecker
	repositories workspaceRepositoryLister
	directories  workspaceDirectoryChecker
}

type workspaceCheckReport struct {
	workspace       workspace.Workspace
	root            string
	baseURL         string
	organization    string
	cli             string
	authentication  string
	repositoryError string
	discovered      int
	cloned          int
	missing         int
	invalid         int
	notCloned       int
	status          workspaceCheckStatus
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
	client := githubintegration.NewCLIClient()
	repositoryService := githubintegration.NewService(
		workspaceRepository,
		configRepository,
		sqlite.NewRepositoryResourceStore(db),
		client,
		gitintegration.NewCLIInspector(),
		platform.DirectoryManager{},
	)
	return executeWorkspaceCheck(ctx, workspaceIdentifier, output, feedback, workspaceCheckDependencies{
		workspaces:   workspace.NewService(workspaceRepository),
		config:       workspace.NewConfigService(workspaceRepository, configRepository),
		github:       repositoryService,
		repositories: repositoryService,
		directories:  platform.DirectoryManager{},
	})
}

func executeWorkspaceCheck(ctx context.Context, workspaceIdentifier string, output, feedback io.Writer, dependencies workspaceCheckDependencies) error {
	item, err := dependencies.workspaces.Get(ctx, workspaceIdentifier)
	if err != nil {
		return err
	}
	values, err := dependencies.config.Namespace(ctx, item.ID, "github")
	if err != nil {
		return err
	}

	report := workspaceCheckReport{
		workspace:      item,
		root:           "present",
		cli:            "not checked",
		authentication: "not checked",
		status:         workspaceReady,
	}
	var degradationErrors []error

	exists, pathErr := dependencies.directories.Exists(item.LocalPath)
	switch {
	case pathErr != nil:
		report.root = "ERROR: " + pathErr.Error()
		degradationErrors = append(degradationErrors, pathErr)
	case !exists:
		report.root = "MISSING"
		degradationErrors = append(degradationErrors, errors.New("workspace root is missing or is not a directory"))
	}

	hostConfig, hostErr := githubintegration.ResolveHostConfig(values)
	storedBaseURL := strings.TrimSpace(values["base_url"])
	switch {
	case hostErr != nil:
		report.baseURL = fmt.Sprintf("%q (INVALID)", storedBaseURL)
		degradationErrors = append(degradationErrors, hostErr)
	case storedBaseURL == "":
		report.baseURL = hostConfig.BaseURL + " (default)"
	default:
		report.baseURL = hostConfig.BaseURL + " (configured)"
	}

	organization := strings.TrimSpace(values["org"])
	missingOrganization := organization == ""
	if missingOrganization {
		report.organization = "MISSING"
		report.status = workspaceIncomplete
	} else {
		report.organization = organization + " (configured)"
	}

	if !missingOrganization && hostErr == nil {
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
			degradationErrors = append(degradationErrors, checkErr)
		case errors.Is(checkErr, githubintegration.ErrAuthentication):
			report.cli = "installed"
			report.authentication = "failed"
			degradationErrors = append(degradationErrors, checkErr)
		default:
			report.cli = "error"
			report.authentication = "failed"
			degradationErrors = append(degradationErrors, checkErr)
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

	if report.status != workspaceIncomplete && len(degradationErrors) > 0 {
		report.status = workspaceDegraded
	}
	printWorkspaceCheckReport(output, report)

	if report.status == workspaceIncomplete {
		return workspaceStatusError{
			message: fmt.Sprintf("workspace %q setup is incomplete", item.Name),
			cause:   ErrWorkspaceIncomplete,
		}
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

func printWorkspaceCheckReport(output io.Writer, report workspaceCheckReport) {
	fmt.Fprintf(output, "Workspace\n  Name: %s\n  Path: %s\n  Root: %s\n\n", report.workspace.Name, report.workspace.LocalPath, report.root)
	fmt.Fprintf(output, "GitHub\n  github.base_url: %s\n  github.org: %s\n  gh: %s\n  Authentication: %s\n\n", report.baseURL, report.organization, report.cli, report.authentication)
	fmt.Fprintln(output, "Repositories")
	if report.repositoryError != "" {
		fmt.Fprintf(output, "  Inspection: failed: %s\n", report.repositoryError)
	}
	fmt.Fprintf(output, "  Discovered: %d\n  Cloned: %d\n  Missing: %d\n  Invalid: %d\n  Not cloned: %d\n\n", report.discovered, report.cloned, report.missing, report.invalid, report.notCloned)
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

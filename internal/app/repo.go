package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/ngochc/dev-dash/internal/config"
	gitintegration "github.com/ngochc/dev-dash/internal/integration/git"
	githubintegration "github.com/ngochc/dev-dash/internal/integration/github"
	"github.com/ngochc/dev-dash/internal/platform"
	repositorydomain "github.com/ngochc/dev-dash/internal/repository"
	"github.com/ngochc/dev-dash/internal/storage/sqlite"
	"github.com/ngochc/dev-dash/internal/workspace"
)

type repositoryService interface {
	Refresh(context.Context, string) (workspace.Workspace, int, error)
	List(context.Context, string) (workspace.Workspace, []repositorydomain.Listed, error)
	Clone(context.Context, string, []string, bool) (workspace.Workspace, []repositorydomain.CloneResult, error)
}

func runRepo(ctx context.Context, args []string, output io.Writer) error {
	if err := validateRepoArgs(args); err != nil {
		return err
	}
	dbPath, err := config.DatabasePath()
	if err != nil {
		return fmt.Errorf("resolve database path: %w", err)
	}
	db, err := sqlite.Open(ctx, dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	service := githubintegration.NewService(
		sqlite.NewWorkspaceRepository(db),
		sqlite.NewWorkspaceConfigRepository(db),
		sqlite.NewRepositoryResourceStore(db),
		githubintegration.NewCLIClient(),
		gitintegration.NewCLIInspector(),
		platform.DirectoryManager{},
	)
	return executeRepo(ctx, args, output, service)
}

func executeRepo(ctx context.Context, args []string, output io.Writer, service repositoryService) error {
	switch args[0] {
	case "refresh":
		_, count, err := service.Refresh(ctx, args[1])
		if err != nil {
			return err
		}
		fmt.Fprintf(output, "Repositories refreshed: %d\n", count)
		return nil

	case "list":
		_, items, err := service.List(ctx, args[1])
		if err != nil {
			return err
		}
		if len(items) == 0 {
			fmt.Fprintln(output, "No repositories found.")
			return nil
		}
		writer := tabwriter.NewWriter(output, 0, 4, 3, ' ', 0)
		fmt.Fprintln(writer, "REPOSITORY\tSTATUS\tPATH")
		for _, item := range items {
			path := item.Repository.CheckoutPath
			if path == "" {
				path = "-"
			}
			fmt.Fprintf(writer, "%s\t%s\t%s\n", item.Repository.ExternalKey, item.State, path)
		}
		if err := writer.Flush(); err != nil {
			return fmt.Errorf("flush repository list: %w", err)
		}
		return nil

	case "clone":
		all := args[2] == "--all"
		selectors := args[2:]
		if all {
			selectors = nil
		}
		_, results, cloneErr := service.Clone(ctx, args[1], selectors, all)
		writer := tabwriter.NewWriter(output, 0, 4, 3, ' ', 0)
		for _, result := range results {
			status := result.Status
			if result.Error != nil {
				status = "failed: " + result.Error.Error()
			}
			fmt.Fprintf(writer, "%s\t%s\n", result.Repository, status)
		}
		if err := writer.Flush(); err != nil {
			return fmt.Errorf("flush repository clone results: %w", err)
		}
		return cloneErr
	}
	panic("validated repo command was not handled")
}

func validateRepoArgs(args []string) error {
	if len(args) == 0 {
		return errors.New("repo command required: refresh, list, or clone")
	}
	switch args[0] {
	case "refresh", "list":
		if len(args) != 2 {
			return fmt.Errorf("usage: devdash repo %s <workspace>", args[0])
		}
	case "clone":
		if len(args) < 3 {
			return errors.New("usage: devdash repo clone <workspace> --all|<repo> [<repo>...]")
		}
		for _, selector := range args[2:] {
			if selector == "--all" && len(args) != 3 {
				return errors.New("usage: devdash repo clone <workspace> --all|<repo> [<repo>...]")
			}
		}
	default:
		return fmt.Errorf("unknown repo command: %s", args[0])
	}
	return nil
}

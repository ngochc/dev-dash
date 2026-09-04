package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/ngochc/dev-dash/internal/config"
	confluenceintegration "github.com/ngochc/dev-dash/internal/integration/confluence"
	"github.com/ngochc/dev-dash/internal/platform"
	"github.com/ngochc/dev-dash/internal/secret"
	"github.com/ngochc/dev-dash/internal/storage/sqlite"
	"github.com/ngochc/dev-dash/internal/ui/progress"
	"github.com/ngochc/dev-dash/internal/wiki"
	"github.com/ngochc/dev-dash/internal/workspace"
)

type wikiService interface {
	Refresh(context.Context, string) (workspace.Workspace, int, error)
	List(context.Context, string) (workspace.Workspace, []wiki.Listed, error)
	FetchSelected(context.Context, string, []string) (workspace.Workspace, []wiki.FetchResult, error)
	FetchAll(context.Context, string) (workspace.Workspace, []wiki.FetchResult, error)
}

func runWiki(ctx context.Context, args []string, output, feedback io.Writer) error {
	if err := validateWikiArgs(args); err != nil {
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

	service := confluenceintegration.NewService(
		sqlite.NewWorkspaceRepository(db),
		sqlite.NewWorkspaceConfigRepository(db),
		sqlite.NewResourceSyncStore(db),
		secret.NewService(sqlite.NewSecretRepository(db)),
		confluenceintegration.NewClient(nil),
		platform.PageMaterializer{},
	)
	return executeWiki(ctx, args, output, feedback, service)
}

func executeWiki(ctx context.Context, args []string, output, feedback io.Writer, service wikiService) error {
	switch args[0] {
	case "refresh":
		var count int
		err := progress.Run(feedback, "Refreshing wiki pages", func() error {
			var refreshErr error
			_, count, refreshErr = service.Refresh(ctx, args[1])
			return refreshErr
		})
		if err != nil {
			return err
		}
		fmt.Fprintf(output, "Wiki pages refreshed: %d\n", count)
		return nil

	case "list":
		var items []wiki.Listed
		err := progress.Run(feedback, "Inspecting wiki pages", func() error {
			var listErr error
			_, items, listErr = service.List(ctx, args[1])
			return listErr
		})
		if err != nil {
			return err
		}
		if len(items) == 0 {
			fmt.Fprintln(output, "No wiki pages found.")
			return nil
		}
		writer := tabwriter.NewWriter(output, 0, 4, 3, ' ', 0)
		fmt.Fprintln(writer, "PAGE ID\tSTATUS\tTITLE\tPATH")
		for _, item := range items {
			path := item.Page.MaterializedPath
			if path == "" {
				path = "-"
			}
			fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", item.Page.PageID, item.State, item.Page.Title, path)
		}
		if err := writer.Flush(); err != nil {
			return fmt.Errorf("flush wiki list: %w", err)
		}
		return nil

	case "fetch":
		all := args[2] == "--all"
		selectors := args[2:]
		var results []wiki.FetchResult
		fetchErr := progress.Run(feedback, "Refreshing and fetching wiki pages", func() error {
			var err error
			if all {
				_, results, err = service.FetchAll(ctx, args[1])
			} else {
				_, results, err = service.FetchSelected(ctx, args[1], selectors)
			}
			return err
		})
		if err := printWikiFetchResults(output, results); err != nil {
			return err
		}
		return fetchErr
	}
	panic("validated wiki command was not handled")
}

func printWikiFetchResults(output io.Writer, results []wiki.FetchResult) error {
	writer := tabwriter.NewWriter(output, 0, 4, 3, ' ', 0)
	fmt.Fprintln(writer, "PAGE ID\tSTATUS\tPATH")
	for _, result := range results {
		status := result.Status
		if result.Error != nil {
			status = "failed: " + result.Error.Error()
		}
		path := result.Path
		if path == "" {
			path = "-"
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\n", result.PageID, status, path)
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush wiki fetch results: %w", err)
	}
	return nil
}

func validateWikiArgs(args []string) error {
	if len(args) == 0 {
		return errors.New("wiki command required: refresh, list, or fetch")
	}
	switch args[0] {
	case "refresh", "list":
		if len(args) != 2 {
			return fmt.Errorf("usage: devdash wiki %s <workspace>", args[0])
		}
	case "fetch":
		if len(args) < 3 {
			return errors.New("usage: devdash wiki fetch <workspace> --all|<page> [<page>...]")
		}
		for _, selector := range args[2:] {
			if selector == "--all" && len(args) != 3 {
				return errors.New("usage: devdash wiki fetch <workspace> --all|<page> [<page>...]")
			}
		}
	default:
		return fmt.Errorf("unknown wiki command: %s", args[0])
	}
	return nil
}

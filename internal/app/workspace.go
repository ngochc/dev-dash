package app

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/ngochc/dev-dash/internal/config"
	"github.com/ngochc/dev-dash/internal/storage/sqlite"
	"github.com/ngochc/dev-dash/internal/workspace"
)

func runWorkspace(ctx context.Context, args []string, input io.Reader, output io.Writer) error {
	if len(args) > 0 && args[0] == "resource" {
		return runWorkspaceResource(ctx, args[1:], output)
	}
	if len(args) > 0 && args[0] == "config" {
		return runWorkspaceConfig(ctx, args[1:], input, output)
	}

	if err := validateWorkspaceArgs(args); err != nil {
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

	service := workspace.NewService(sqlite.NewWorkspaceRepository(db))

	switch args[0] {
	case "list":
		items, err := service.List(ctx)
		if err != nil {
			return err
		}
		printWorkspaceList(output, items)
		return nil

	case "add":
		path := ""
		if len(args) == 3 {
			path = args[2]
		}
		item, err := service.Add(ctx, args[1], path)
		if err != nil {
			return err
		}
		fmt.Fprintf(output, "Workspace added: %s\n", item.Name)
		return nil

	case "show":
		item, err := service.Get(ctx, args[1])
		if err != nil {
			return err
		}
		fmt.Fprintf(output, "ID:   %s\nName: %s\nPath: %s\n", item.ID, item.Name, item.LocalPath)
		return nil

	case "remove":
		item, err := service.Remove(ctx, args[1])
		if err != nil {
			return err
		}
		fmt.Fprintf(output, "Workspace removed: %s\n", item.Name)
		return nil
	}

	panic("validated workspace command was not handled")
}

func validateWorkspaceArgs(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("workspace command required: list, add, show, or remove")
	}

	switch args[0] {
	case "list":
		if len(args) != 1 {
			return fmt.Errorf("usage: devdash workspace list")
		}
	case "add":
		if len(args) < 2 || len(args) > 3 {
			return fmt.Errorf("usage: devdash workspace add <name> [path]")
		}
	case "show":
		if len(args) != 2 {
			return fmt.Errorf("usage: devdash workspace show <name-or-id>")
		}
	case "remove":
		if len(args) != 2 {
			return fmt.Errorf("usage: devdash workspace remove <name-or-id>")
		}
	default:
		return fmt.Errorf("unknown workspace command: %s", args[0])
	}
	return nil
}

func printWorkspaceList(output io.Writer, items []workspace.Workspace) {
	if len(items) == 0 {
		fmt.Fprintln(output, "No workspaces found.")
		return
	}

	writer := tabwriter.NewWriter(output, 0, 4, 3, ' ', 0)
	fmt.Fprintln(writer, "NAME\tPATH")
	for _, item := range items {
		fmt.Fprintf(writer, "%s\t%s\n", item.Name, item.LocalPath)
	}
	writer.Flush()
}

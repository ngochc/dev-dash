package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/ngochc/dev-dash/internal/config"
	"github.com/ngochc/dev-dash/internal/storage/sqlite"
	"github.com/ngochc/dev-dash/internal/workspace"
)

func runWorkspaceResource(ctx context.Context, args []string, output io.Writer) error {
	if err := validateWorkspaceResourceArgs(args); err != nil {
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

	service := workspace.NewMembershipService(
		sqlite.NewWorkspaceRepository(db),
		sqlite.NewResourceRepository(db),
		sqlite.NewWorkspaceResourceRepository(db),
	)
	switch args[0] {
	case "list":
		workspaceItem, items, err := service.List(ctx, args[1])
		if err != nil {
			return err
		}
		printWorkspaceResourceList(output, workspaceItem, items)
		return nil
	case "add":
		role := ""
		if len(args) == 4 {
			role = args[3]
		}
		workspaceItem, err := service.Add(ctx, args[1], args[2], role)
		if err != nil {
			return err
		}
		fmt.Fprintf(output, "Resource added to workspace %s: %s\n", workspaceItem.Name, args[2])
		return nil
	case "remove":
		workspaceItem, err := service.Remove(ctx, args[1], args[2])
		if err != nil {
			return err
		}
		fmt.Fprintf(output, "Resource removed from workspace %s: %s\n", workspaceItem.Name, args[2])
		return nil
	}
	panic("validated workspace resource command was not handled")
}

func validateWorkspaceResourceArgs(args []string) error {
	if len(args) == 0 {
		return errors.New("workspace resource command required: add, list, or remove")
	}
	switch args[0] {
	case "list":
		if len(args) != 2 {
			return errors.New("usage: devdash workspace resource list <workspace-name-or-id>")
		}
	case "add":
		if len(args) < 3 || len(args) > 4 {
			return errors.New("usage: devdash workspace resource add <workspace-name-or-id> <resource-id> [role]")
		}
	case "remove":
		if len(args) != 3 {
			return errors.New("usage: devdash workspace resource remove <workspace-name-or-id> <resource-id>")
		}
	default:
		return fmt.Errorf("unknown workspace resource command: %s", args[0])
	}
	return nil
}

func printWorkspaceResourceList(output io.Writer, workspaceItem workspace.Workspace, items []workspace.ResourceMembership) {
	if len(items) == 0 {
		fmt.Fprintf(output, "No resources found in workspace: %s\n", workspaceItem.Name)
		return
	}
	writer := tabwriter.NewWriter(output, 0, 4, 3, ' ', 0)
	fmt.Fprintln(writer, "ID\tTYPE\tNAME\tROLE")
	for _, item := range items {
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", item.Resource.ID, item.Resource.Type, item.Resource.Name, item.Role)
	}
	writer.Flush()
}

package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/ngochc/dev-dash/internal/config"
	"github.com/ngochc/dev-dash/internal/resource"
	"github.com/ngochc/dev-dash/internal/storage/sqlite"
)

func runResource(ctx context.Context, args []string, output io.Writer) error {
	if err := validateResourceArgs(args); err != nil {
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

	service := resource.NewService(sqlite.NewResourceRepository(db), sqlite.NewResourceTypeRepository(db))
	switch args[0] {
	case "list":
		items, err := service.List(ctx)
		if err != nil {
			return err
		}
		printResourceList(output, items)
		return nil
	case "show":
		item, err := service.Get(ctx, args[1])
		if err != nil {
			return err
		}
		printResource(output, item)
		return nil
	case "add":
		url := ""
		if len(args) == 4 {
			url = args[3]
		}
		item, err := service.Add(ctx, args[1], args[2], url)
		if err != nil {
			return err
		}
		fmt.Fprintf(output, "Resource added: %s\n", item.ID)
		return nil
	case "update":
		url := ""
		if len(args) == 5 {
			url = args[4]
		}
		item, err := service.Update(ctx, args[1], args[2], args[3], url)
		if err != nil {
			return err
		}
		fmt.Fprintf(output, "Resource updated: %s\n", item.ID)
		return nil
	case "remove":
		item, err := service.Remove(ctx, args[1])
		if err != nil {
			return err
		}
		fmt.Fprintf(output, "Resource removed: %s\n", item.ID)
		return nil
	}
	panic("validated resource command was not handled")
}

func validateResourceArgs(args []string) error {
	if len(args) == 0 {
		return errors.New("resource command required: add, list, show, update, or remove")
	}
	switch args[0] {
	case "list":
		if len(args) != 1 {
			return errors.New("usage: devdash resource list")
		}
	case "show":
		if len(args) != 2 {
			return errors.New("usage: devdash resource show <id>")
		}
	case "add":
		if len(args) < 3 || len(args) > 4 {
			return errors.New("usage: devdash resource add <type> <name> [url]")
		}
	case "update":
		if len(args) < 4 || len(args) > 5 {
			return errors.New("usage: devdash resource update <id> <type> <name> [url]")
		}
	case "remove":
		if len(args) != 2 {
			return errors.New("usage: devdash resource remove <id>")
		}
	default:
		return fmt.Errorf("unknown resource command: %s", args[0])
	}
	return nil
}

func printResourceList(output io.Writer, items []resource.Resource) {
	if len(items) == 0 {
		fmt.Fprintln(output, "No resources found.")
		return
	}
	writer := tabwriter.NewWriter(output, 0, 4, 3, ' ', 0)
	fmt.Fprintln(writer, "ID\tTYPE\tNAME\tURL")
	for _, item := range items {
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", item.ID, item.Type, item.Name, item.URL)
	}
	writer.Flush()
}

func printResource(output io.Writer, item resource.Resource) {
	fmt.Fprintf(output, "ID: %s\nType: %s\nName: %s\nURL: %s\n", item.ID, item.Type, item.Name, item.URL)
}

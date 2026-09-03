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

func runResourceType(ctx context.Context, args []string, output io.Writer) error {
	if err := validateResourceTypeArgs(args); err != nil {
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

	service := resource.NewTypeService(sqlite.NewResourceTypeRepository(db))
	switch args[0] {
	case "list":
		items, err := service.List(ctx)
		if err != nil {
			return err
		}
		printResourceTypeList(output, items)
		return nil
	case "show":
		item, err := service.Get(ctx, args[1])
		if err != nil {
			return err
		}
		fmt.Fprintf(output, "Name: %s\nDisplay name: %s\nOwner: %s\nDescription: %s\n", item.Name, item.DisplayName, item.Owner, item.Description)
		return nil
	case "add":
		owner, description := "", ""
		if len(args) >= 4 {
			owner = args[3]
		}
		if len(args) == 5 {
			description = args[4]
		}
		item, err := service.Register(ctx, args[1], args[2], owner, description)
		if err != nil {
			return err
		}
		fmt.Fprintf(output, "Resource type registered: %s\n", item.Name)
		return nil
	}
	panic("validated resource-type command was not handled")
}

func validateResourceTypeArgs(args []string) error {
	if len(args) == 0 {
		return errors.New("resource-type command required: add, list, or show")
	}
	switch args[0] {
	case "list":
		if len(args) != 1 {
			return errors.New("usage: devdash resource-type list")
		}
	case "show":
		if len(args) != 2 {
			return errors.New("usage: devdash resource-type show <name>")
		}
	case "add":
		if len(args) < 3 || len(args) > 5 {
			return errors.New("usage: devdash resource-type add <name> <display-name> [owner] [description]")
		}
	default:
		return fmt.Errorf("unknown resource-type command: %s", args[0])
	}
	return nil
}

func printResourceTypeList(output io.Writer, items []resource.ResourceType) {
	if len(items) == 0 {
		fmt.Fprintln(output, "No resource types found.")
		return
	}
	writer := tabwriter.NewWriter(output, 0, 4, 3, ' ', 0)
	fmt.Fprintln(writer, "NAME\tDISPLAY NAME\tOWNER")
	for _, item := range items {
		fmt.Fprintf(writer, "%s\t%s\t%s\n", item.Name, item.DisplayName, item.Owner)
	}
	writer.Flush()
}

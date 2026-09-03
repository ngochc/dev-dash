package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/ngochc/dev-dash/internal/config"
	"github.com/ngochc/dev-dash/internal/graph"
	"github.com/ngochc/dev-dash/internal/storage/sqlite"
)

func runRelationType(ctx context.Context, args []string, output io.Writer) error {
	if err := validateRelationTypeArgs(args); err != nil {
		return err
	}
	var symmetric bool
	if args[0] == "add" {
		switch args[4] {
		case "true":
			symmetric = true
		case "false":
			symmetric = false
		default:
			return errors.New("symmetric must be true or false")
		}
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

	service := graph.NewTypeService(sqlite.NewRelationTypeRepository(db))
	switch args[0] {
	case "list":
		items, err := service.List(ctx)
		if err != nil {
			return err
		}
		printRelationTypeList(output, items)
		return nil
	case "show":
		item, err := service.Get(ctx, args[1])
		if err != nil {
			return err
		}
		fmt.Fprintf(output, "Name: %s\nDisplay name: %s\nInverse: %s\nSymmetric: %t\nOwner: %s\nDescription: %s\n", item.Name, item.DisplayName, item.InverseName, item.Symmetric, item.Owner, item.Description)
		return nil
	case "add":
		inverseName := args[3]
		if inverseName == "-" {
			inverseName = ""
		}
		owner, description := "", ""
		if len(args) >= 6 {
			owner = args[5]
		}
		if len(args) == 7 {
			description = args[6]
		}
		item, err := service.Register(ctx, args[1], args[2], inverseName, symmetric, owner, description)
		if err != nil {
			return err
		}
		fmt.Fprintf(output, "Relation type registered: %s\n", item.Name)
		return nil
	}
	panic("validated relation-type command was not handled")
}

func validateRelationTypeArgs(args []string) error {
	if len(args) == 0 {
		return errors.New("relation-type command required: add, list, or show")
	}
	switch args[0] {
	case "list":
		if len(args) != 1 {
			return errors.New("usage: devdash relation-type list")
		}
	case "show":
		if len(args) != 2 {
			return errors.New("usage: devdash relation-type show <name>")
		}
	case "add":
		if len(args) < 5 || len(args) > 7 {
			return errors.New("usage: devdash relation-type add <name> <display-name> <inverse-name-or-> <true|false> [owner] [description]")
		}
	default:
		return fmt.Errorf("unknown relation-type command: %s", args[0])
	}
	return nil
}

func printRelationTypeList(output io.Writer, items []graph.RelationType) {
	if len(items) == 0 {
		fmt.Fprintln(output, "No relation types found.")
		return
	}
	writer := tabwriter.NewWriter(output, 0, 4, 3, ' ', 0)
	fmt.Fprintln(writer, "NAME\tDISPLAY NAME\tINVERSE\tSYMMETRIC\tOWNER")
	for _, item := range items {
		fmt.Fprintf(writer, "%s\t%s\t%s\t%t\t%s\n", item.Name, item.DisplayName, item.InverseName, item.Symmetric, item.Owner)
	}
	writer.Flush()
}

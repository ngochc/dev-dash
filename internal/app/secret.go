package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/ngochc/dev-dash/internal/config"
	"github.com/ngochc/dev-dash/internal/secret"
	"github.com/ngochc/dev-dash/internal/storage/sqlite"
	"golang.org/x/term"
)

func runSecret(ctx context.Context, args []string, input io.Reader, output, feedback io.Writer) error {
	if err := validateSecretArgs(args); err != nil {
		return err
	}
	if args[0] != "list" {
		if err := secret.ValidateKey(args[1]); err != nil {
			return err
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

	service := secret.NewService(sqlite.NewSecretRepository(db))

	switch args[0] {
	case "set":
		value, err := readSecret(input, feedback)
		if err != nil {
			return err
		}
		if err := service.Set(ctx, args[1], value); err != nil {
			return err
		}
		fmt.Fprintf(output, "Secret stored: %s\n", args[1])
		return nil

	case "get":
		item, err := service.Get(ctx, args[1])
		if err != nil {
			return err
		}
		fmt.Fprintln(output, item.Value)
		return nil

	case "show":
		item, err := service.Get(ctx, args[1])
		if err != nil {
			return err
		}
		fmt.Fprintf(output, "%s  %s\n", item.Key, secret.Mask(item.Value))
		return nil

	case "list":
		items, err := service.List(ctx)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			fmt.Fprintln(output, "No secrets found.")
			return nil
		}
		for _, item := range items {
			fmt.Fprintln(output, item.Key)
		}
		return nil

	case "delete":
		if err := service.Delete(ctx, args[1]); err != nil {
			return err
		}
		fmt.Fprintf(output, "Secret deleted: %s\n", args[1])
		return nil
	}

	panic("validated secret command was not handled")
}

func validateSecretArgs(args []string) error {
	if len(args) == 0 {
		return errors.New("secret command required: set, get, show, list, or delete")
	}

	switch args[0] {
	case "set", "get", "show", "delete":
		if len(args) != 2 {
			return fmt.Errorf("usage: devdash secret %s <key>", args[0])
		}
	case "list":
		if len(args) != 1 {
			return errors.New("usage: devdash secret list")
		}
	default:
		return fmt.Errorf("unknown secret command: %s", args[0])
	}
	return nil
}

func readSecret(input io.Reader, feedback io.Writer) (string, error) {
	if file, ok := input.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		fmt.Fprint(feedback, "Secret (input hidden; Enter to submit): ")
		value, err := term.ReadPassword(int(file.Fd()))
		if err != nil {
			return "", fmt.Errorf("read secret: %w", err)
		}
		fmt.Fprintln(feedback)
		return string(value), nil
	}

	value, err := io.ReadAll(input)
	if err != nil {
		return "", fmt.Errorf("read secret: %w", err)
	}
	return string(value), nil
}

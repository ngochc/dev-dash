package app

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/ngochc/dev-dash/internal/platform"
)

type updateRunner func(context.Context, io.Writer) error

func Run(ctx context.Context, args []string) error {
	return run(ctx, args, os.Stdin, os.Stdout)
}

func run(ctx context.Context, args []string, input io.Reader, output io.Writer) error {
	return runWithUpdater(ctx, args, input, output, platform.Update)
}

func runWithUpdater(ctx context.Context, args []string, input io.Reader, output io.Writer, updater updateRunner) error {
	if len(args) == 0 {
		fmt.Fprintln(output, "devdash")
		return nil
	}

	switch args[0] {
	case "doctor":
		return runDoctor(ctx, output)
	case "update":
		if len(args) != 1 {
			return fmt.Errorf("usage: devdash update")
		}
		return updater(ctx, output)

	case "workspace":
		return runWorkspace(ctx, args[1:], output)

	case "secret":
		return runSecret(ctx, args[1:], input, output)

	case "help", "-h", "--help":
		printHelp(output)
		return nil

	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func printHelp(output io.Writer) {
	fmt.Fprintln(output, `Usage:
  devdash
  devdash doctor
  devdash update
  devdash secret set <key>
  devdash secret get <key>
  devdash secret show <key>
  devdash secret list
  devdash secret delete <key>
  devdash workspace list
  devdash workspace add <name> [path]
  devdash workspace show <name-or-id>
  devdash workspace remove <name-or-id>
  devdash help`)
}

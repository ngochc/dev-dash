package app

import (
	"context"
	"fmt"
	"io"
	"os"
)

func Run(ctx context.Context, args []string) error {
	return run(ctx, args, os.Stdout)
}

func run(ctx context.Context, args []string, output io.Writer) error {
	if len(args) == 0 {
		fmt.Fprintln(output, "devdash")
		return nil
	}

	switch args[0] {
	case "doctor":
		return runDoctor(ctx, output)

	case "workspace":
		return runWorkspace(ctx, args[1:], output)

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
  devdash workspace list
  devdash workspace add <name> [path]
  devdash workspace show <name-or-id>
  devdash workspace remove <name-or-id>
  devdash help`)
}

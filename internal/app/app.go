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
	case "config":
		return runConfig(args[1:], output)
	case "repo":
		return runRepo(ctx, args[1:], output)

	case "workspace":
		return runWorkspace(ctx, args[1:], input, output)
	case "resource-type":
		return runResourceType(ctx, args[1:], output)

	case "relation-type":
		return runRelationType(ctx, args[1:], output)

	case "resource":
		return runResource(ctx, args[1:], output)

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
  devdash config keys [provider]
  devdash repo refresh <workspace>
  devdash repo list <workspace>
  devdash repo clone <workspace> --all
  devdash repo clone <workspace> <repo> [<repo>...]
  devdash secret set <key>
  devdash secret get <key>
  devdash secret show <key>
  devdash secret list
  devdash secret delete <key>
  devdash resource-type list
  devdash resource-type show <name>
  devdash resource-type add <name> <display-name> [owner] [description]
  devdash relation-type list
  devdash relation-type show <name>
  devdash relation-type add <name> <display-name> <inverse-name-or-> <true|false> [owner] [description]
  devdash resource list
  devdash resource show <id>
  devdash resource add <type> <name> [url]
  devdash resource update <id> <type> <name> [url]
  devdash resource remove <id>
  devdash workspace list
  devdash workspace add <name> [path]
  devdash workspace show <name-or-id>
  devdash workspace remove <name-or-id>
  devdash workspace config list <workspace>
  devdash workspace config get <workspace> <key>
  devdash workspace config set <workspace> <key> <value>
  devdash workspace config unset <workspace> <key>
  devdash workspace config edit <workspace>
  devdash workspace resource list <workspace-name-or-id>
  devdash workspace resource add <workspace-name-or-id> <resource-id> [role]
  devdash workspace resource remove <workspace-name-or-id> <resource-id>
  devdash help`)
}

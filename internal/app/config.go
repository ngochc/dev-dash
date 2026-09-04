package app

import (
	"errors"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/ngochc/dev-dash/internal/configdef"
	confluenceintegration "github.com/ngochc/dev-dash/internal/integration/confluence"
	githubintegration "github.com/ngochc/dev-dash/internal/integration/github"
)

var configDefinitions = configdef.NewRegistry(append(githubintegration.Definitions, confluenceintegration.Definitions...)...)

func runConfig(args []string, output io.Writer) error {
	if len(args) == 0 {
		return errors.New("config command required: keys")
	}
	if args[0] != "keys" {
		return fmt.Errorf("unknown config command: %s", args[0])
	}
	if len(args) > 2 {
		return errors.New("usage: devdash config keys [provider]")
	}

	provider := ""
	if len(args) == 2 {
		provider = args[1]
	}
	definitions := configDefinitions.List(provider)
	if provider != "" && len(definitions) == 0 {
		return fmt.Errorf("unknown config provider: %s", provider)
	}

	writer := tabwriter.NewWriter(output, 0, 4, 3, ' ', 0)
	fmt.Fprintln(writer, "KEY\tREQUIRED\tDEFAULT\tDESCRIPTION")
	for _, definition := range definitions {
		required := "no"
		if definition.Required {
			required = "yes"
		}
		defaultValue := definition.Default
		if defaultValue == "" {
			defaultValue = "-"
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", definition.Name, required, defaultValue, definition.Description)
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush config keys: %w", err)
	}
	return nil
}

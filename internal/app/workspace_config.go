package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/ngochc/dev-dash/internal/config"
	"github.com/ngochc/dev-dash/internal/platform"
	"github.com/ngochc/dev-dash/internal/storage/sqlite"
	"github.com/ngochc/dev-dash/internal/workspace"
)

type workspaceConfigEditor func(context.Context, []byte, io.Reader, io.Writer) ([]byte, error)

func runWorkspaceConfig(ctx context.Context, args []string, input io.Reader, output, feedback io.Writer) error {
	return runWorkspaceConfigWithEditor(ctx, args, input, output, feedback, func(
		ctx context.Context,
		initial []byte,
		input io.Reader,
		feedback io.Writer,
	) ([]byte, error) {
		return platform.EditTemporaryFile(ctx, "devdash-workspace-*.conf", initial, input, feedback)
	})
}

func runWorkspaceConfigWithEditor(
	ctx context.Context,
	args []string,
	input io.Reader,
	output io.Writer,
	feedback io.Writer,
	editor workspaceConfigEditor,
) error {
	if err := validateWorkspaceConfigArgs(args); err != nil {
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

	service := workspace.NewConfigService(
		sqlite.NewWorkspaceRepository(db),
		sqlite.NewWorkspaceConfigRepository(db),
	)
	switch args[0] {
	case "list":
		_, entries, err := service.ListUser(ctx, args[1])
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			fmt.Fprintln(output, "No workspace config found.")
			return nil
		}
		writer := tabwriter.NewWriter(output, 0, 4, 3, ' ', 0)
		for _, entry := range entries {
			fmt.Fprintf(writer, "%s\t%s\n", entry.FullKey(), entry.Value)
		}
		if err := writer.Flush(); err != nil {
			return fmt.Errorf("flush workspace config list: %w", err)
		}
		return nil

	case "get":
		_, entry, err := service.Get(ctx, args[1], args[2])
		if err != nil {
			return err
		}
		fmt.Fprintln(output, entry.Value)
		return nil

	case "set":
		if _, err := service.SetUser(ctx, args[1], args[2], args[3]); err != nil {
			return err
		}
		fmt.Fprintf(output, "Workspace config updated: %s\n", args[2])
		return nil

	case "unset":
		if _, err := service.UnsetUser(ctx, args[1], args[2]); err != nil {
			return err
		}
		fmt.Fprintf(output, "Workspace config removed: %s\n", args[2])
		return nil

	case "edit":
		item, entries, err := service.ListUser(ctx, args[1])
		if err != nil {
			return err
		}
		fmt.Fprintln(feedback, "Opening editor. Save and close to apply; close without saving to keep current values.")
		edited, err := editor(ctx, formatWorkspaceConfig(entries), input, feedback)
		if err != nil {
			return err
		}
		entries, err = parseWorkspaceConfig(edited)
		if err != nil {
			return err
		}
		if _, err := service.ReplaceUser(ctx, args[1], entries); err != nil {
			return err
		}
		fmt.Fprintf(output, "Workspace config updated: %s\n", item.Name)
		return nil
	}

	panic("validated workspace config command was not handled")
}

func validateWorkspaceConfigArgs(args []string) error {
	if len(args) == 0 {
		return errors.New("workspace config command required: edit, get, list, set, or unset")
	}

	switch args[0] {
	case "list", "edit":
		if len(args) != 2 {
			return fmt.Errorf("usage: devdash workspace config %s <workspace>", args[0])
		}
	case "get", "unset":
		if len(args) != 3 {
			return fmt.Errorf("usage: devdash workspace config %s <workspace> <key>", args[0])
		}
	case "set":
		if len(args) != 4 {
			return errors.New("usage: devdash workspace config set <workspace> <key> <value>")
		}
	default:
		return fmt.Errorf("unknown workspace config command: %s", args[0])
	}
	return nil
}

func formatWorkspaceConfig(entries []workspace.ConfigEntry) []byte {
	sorted := append([]workspace.ConfigEntry(nil), entries...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Namespace == sorted[j].Namespace {
			return sorted[i].Key < sorted[j].Key
		}
		return sorted[i].Namespace < sorted[j].Namespace
	})

	var formatted strings.Builder
	for _, entry := range sorted {
		fmt.Fprintf(&formatted, "%s=%s\n", entry.FullKey(), entry.Value)
	}
	return []byte(formatted.String())
}

func parseWorkspaceConfig(data []byte) ([]workspace.ConfigEntry, error) {
	lines := strings.Split(string(data), "\n")
	entries := make([]workspace.ConfigEntry, 0, len(lines))
	seen := make(map[string]struct{}, len(lines))
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		keyText, value, found := strings.Cut(line, "=")
		keyText = strings.TrimSpace(keyText)
		if !found || keyText == "" {
			return nil, fmt.Errorf("invalid workspace config at line %d: expected key=value", index+1)
		}
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, fmt.Errorf("invalid workspace config at line %d: value is required", index+1)
		}
		namespace, key, err := workspace.ParseConfigKey(keyText)
		if err != nil {
			return nil, fmt.Errorf("invalid workspace config at line %d: %v", index+1, err)
		}
		fullKey := namespace + "." + key
		if _, exists := seen[fullKey]; exists {
			return nil, fmt.Errorf("invalid workspace config at line %d: duplicate key %q", index+1, fullKey)
		}
		seen[fullKey] = struct{}{}
		entries = append(entries, workspace.ConfigEntry{Namespace: namespace, Key: key, Value: value})
	}
	return entries, nil
}

# CLI Reference

This page lists every command in `devdash --help`. Command errors and unknown subcommands exit nonzero.

## General

| Command | Observable behavior |
| --- | --- |
| `devdash` | Prints `devdash`. |
| `devdash help` | Prints complete usage. |
| `devdash -h` | Prints complete usage. |
| `devdash --help` | Prints complete usage. |
| `devdash doctor` | Opens the configured database, applies migrations, and reports database, SQLite, and migration status. |
| `devdash version` | Prints the application-tracked release version. |
| `devdash update` | Reinstalls the latest checksum-verified release over the resolved current executable. |

## Workspace

| Command | Observable behavior |
| --- | --- |
| `devdash workspace list` | Lists workspaces ordered by name. |
| `devdash workspace add <name> [path]` | Adds an existing explicit directory or creates `~/devdash/<name>` when path is omitted. |
| `devdash workspace show <name-or-id>` | Prints ID, name, and path, resolving exact ID before exact name. |
| `devdash workspace setup <workspace>` | Selects GitHub and/or Confluence, then guides the selected provider configuration. |
| `devdash workspace check <workspace>` | Reports non-mutating workspace, per-provider, cached repository, and cached wiki readiness. |
| `devdash workspace remove <name-or-id>` | Removes registry state, not the filesystem directory. |

## Workspace configuration

| Command | Observable behavior |
| --- | --- |
| `devdash workspace config list <workspace>` | Lists user configuration sorted by namespace/key; reserved internal entries are hidden. |
| `devdash workspace config get <workspace> <key>` | Prints the raw stored value to stdout. |
| `devdash workspace config set <workspace> <key> <value>` | Creates or updates one trimmed, nonempty, single-line value. |
| `devdash workspace config unset <workspace> <key>` | Removes one exact user key. |
| `devdash workspace config edit <workspace>` | Edits `key=value` lines through `$VISUAL` or `$EDITOR` and transactionally replaces user values while preserving internal entries. |

## Configuration definitions

| Command | Observable behavior |
| --- | --- |
| `devdash config keys [provider]` | Lists known integration definitions, requirements, defaults, and descriptions; the optional provider filters the list. |

## Secrets

| Command | Observable behavior |
| --- | --- |
| `devdash secret set <key>` | Reads without echo from a terminal or consumes all piped stdin bytes, then stores the value. |
| `devdash secret get <key>` | Prints the raw value to stdout. **This output is sensitive.** |
| `devdash secret show <key>` | Prints the key and masked value. |
| `devdash secret list` | Prints sorted keys without values. |
| `devdash secret delete <key>` | Deletes one key. |

## Repositories

| Command | Observable behavior |
| --- | --- |
| `devdash repo refresh <workspace>` | Validates GitHub configuration/authentication and additively refreshes up to 1000 non-archived repositories. |
| `devdash repo list <workspace>` | Lists cached repositories with state derived from tracked checkout paths. |
| `devdash repo pick <workspace>` | Refreshes once, interactively selects repositories, and clones the selected set. |
| `devdash repo clone <workspace> --all` | Refreshes, then processes every known repository conservatively. |
| `devdash repo clone <workspace> <repo> [<repo>...]` | Refreshes, resolves exact owner/name or unique short-name selectors, and processes each repository. |

## Wiki pages

| Command | Observable behavior |
| --- | --- |
| `devdash wiki refresh <workspace>` | Validates Confluence Data Center configuration and PAT, then additively refreshes page metadata without bodies. |
| `devdash wiki list <workspace>` | Offline list of cached pages with `not-fetched`, `fetched`, or `missing` state derived from tracked paths. |
| `devdash wiki fetch <workspace> --all` | Refreshes metadata, then fetches and materializes every discovered page. |
| `devdash wiki fetch <workspace> <page> [<page>...]` | Refreshes metadata, resolves exact page IDs or unique exact titles, and materializes selections in selector order. |

Fetch output includes each page result before any aggregate failure is returned. `--all` must be the sole selector.

## Resource types

| Command | Observable behavior |
| --- | --- |
| `devdash resource-type list` | Lists registered resource types. |
| `devdash resource-type show <name>` | Prints one registered resource type. |
| `devdash resource-type add <name> <display-name> [owner] [description]` | Registers a new stable resource type. Quote multiword arguments in the shell. |

## Relation types

| Command | Observable behavior |
| --- | --- |
| `devdash relation-type list` | Lists registered relation types. |
| `devdash relation-type show <name>` | Prints one registered relation type. |
| `devdash relation-type add <name> <display-name> <inverse-name-or-> <true|false> [owner] [description]` | Registers relation semantics; use `-` for no inverse and a literal boolean for symmetry. |

Relation-type commands manage vocabulary only. Relation-edge operations are not implemented.

## Resources

| Command | Observable behavior |
| --- | --- |
| `devdash resource list` | Lists logical resources ordered by name, then ID. |
| `devdash resource show <id>` | Prints one resource by opaque ID. |
| `devdash resource add <type> <name> [url]` | Creates a resource using an existing registered type. |
| `devdash resource update <id> <type> <name> [url]` | Replaces the resource's editable type, name, and URL. |
| `devdash resource remove <id>` | Deletes one resource and cascading dependent rows. |

## Workspace resources

| Command | Observable behavior |
| --- | --- |
| `devdash workspace resource list <workspace-name-or-id>` | Lists resources associated with a workspace. |
| `devdash workspace resource add <workspace-name-or-id> <resource-id> [role]` | Adds one membership without changing resource identity. |
| `devdash workspace resource remove <workspace-name-or-id> <resource-id>` | Removes one membership without deleting the resource. |

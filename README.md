# Devdash

Devdash is a local developer-context registry and resource graph written in Go. It is designed to connect workspaces, repositories, branches, tickets, documents, integrations, aliases, and their relationships without putting provider-specific concepts into the core model.

## Status

Early development. The repository currently implements:

- a CLI dispatcher with workspace, config-definition, repository, registry, resource, membership, secret, doctor, and update commands;
- database-path resolution through `DEVDASH_DB` or `~/.devdash/devdash.db`;
- SQLite connection setup with required pragmas;
- embedded Goose migration loading;
- an initial provider-neutral schema and metadata seed data;
- resource-type and relation-type registry services backed by SQLite;
- provider-neutral resource CRUD, workspace configuration ownership, GitHub repository discovery, derived checkout state, and safe cloning.

Alias resolution, relation-edge operations, and Jira/Confluence provider behavior are not implemented yet.

## Install

Install the latest release on macOS or Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/ngochc/dev-dash/main/install.sh | sh
```

The installer supports amd64 and arm64, verifies the selected GitHub Release archive against the published SHA-256 checksums, and installs `devdash` to `~/.local/bin/devdash` by default. It prints the required `PATH` update when `~/.local/bin` is not already present.

Choose another destination or pin a release tag by setting installer environment variables on `sh`:

```bash
curl -fsSL https://raw.githubusercontent.com/ngochc/dev-dash/main/install.sh | DEVDASH_INSTALL_DIR="$HOME/bin" sh
curl -fsSL https://raw.githubusercontent.com/ngochc/dev-dash/main/install.sh | DEVDASH_VERSION=v0.1.0 sh
```

Update the current executable in place:

```bash
devdash update
```

The update command always reinstalls the latest checksum-verified GitHub Release over the resolved current executable. It fails if that executable's directory is not writable.

If the update command is unavailable or cannot run, rerun the installer manually:

```bash
curl -fsSL https://raw.githubusercontent.com/ngochc/dev-dash/main/install.sh | sh
```

For a custom installation directory, repeat the original override so the intended copy is replaced instead of installing to `~/.local/bin`:

```bash
curl -fsSL https://raw.githubusercontent.com/ngochc/dev-dash/main/install.sh | DEVDASH_INSTALL_DIR="$HOME/bin" sh
```

## Development requirements

- Go `1.26.0`
- `asdf` for the project-managed Go installation
- SQLite CLI (optional, for database inspection)
- `staticcheck` (optional, for the full validation pass)

Install and verify the project runtime:

```bash
asdf install
asdf current golang
go version
```

The repository pins Go in `.tool-versions`. The current `Makefile` defines no working targets, so use direct Go commands.

## Usage

Show help:

```bash
go run ./cmd/devdash --help
```

Initialize and check the configured database:

```bash
go run ./cmd/devdash doctor
```

Use an isolated database during development:

```bash
DEVDASH_DB="$(mktemp -d)/devdash.db" go run ./cmd/devdash doctor
```

Manage workspaces:

```bash
go run ./cmd/devdash workspace list
go run ./cmd/devdash workspace add devdash "$PWD"
go run ./cmd/devdash workspace show devdash
go run ./cmd/devdash workspace remove devdash
```

Discover supported configuration keys and configure a workspace:

```bash
go run ./cmd/devdash config keys github
go run ./cmd/devdash workspace config set devdash github.org acme
# Optional GitHub Enterprise override; github.com is the default.
go run ./cmd/devdash workspace config set devdash github.base_url https://git.example.com
go run ./cmd/devdash workspace config list devdash
EDITOR="code --wait" go run ./cmd/devdash workspace config edit devdash
```

Configuration keys remain open-ended, but keys beginning with `_` are reserved for Devdash internal state. Normal config list/edit operations hide internal keys and preserve them during edits. User configuration and credential references such as `jira.secret=secret:jira-work` remain editable; generic config operations never resolve secret references.

GitHub operations apply the default `github.base_url=https://github.com` when no override is stored. They fail before invoking `gh` when `github.org` is missing or the base URL is invalid, and explain how to complete the configuration. `gh` authentication remains external to Devdash; GitHub Enterprise commands receive a process-local `GH_HOST` without changing global GitHub CLI configuration.

Refresh repository metadata, inspect derived clone state, and clone selected or all repositories:

```bash
go run ./cmd/devdash repo refresh devdash
go run ./cmd/devdash repo list devdash
go run ./cmd/devdash repo clone devdash web api
go run ./cmd/devdash repo clone devdash --all
```

Repositories are discovered only through GitHub and associated with the workspace as provider-neutral repository resources. Devdash does not scan the workspace for arbitrary Git repositories. A workspace rooted at `~/devdash/devdash` places clones under `~/devdash/devdash/repos/<repo>`.

Repository list state is derived from tracked `local_checkout` locations:

- `not-cloned`: no tracked checkout;
- `cloned`: the tracked path is a Git checkout with the expected origin;
- `missing`: a tracked checkout path no longer exists;
- `invalid`: the tracked path exists but is not the expected repository.

Clone never fetches, pulls, resets, or updates an existing checkout. An untracked exact destination is adopted only when its Git origin matches the expected repository; conflicting destinations are left untouched.

Manage resource vocabulary, resources, and workspace membership:

```bash
go run ./cmd/devdash resource-type list
go run ./cmd/devdash resource-type add service_component "Service Component" core
go run ./cmd/devdash relation-type list
go run ./cmd/devdash relation-type add supports Supports supported_by false core
go run ./cmd/devdash resource add service_component api https://example.test/api
go run ./cmd/devdash workspace resource add devdash <resource-id> primary
go run ./cmd/devdash workspace resource list devdash
```

Current command behavior:

| Command | Behavior |
| --- | --- |
| `devdash` | Prints the application name. |
| `devdash help`, `-h`, `--help` | Prints CLI usage. |
| `devdash doctor` | Opens the database, applies migrations, and reports database, SQLite, and migration status. |
| `devdash update` | Reinstalls the latest checksum-verified GitHub Release over the resolved current executable. |
| `devdash config keys [provider]` | Lists configuration keys understood by integrations, including requirements and defaults. |
| `devdash workspace list` | Lists workspaces ordered by name. |
| `devdash workspace add <name> [path]` | Adds a workspace using the supplied directory or the current directory. |
| `devdash workspace show <name-or-id>` | Shows a workspace, resolving exact ID before exact name. |
| `devdash workspace remove <name-or-id>` | Removes a workspace, resolving exact ID before exact name. |
| `devdash workspace config list <workspace>` | Lists namespaced configuration ordered by namespace and key. |
| `devdash workspace config get <workspace> <key>` | Prints one raw configuration value. |
| `devdash workspace config set <workspace> <key> <value>` | Creates or updates one configuration value. |
| `devdash workspace config unset <workspace> <key>` | Removes one exact configuration key. |
| `devdash workspace config edit <workspace>` | Atomically replaces user configuration through `$VISUAL` or `$EDITOR` while preserving reserved internal keys. |
| `devdash repo refresh <workspace>` | Discovers up to 1000 non-archived GitHub repositories and idempotently associates them with the workspace. |
| `devdash repo list <workspace>` | Lists known repositories with clone state derived only from tracked checkout paths. |
| `devdash repo clone <workspace> <repo> [<repo>...]` | Refreshes metadata and clones selected repositories by unique name or exact owner/name. |
| `devdash repo clone <workspace> --all` | Refreshes metadata and clones or restores every repository that needs a checkout while reporting independent failures. |
| `devdash resource-type list`, `show`, `add` | Lists, inspects, or registers stable provider-neutral resource types. |
| `devdash relation-type list`, `show`, `add` | Lists, inspects, or registers directed or symmetric relation types. |
| `devdash resource list`, `show`, `add`, `update`, `remove` | Manages provider-neutral logical resources by opaque ID. |
| `devdash workspace resource list`, `add`, `remove` | Manages resource membership without changing resource identity. |
| Any other command | Returns an `unknown command` error and exits non-zero. |

## Database

The database path is resolved in this order:

1. `DEVDASH_DB`
2. `$HOME/.devdash/devdash.db`

The SQLite adapter creates the parent directory and enables:

```text
foreign_keys = ON
journal_mode = WAL
busy_timeout = 5000
```

Migrations are embedded from `internal/storage/migrations/` and applied by Goose when the database is opened. Do not edit a migration after it has been applied in shared project history; add a new numbered migration instead.

The initial schema models:

- workspaces, normalized namespaced workspace configuration, and many-to-many workspace/resource membership;
- repository checkout locations and their workspace ownership;
- integrations and provider-neutral resources;
- physical resource locations separate from logical identity;
- workspace-scoped and global aliases;
- directed, typed resource relations;
- secrets, tags, and resource/tag membership.

## Architecture

```text
cmd/devdash
    -> internal/app
       -> internal/config
       -> provider-neutral domain packages
       -> internal/integration/*
       -> internal/platform
       -> internal/storage/sqlite
          -> embedded migrations
```

- `cmd/devdash/`: thin executable entry point.
- `internal/app/`: command dispatch and application orchestration.
- `internal/config/`: environment and path resolution.
- `internal/configdef/`: provider-neutral configuration definitions and discovery.
- `internal/{domain,workspace,resource,repository,alias,graph}/`: provider-neutral model and behavior.
- `internal/integration/github/`: GitHub configuration, `gh` discovery, authentication checks, and cloning.
- `internal/integration/git/`: targeted Git checkout and origin inspection.
- `internal/platform/`: filesystem, editor, process, and OS boundaries.
- `internal/storage/sqlite/`: SQLite setup and persistence adapters.
- `internal/storage/migrations/`: embedded schema and seed migrations.

Core design rules:

- A resource is the universal logical identity and may belong to multiple workspaces.
- Physical paths and worktrees are resource locations, not resource identity.
- Relations are directed and typed; inverse relation metadata does not automatically create inverse edges.
- Provider-specific behavior stays in `internal/integration/<provider>`; generic workspace configuration treats namespaces and values as opaque data.
- Store `secret:<key>` credential references, not tokens or secrets, in workspace or integration configuration.
- Repository discovery is remote-only; local inspection is limited to tracked or exact expected checkout paths.
- Clone state is derived from `resource_locations` plus targeted filesystem/Git checks and is never persisted as config or metadata.

See `AGENTS.md` for detailed repository conventions and model invariants.

## Development

Format and validate changes:

```bash
go fmt ./...
go vet ./...
staticcheck ./...
go test ./...
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out
go build ./cmd/devdash
```

Use `go mod tidy` only after imports or dependencies change, and review the resulting `go.mod` and `go.sum` changes.

Tests cover configuration ownership and definitions, GitHub command execution, repository refresh and persistence, targeted Git origin inspection, clone-state derivation, clone safety, registry and resource services, SQLite persistence, CLI dispatch, path resolution, and migrations. Project statement coverage must remain above 80%. Automated tests use fakes for GitHub operations and temporary paths for storage and checkout behavior; they never call real GitHub repositories or scan user directories.

## Roadmap

Near-term work is expected to establish:

1. workspace-scoped and global alias resolution;
2. relation-edge CRUD and traversal;
3. Jira and Confluence provider operations.

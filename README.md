# Devdash

Devdash is a local developer-context registry and resource graph written in Go. It is designed to connect workspaces, repositories, branches, tickets, documents, integrations, aliases, and their relationships without putting provider-specific concepts into the core model.

## Status

Early development. The repository currently implements:

- a small CLI dispatcher with `help` and `doctor` commands;
- database-path resolution through `DEVDASH_DB` or `~/.devdash/devdash.db`;
- SQLite connection setup with required pragmas;
- embedded Goose migration loading;
- an initial provider-neutral schema and metadata seed data.

Workspace/resource CRUD, alias resolution, graph operations, and provider integrations are not implemented yet.

## Requirements

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

Current command behavior:

| Command | Behavior |
| --- | --- |
| `devdash` | Prints the application name. |
| `devdash help`, `-h`, `--help` | Prints CLI usage. |
| `devdash doctor` | Opens the database, applies migrations, and reports database, SQLite, and migration status. |
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

- workspaces and many-to-many workspace/resource membership;
- integrations and provider-neutral resources;
- physical resource locations separate from logical identity;
- workspace-scoped and global aliases;
- directed, typed resource relations;
- tags and resource/tag membership.

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
- `internal/{domain,workspace,resource,alias,graph}/`: provider-neutral model and behavior.
- `internal/integration/`: provider contracts and adapters such as Git, GitHub, Jira, and Confluence.
- `internal/platform/`: filesystem, process, Git, and OS boundaries.
- `internal/storage/sqlite/`: SQLite setup and migration execution.
- `internal/storage/migrations/`: embedded schema and seed migrations.

Core design rules:

- A resource is the universal logical identity and may belong to multiple workspaces.
- Physical paths and worktrees are resource locations, not resource identity.
- Relations are directed and typed; inverse relation metadata does not automatically create inverse edges.
- Provider-specific behavior stays in `internal/integration/<provider>`.
- Store credential references, not tokens or secrets, in integration configuration.

See `AGENTS.md` for detailed repository conventions and model invariants.

## Development

Format and validate changes:

```bash
go fmt ./...
go vet ./...
staticcheck ./...
go test ./...
go build ./cmd/devdash
```

Use `go mod tidy` only after imports or dependencies change, and review the resulting `go.mod` and `go.sum` changes.

The SQLite migration path has regression coverage; other packages currently have no tests. There are no CI workflows, lint configuration, or coverage requirements. New behavior should include package-level tests using Go's `testing` package; storage tests should use temporary database paths and must not commit SQLite, WAL, or SHM files.

## Roadmap

Near-term work is expected to establish:

1. resource-type and relation-type registries;
2. workspace and resource CRUD;
3. workspace/resource membership and aliases;
4. relation traversal;
5. local Git integration before authenticated remote providers.

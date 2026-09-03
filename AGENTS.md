# Repository Guidelines

## Project Overview

Devdash is a Go application for managing developer and project context across local and external resources. Its design centers on a provider-neutral resource graph so Git, GitHub, Jira, Confluence, CI/CD, and future providers can share one model.

Current status: early implementation includes CLI dispatch, `doctor`, database-path resolution, SQLite setup, embedded migrations, and an initial schema. CRUD, graph behavior, integrations, and tests are absent. The build and migrations have known blockers documented in `README.md`.

## Architecture & Data Flow

Intended dependency flow:

```text
cmd/devdash -> internal/app -> domain feature packages
                            -> boundary abstractions
                               -> internal/integration/*
                               -> internal/platform
                               -> internal/storage/sqlite
```

- Keep `cmd/devdash` thin: parse the small CLI surface and invoke application behavior.
- Put orchestration in `internal/app`; keep provider-neutral behavior in `domain`, `workspace`, `resource`, `alias`, and `graph`.
- Put provider adapters only in `internal/integration/<provider>`.
- Keep filesystem, subprocess, Git, and OS operations in `internal/platform`.
- Keep persistence concerns in `internal/storage`; SQLite is an adapter, not the domain model.
- Dependencies should point from application/domain logic toward abstractions, with infrastructure at the boundary. Pass dependencies explicitly; do not use global mutable state.

Core model invariants:

- `Resource` is the universal logical identity. Register new resource and relation types before changing the core schema.
- Resources exist independently of workspaces. Model membership through `workspace_resources`; one resource may belong to many workspaces.
- Keep logical identity separate from physical instances such as local paths and worktrees; store those in `resource_locations`.
- Resolve aliases deterministically: workspace-scoped alias, global alias, then native identifier/name. Alias names are scope-unique; only one primary alias exists per resource and scope.
- Relations are directed and typed. Store inverse or symmetric semantics on relation types; do not silently create inverse edges.
- Extend in this order: resource type, relation type, metadata, provider-specific extension table, then core-schema change. Promote frequently queried or constrained metadata into typed tables keyed by `resources.id`.

The implemented flow is `cmd/devdash` → `internal/app` → `internal/config` and `internal/storage/sqlite` → embedded migrations. Domain feature and integration packages remain placeholders; no dependency-injection or concurrency pattern exists yet.

## Key Directories

| Path | Purpose |
| --- | --- |
| `cmd/devdash/` | Intended CLI executable entry point. |
| `internal/app/` | Application assembly, orchestration, and command execution. |
| `internal/domain/` | Shared domain types, IDs, errors, and primitives. |
| `internal/workspace/` | Workspace lifecycle and resource membership. |
| `internal/resource/` | Resource identity and resource-type behavior. |
| `internal/alias/` | Alias registration and deterministic resolution. |
| `internal/graph/` | Relation types, edges, and traversal. |
| `internal/integration/` | Integration contracts and provider adapters such as `git`, `github`, `jira`, and `confluence`. |
| `internal/platform/` | Filesystem, process, Git, and OS boundaries. |
| `internal/config/` | Configuration and path resolution. |
| `internal/storage/sqlite/` | SQLite connection, queries, and migration execution. |
| `internal/storage/migrations/` | Embedded Goose migrations. Never rewrite an applied migration. |

## Development Commands

Go is managed through `asdf`:

```bash
asdf current golang
go version
```

After installing a Go binary with `go install`, run `asdf reshim golang`.

Use direct Go commands; the current `Makefile` defines no working targets.

```bash
go fmt ./...                 # format source
go vet ./...                 # standard static checks
staticcheck ./...            # additional checks; tool is not pinned by this repo
go test ./...                # run all tests
go build ./cmd/devdash       # currently blocked; see README.md
go run ./cmd/devdash         # currently blocked; see README.md
go mod tidy                  # update module metadata after dependency changes
```

The current revision does not build because several placeholder `doc.go` files are empty. After that is corrected, `doctor` remains blocked by invalid migrations. Do not report validation commands as passing unless they ran successfully.

## Code Conventions & Common Patterns

- Use idiomatic, `gofmt`-formatted Go; keep packages small, cohesive, and lower-case.
- Prefer the standard library and simple concrete types. Add an interface only for multiple implementations, a meaningful infrastructure boundary, or a clear testing seam.
- Keep provider names out of core packages and schemas unless the concept is genuinely universal.
- Return explicit errors for invalid or corrupt persisted state. Add operation context and preserve causes with `%w`; do not silently fall back.
- Pass `context.Context` through operations that perform I/O or may block.
- Prefer synchronous code until concurrency is justified. Any goroutine must have explicit ownership, cancellation, and shutdown behavior; no async pattern is established yet.
- Inject stores, clocks, provider clients, and platform operations explicitly through constructors or function parameters. Avoid package-level mutable state.
- Use transactions for writes spanning related records. Let SQLite enforce foreign keys rather than emulating integrity in Go.
- Use opaque string primary keys. Store provider-native identity separately as `provider_id` or `external_key`; never use mutable URLs, display names, or row order as canonical identity.
- Explicitly update `updated_at`; SQLite's `DEFAULT CURRENT_TIMESTAMP` does not update existing rows.
- Store credential references such as `env:JIRA_TOKEN` or `keychain:github-work`, never tokens or credentials, in integration configuration.
- Keep CLI parsing in the standard library until real command complexity justifies a framework.
- Make the smallest coherent change. Do not add speculative integrations or abstractions for hypothetical requirements.

## Important Files

| Path | Relevance |
| --- | --- |
| `go.mod` | Module identity (`github.com/ngochc/dev-dash`), Go `1.26.0`, and dependency versions. All current requirements are indirect. |
| `go.sum` | Dependency integrity checksums. |
| `README.md` | Current behavior, intended usage, architecture, and known migration blockers. |
| `cmd/devdash/main.go` | Executable entry point; delegates to `app.Run` and reports errors on stderr. |
| `internal/app/app.go` | Dispatches the root, help, and doctor commands. |
| `internal/config/paths.go` | Resolves `DEVDASH_DB` or the default database path. |
| `internal/storage/sqlite/sqlite.go` | Creates the database directory, configures SQLite, and invokes migrations. |
| `internal/storage/sqlite/migrate.go` | Runs embedded Goose migrations. |
| `internal/storage/migrations/embed.go` | Embeds migration SQL into the binary. |
| `Makefile` | Contains three recursive `make` command lines but no target definitions; do not rely on it. |
| `.gitignore` | Excludes local Devdash/SQLite state, Go build and test artifacts, local workspace files, environment files, and OS metadata. |

## Runtime/Tooling Preferences

- Runtime: Go `1.26.0`, selected through `asdf`. Do not switch or install Go outside `asdf` unless explicitly requested.
- Dependency management: Go modules. Use the module path declared in `go.mod`; update internal imports if that path changes.
- Database: SQLite through pure-Go `modernc.org/sqlite`.
- Migrations: `github.com/pressly/goose/v3`, embedded from `internal/storage/migrations/`; runtime must not depend on loose migration files.
- Database path precedence: `DEVDASH_DB`, then `$HOME/.devdash/devdash.db`. Create `$HOME/.devdash` when needed; do not substitute OS-specific application-data directories.
- Every SQLite connection must enable `foreign_keys = ON`, `journal_mode = WAL`, and `busy_timeout = 5000`.
- No CI, lint configuration, release tooling, container setup, code generation, vendoring, or separate toolchain directive currently exists.

## Testing & QA

There are currently no `*_test.go` files, fixtures, mocks, benchmarks, fuzz tests, coverage configuration, or CI checks. No third-party test framework is declared.

- Add tests beside the package under test using Go's `testing` package unless a concrete need justifies another dependency.
- Test observable domain behavior and invariants, not implementation plumbing. Cover alias precedence, workspace/resource membership, directed relations, transactions, migration behavior, and provider-failure isolation as those features are implemented.
- Use temporary databases/directories for storage tests. Never commit `.db`, WAL, or SHM artifacts as fixtures.
- Run targeted tests while developing, then before completion run:

```bash
go fmt ./...
go vet ./...
staticcheck ./...
go test ./...
go build ./cmd/devdash
```

- Run `go mod tidy` only when imports or dependencies changed, and review its changes.
- The repository defines no coverage threshold. Do not treat an empty test suite as meaningful verification; add a regression test for each fixed bug and tests for new observable contracts.
- If a required tool is unavailable, report that fact rather than claiming the check passed.

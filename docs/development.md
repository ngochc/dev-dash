# Development

## Toolchain

The module is `github.com/ngochc/dev-dash`. `.tool-versions` pins Go `1.26.0`, managed through `asdf`.

```bash
asdf current golang
go version
```

Use direct Go commands. The current `Makefile` contains recursive `make` command lines but defines no usable targets.

## Package map

```text
cmd/devdash/                    thin executable entry point
internal/app/                   CLI dispatch, composition, orchestration
internal/config/                database path resolution
internal/configdef/             integration configuration definitions
internal/workspace/             workspace, config, and membership behavior
internal/resource/              resource and resource-type behavior
internal/repository/            provider-neutral repository model/state
internal/graph/                 relation-type behavior
internal/secret/                secret behavior
internal/domain/                provider-neutral placeholder
internal/alias/                 alias placeholder
internal/integration/           provider/external command adapters
internal/platform/              filesystem, editor, path, update boundaries
internal/ui/picker/             fzf and numbered interactive selection
internal/storage/sqlite/        SQLite repository adapters
internal/storage/migrations/    embedded numbered Goose migrations
```

Keep `cmd/devdash` thin, provider names out of core packages, and infrastructure in adapter packages. Pass dependencies and `context.Context` explicitly.

## Everyday commands

```bash
go fmt ./...
go vet ./...
staticcheck ./...
go test ./...
go build -o bin/devdash ./cmd/devdash
```

`staticcheck` is not pinned by this repository. If it is unavailable, report that fact; do not claim it passed.

Run `go mod tidy` only after import or dependency changes, then review the resulting `go.mod` and `go.sum` changes. It is not a routine documentation or validation command.

## Isolated databases

Normal state uses `~/.devdash/devdash.db`. A fixed temporary override is useful when collision risk is controlled:

```bash
DEVDASH_DB=/tmp/devdash-test.db go run ./cmd/devdash doctor
```

Prefer a collision-safe temporary directory for repeated or concurrent work:

```bash
DEVDASH_DB="$(mktemp -d)/devdash.db" go run ./cmd/devdash doctor
```

## Contributor completion checks

Run the repository checks from the root in this order:

```bash
sh -n install.sh
sh -n test/install_test.sh
sh test/install_test.sh
go fmt ./...
go vet ./...
staticcheck ./...
go test ./...
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out
go build ./cmd/devdash
go build -o bin/devdash ./cmd/devdash
```

The `total:` statement coverage reported by `go tool cover -func=coverage.out` must be strictly above 80%. Installer changes require the shell syntax and black-box installer tests. Do not report a command as passing unless it ran successfully.

## Release workflow

`.github/workflows/release.yml` runs only when a `v*` tag is pushed; it is not general pull-request or branch CI. It runs Go tests and coverage enforcement, runs installer tests, builds macOS/Linux archives for amd64/arm64, smoke-tests the Linux archive, and publishes checksummed assets to a GitHub Release.

Do not publish a release archive without its entry in `checksums.txt`.

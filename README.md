# Devdash

Devdash is a local developer-context registry and provider-neutral resource graph for connecting workspaces, repositories, tickets, documents, integrations, aliases, and their relationships.

## Status

Implemented: CLI dispatch; workspace lifecycle and configuration; resource, resource-type, relation-type, membership, and secret persistence; SQLite setup and migrations; GitHub repository discovery; derived checkout state; and conservative cloning.

Alias operations, relation-edge operations, Jira integration, and Confluence integration are not implemented. Their schema or design concepts are documented as planned rather than current behavior.

## Requirements

- **Release use:** macOS or Linux on amd64 or arm64.
- **Repository workflows:** `git` for exact checkout inspection and cloning support.
- **GitHub workflows:** `gh`, installed and authenticated externally.
- **Interactive selection:** `fzf` is optional; numbered prompts are the fallback.
- **Source development:** `asdf` with Go `1.26.0` from `.tool-versions`.

## Install, build, and run

Install the latest checksum-verified release. The default destination is `~/.local/bin/devdash`.

```bash
curl -fsSL https://raw.githubusercontent.com/ngochc/dev-dash/main/install.sh | sh
```

Build the current checkout into `bin/devdash`, or build and install it locally. Source builds report version `devel`; `make install` defaults to `~/.local/bin/devdash` and accepts the same `DEVDASH_INSTALL_DIR` override as the release installer.

```bash
make build
./bin/devdash --help
make install
DEVDASH_INSTALL_DIR=/custom/bin make install
```

Run without retaining a binary:

```bash
go run ./cmd/devdash --help
```

## Quick start

Create the default workspace at `~/devdash/mqms`, configure GitHub interactively, check readiness, and inspect discovered repositories:

```bash
devdash workspace add mqms
devdash workspace setup mqms
devdash workspace check mqms
devdash repo list mqms
```

Repositories clone to `~/devdash/mqms/repos/<repo>` by default. The lower-level alternative to guided setup is:

```bash
devdash workspace config set mqms github.org example-org
devdash repo refresh mqms
devdash repo clone mqms repo-a repo-b
```

See [Getting Started](docs/getting-started.md) for installation, setup, readiness, and recovery details.

## Database

Devdash uses SQLite at `DEVDASH_DB` when set, otherwise `~/.devdash/devdash.db`. Opening the database applies embedded migrations automatically. Workspace directories and checkouts remain separate filesystem state. See [Database](docs/database.md).

## Architecture

The CLI is thin, application orchestration lives in `internal/app`, provider-neutral services own core behavior, and GitHub, Git, platform, and SQLite code stay behind adapter boundaries. See [Architecture](docs/architecture.md) and [Data Model](docs/data-model.md).

## CLI examples

```bash
devdash doctor
devdash workspace list
devdash config keys github
devdash secret list
devdash resource list
```

See the [CLI Reference](docs/cli.md) for every command and argument form.

## Documentation

- [Getting Started](docs/getting-started.md)
- [Architecture](docs/architecture.md)
- [Data Model](docs/data-model.md)
- [CLI Reference](docs/cli.md)
- [Development](docs/development.md)

## Development

Use the Make targets as one development-to-release progression:

```bash
version=v0.2.0
make build &&
  make test &&
  make release VERSION="$version" &&
  git tag "$version" &&
  git push origin "$version"
```

Use `make build` while iterating, `make test` as the shared completion gate, and `make release` as the final local rehearsal. The rehearsal builds and verifies the four platform archives plus `dist/checksums.txt`; it does not create tags, push Git state, or publish a GitHub Release.

Only a pushed `v*` tag publishes. The GitHub workflow rebuilds from that tagged commit, creates the release when absent, or uploads the five generated assets with `--clobber` when the release already exists. Existing release notes, state, and differently named assets are preserved. A clobber upload is not atomic: it removes a same-named asset before uploading its replacement, so rerun a failed publication and verify all five generated assets.

Tagging order, safe retries, and post-publication verification are documented in [Development](docs/development.md).

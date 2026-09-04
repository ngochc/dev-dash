# Development

## Toolchain

The module is `github.com/ngochc/dev-dash`. `.tool-versions` pins Go `1.26.0`, managed through `asdf`.

```bash
asdf current golang
go version
```

Use the Make targets for repeatable build, local-install, test, and release-artifact workflows. Direct Go commands remain appropriate for focused development checks.

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
make build
make test
DEVDASH_INSTALL_DIR="$HOME/.local/bin" make install
go fmt ./...
go vet ./...
staticcheck ./...
```

`make build` writes `bin/devdash` with the source-build version `devel`. `make install` rebuilds first, then atomically installs that binary to `DEVDASH_INSTALL_DIR` or, when the override is unset, `$HOME/.local/bin`. `make test` runs the installer and tooling black-box suites, one coverage-enabled `go test ./...` pass, prints the full coverage report, and requires total statement coverage strictly greater than 80%.

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
go fmt ./...
go vet ./...
staticcheck ./...
make test
go build ./cmd/devdash
make build
```

`make test` includes `sh -n` for `install.sh`, every script under `scripts/`, and both shell test suites; it then executes `test/install_test.sh`, `test/tooling_test.sh`, and the coverage-enabled Go suite. The `total:` statement coverage reported by `go tool cover -func` must be strictly above 80%. Do not report a command as passing unless it ran successfully.

## Development-to-release workflow

Use the same repository entry points from local iteration through publication. Normal branch and pull-request work never publishes a release; publication starts only when a `v*` tag is pushed.

### 1. Iterate locally

Run focused Go tests for the packages being changed, then build the source checkout:

```bash
make build
./bin/devdash --help
```

Source builds report version `devel`. Do not use a release version during normal development.

### 2. Run the shared completion gate

Before merging or preparing a release, run:

```bash
make test
```

This is the same test gate used by release artifact generation. It runs shell syntax checks, installer and tooling black-box suites, the coverage-enabled Go suite, and the total statement coverage requirement.

### 3. Rehearse the release locally

Choose the next version once and reuse it through the remaining steps:

```bash
version=v0.2.0
make release VERSION="$version"
```

The version must be nonempty and begin with `v`. `make release` runs the shared test gate, cross-compiles static binaries, injects the candidate version, creates these files, and installs the host archive through the real installer as a smoke test:

```text
dist/devdash_darwin_amd64.tar.gz
dist/devdash_darwin_arm64.tar.gz
dist/devdash_linux_amd64.tar.gz
dist/devdash_linux_arm64.tar.gz
dist/checksums.txt
```

Each archive contains only the root-level `devdash` binary. `checksums.txt` contains one basename-only SHA-256 entry per archive. The command stages its output in a temporary directory and replaces `dist/` only after the tests, builds, checksums, installation, help check, and version check succeed. It never invokes `gh`, creates a tag, pushes Git state, or changes a GitHub Release.

### 4. Publish the tagged commit

Finalize and commit all release content before creating the tag. From the exact commit intended for publication:

```bash
git tag "$version" &&
  git push origin "$version"
```

Treat a pushed release tag as immutable. If the source needs correction, publish a new version instead of moving the tag. Do not upload the local `dist/` manually: `.github/workflows/release.yml` checks out the tagged commit and runs `scripts/release.sh "$GITHUB_REF_NAME"`, so published artifacts are rebuilt from the immutable tagged source.

The publish step supports both release ordering cases:

- If the release does not exist, `gh release create` verifies the tag, attaches the five generated assets, and generates release notes.
- If the release already exists, `gh release upload --clobber` uploads the same five generated assets without deleting or recreating the release. Existing notes, draft/prerelease/publication state, and differently named assets remain unchanged. For each same-named asset, the CLI deletes the old asset before uploading its replacement; an interrupted upload can therefore leave that asset missing.

### 5. Retry and verify publication

If publication fails after the tag is pushed, rerun the failed GitHub Actions workflow for that tag. The job rebuilds from the same tagged commit and replaces only the five generated asset names, but replacement is not atomic. Rerun any failed upload and verify that all five generated assets exist before treating the release as repaired. Do not rebuild from the current development branch, move the tag, or delete the release to retry.

Inspect the final release and exercise its checksum-verified installer path:

```bash
(
  set -eu
  required_assets='checksums.txt
devdash_darwin_amd64.tar.gz
devdash_darwin_arm64.tar.gz
devdash_linux_amd64.tar.gz
devdash_linux_arm64.tar.gz'
  release_assets="$(gh release view "$version" --repo ngochc/dev-dash \
    --json assets --jq '.assets[].name')"
  for asset in $required_assets; do
    printf '%s\n' "$release_assets" | grep -Fx "$asset" >/dev/null
  done

  gh release view "$version" --repo ngochc/dev-dash \
    --json isDraft,isPrerelease,url \
    --jq '{isDraft,isPrerelease,url}'

  install_root="$(mktemp -d)"
  cleanup() { rm -rf "$install_root"; }
  trap cleanup EXIT
  DEVDASH_VERSION="$version" \
    DEVDASH_INSTALL_DIR="$install_root/bin" \
    ./install.sh
  test "$("$install_root/bin/devdash" version)" = "devdash $version"
)
```

The release must contain all four generated platform archives and `checksums.txt`. Never publish an archive without its checksum entry.

# Getting Started

This guide owns the first Devdash workflow: install the CLI, create a workspace, configure GitHub, inspect readiness, and select repositories.

## Requirements

Released binaries support macOS and Linux on amd64 and arm64. Repository workflows require `git`. Guided GitHub setup, discovery, validation, and cloning require an installed, externally authenticated `gh`. Interactive selection uses `fzf` when available and otherwise uses numbered prompts.

Source development uses Go `1.26.0` managed through `asdf`.

## Install a release or build from source

Install the latest checksum-verified release:

```bash
curl -fsSL https://raw.githubusercontent.com/ngochc/dev-dash/main/install.sh | sh
```

The default destination is `~/.local/bin/devdash`.

Build and inspect the CLI from a source checkout:

```bash
go build -o bin/devdash ./cmd/devdash
go run ./cmd/devdash --help
```

## Create a workspace

```bash
devdash workspace add mqms
```

With no path argument, this creates and registers `~/devdash/mqms`. See [Workspaces](workspaces.md) for explicit paths and lifecycle behavior.

## Run guided setup

```bash
devdash workspace setup mqms
```

Setup chooses a GitHub or GitHub Enterprise host, validates `gh` authentication, chooses a personal login or organization, refreshes repository metadata, and optionally clones selected repositories. Devdash does not install `gh` or authenticate for you.

For GitHub.com authentication:

```bash
gh auth login
```

For GitHub Enterprise Server:

```bash
gh auth login --hostname git.example.com
```

Canceling a required host or owner prints `Workspace setup cancelled.` and succeeds. A selected host may already have been saved before later owner cancellation. Canceling or leaving repository selection empty, or declining clone confirmation, completes setup without cloning. See [GitHub Integration](integrations/github.md).

## Check readiness

```bash
devdash workspace check mqms
```

The check is non-mutating. It validates the workspace directory, effective GitHub configuration, `gh` availability and authentication, and cached checkout state. It does not refresh repository discovery. Status is `ready`, `incomplete`, or `degraded`; missing `github.org` takes precedence as `incomplete`.

## Select and clone repositories

After setup, inspect the cached snapshot and choose additional repositories:

```bash
devdash repo list mqms
devdash repo pick mqms
```

Repositories clone to `~/devdash/mqms/repos/<repo>` by default. Devdash inspects tracked or exact expected destinations; it does not scan the workspace for arbitrary Git repositories. See [Repositories](repositories.md) for clone safety and state meanings.

## Manual alternative

Set the owner and clone a repository without guided setup:

```bash
devdash workspace config set mqms github.org example-org
devdash repo clone mqms repo-a
```

`repo clone` refreshes discovery before resolving the selector. Configure `github.base_url` first when using GitHub Enterprise. See [Workspace Configuration](configuration.md).

## Inspect repository state

```bash
devdash repo list mqms
```

The list reports `not-cloned`, `cloned`, `missing`, or `invalid`, derived from registered checkout paths at read time. A missing checkout can be restored with `repo clone`. Devdash refuses to overwrite an invalid checkout or conflicting exact destination; move or repair the conflicting path before retrying.

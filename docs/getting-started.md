# Getting Started

This guide owns the first Devdash workflow: install the CLI, create a workspace, configure GitHub and/or Confluence, inspect readiness, and materialize selected resources.

## Requirements

Released binaries support macOS and Linux on amd64 and arm64. Repository workflows require `git`; GitHub setup and discovery require an installed, externally authenticated `gh`. Confluence workflows require a Data Center REST v1 endpoint and Bearer PAT. Interactive selection uses `fzf` when available and otherwise uses numbered prompts.

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

Setup first lets you select GitHub and/or Confluence. GitHub setup chooses a GitHub.com or GitHub Enterprise host, validates `gh`, selects an owner, refreshes repository metadata, and optionally clones repositories. Confluence setup prompts for the Data Center base URL, space key, hidden PAT, and optional root page, then validates access before storing configuration or a replacement PAT.

For GitHub.com authentication:

```bash
gh auth login
```

For GitHub Enterprise Server:

```bash
gh auth login --hostname git.example.com
```

Canceling or leaving the top-level provider selection empty prints `Workspace setup cancelled.` and changes nothing. GitHub host or owner cancellation retains its existing behavior; repository selection may be skipped. See [GitHub Integration](integrations/github.md) and [Confluence Integration](integrations/confluence.md).

## Check readiness

```bash
devdash workspace check mqms
```

The check is non-mutating and non-discovering. It validates the workspace directory and each configured provider independently, then derives cached repository and wiki state. An unconfigured provider is optional. Missing required keys in an active namespace are `incomplete`; invalid configuration, missing secrets, authentication failures, or inspection failures are `degraded`.

## Select and clone repositories

After setup, inspect the cached snapshot and choose additional repositories:

```bash
devdash repo list mqms
devdash repo pick mqms
```

Repositories clone to `~/devdash/mqms/repos/<repo>` by default. Devdash inspects tracked or exact expected destinations; it does not scan the workspace for arbitrary Git repositories. See [Repositories](repositories.md) for clone safety and state meanings.

## Refresh and fetch wiki pages

Refresh stores metadata only; it does not download page bodies:

```bash
devdash wiki refresh mqms
devdash wiki list mqms
devdash wiki fetch mqms 123456
devdash wiki fetch mqms "Architecture Overview"
```

Selectors resolve exact page ID first, then an exact case-sensitive title when unique. Use `--all` explicitly to materialize every discovered page. Files are generated beneath `<workspace>/wiki`; see [Confluence Integration](integrations/confluence.md).

## Manual alternative

Set the owner and clone a repository without guided setup:

```bash
devdash workspace config set mqms github.org example-org
devdash repo clone mqms repo-a
```

`repo clone` refreshes discovery before resolving the selector. Configure `github.base_url` first when using GitHub Enterprise. See [Workspace Configuration](configuration.md).

Configure Confluence manually:

```bash
printf %s 'confluence-pat' | devdash secret set confluence.pat
devdash workspace config set mqms confluence.base_url https://wiki.example.com/confluence
devdash workspace config set mqms confluence.space MQMS
devdash workspace config set mqms confluence.secret secret:confluence.pat
devdash wiki refresh mqms
```

The PAT is stored as application-readable SQLite secret data; only its `secret:<key>` reference is stored in workspace configuration.

## Inspect repository state

```bash
devdash repo list mqms
```

The list reports `not-cloned`, `cloned`, `missing`, or `invalid`, derived from registered checkout paths at read time. A missing checkout can be restored with `repo clone`. Devdash refuses to overwrite an invalid checkout or conflicting exact destination; move or repair the conflicting path before retrying.

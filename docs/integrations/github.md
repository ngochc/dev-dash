# GitHub Integration

GitHub is the only implemented remote provider. It uses the installed GitHub CLI rather than embedding an OAuth or API transport.

## Configuration

| Key | Requirement | Meaning |
| --- | --- | --- |
| `github.base_url` | Optional; default `https://github.com` | GitHub.com or GitHub Enterprise Server base URL |
| `github.org` | Required | Repository owner; either the authenticated personal login or an organization |

Host resolution trims surrounding whitespace, applies the default when empty, and requires an HTTP or HTTPS URL with a hostname. It removes trailing slashes from the effective base URL; guided setup stores that normalized value. A generic manual config set stores its trimmed input and normalization occurs when GitHub resolves it.

The command host is lowercase `url.Hostname()`, so a URL port is excluded. For example, `https://git.example.com` resolves to command host `git.example.com`. No validation beyond these implemented URL checks is implied. See [Workspace Configuration](../configuration.md).

## External `gh` boundary

Devdash executes these GitHub CLI operations:

- authentication: `gh auth status --hostname <host>`;
- personal login: `gh api user`;
- organizations: `gh api --paginate user/orgs`;
- repositories: `gh repo list <owner> --limit 1000 --no-archived --json <fields>`;
- cloning: `gh repo clone <owner/repository> <destination>`.

Authenticate GitHub.com with:

```bash
gh auth login
```

Authenticate GitHub Enterprise Server with:

```bash
gh auth login --hostname git.example.com
```

Devdash never authenticates for the user and never changes global `gh` configuration. For a non-`github.com` host it supplies process-local `GH_HOST` to each command.

If `gh` is missing, install it from <https://cli.github.com/> and rerun setup. If authentication fails, run the matching `gh auth login` command and retry.

## Command behavior

- `workspace setup` interactively selects and stores the host and owner, refreshes repositories, then optionally clones selected repositories.
- `workspace check` resolves configuration, validates the workspace root, `gh`, authentication, and cached checkout inspection without changing configuration or refreshing discovery.
- `repo refresh` validates effective configuration and authentication, discovers repositories, and additively upserts the cached resource snapshot.
- `repo list` reads cached repositories and derives checkout state through targeted `git` inspection; it does not call GitHub or refresh.
- `repo clone` performs a refresh before selector resolution, then clones, adopts, restores, or refuses each selected destination conservatively.
- `repo pick` refreshes once before selection and uses the known selection for cloning.

See [Repositories](../repositories.md) for resource mapping, selectors, states, and clone safety.

## Guided setup order and cancellation

Setup performs these steps:

1. Resolve and print the workspace.
2. Keep or select a GitHub host; blank Enter uses `github.com` when selection is required.
3. Save the selected host, then validate `gh` availability and authentication.
4. Discover the personal login and organizations; keep, select, or enter an owner. Blank Enter uses the authenticated personal login when one is available.
5. Save the owner and refresh repository discovery.
6. Select repositories and optionally confirm cloning.
7. Print completion counts and the next list command.

Host selection may therefore be persisted before a later owner cancellation or failure. Esc or EOF at a required host or owner selection prints `Workspace setup cancelled.` and succeeds; blank Enter also cancels when that prompt has no default. A canceled or empty repository selection, or declined clone confirmation, still completes setup without cloning.

## Readiness status

`workspace check` reports:

- `ready`: all checked requirements pass; exit zero.
- `incomplete`: `github.org` is missing; this takes precedence over degradation and exits nonzero.
- `degraded`: bad workspace root, invalid base URL, unavailable `gh`, failed authentication, or repository inspection errors; exits nonzero.

Cached `missing` and `invalid` repository counts alone are informational and do not make readiness degraded.

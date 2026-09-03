# Repositories

Repository workflows discover GitHub repositories, map them into the provider-neutral resource model, optionally register local checkouts, and derive current checkout state.

## Lifecycle

1. The [GitHub integration](integrations/github.md) resolves effective workspace configuration and validates `gh` authentication.
2. `gh` discovers remote repositories.
3. SQLite upserts the GitHub integration and `resources` with `type = 'repository'`.
4. `workspace_resources` associates each logical repository with the workspace.
5. Successful clone, restore, or exact-destination adoption registers a `resource_locations` row with `location_type = 'local_checkout'`.
6. Reads inspect that tracked path and derive state; state is never persisted.

Resources exist independently of workspaces and can be shared by membership. See [Workspaces](workspaces.md) for local roots and ownership.

## Commands

```bash
devdash repo refresh mqms
devdash repo list mqms
devdash repo clone mqms repo-a repo-b
devdash repo clone mqms --all
devdash repo pick mqms
```

Refresh requests at most 1000 non-archived repositories. It adds or updates resources and workspace memberships. It does not prune repositories that are absent from a later GitHub response.

A direct `repo clone` refreshes discovery before selector resolution. `repo pick` refreshes once, displays known repositories, and clones the selected set without a second discovery refresh. A selector is an exact `owner/name` or a unique short name; ambiguous short names must use `owner/name`.

## Derived state

`repository.DeriveState` defines four states:

| State | Exact meaning |
| --- | --- |
| `not-cloned` | No checkout path is registered. |
| `cloned` | The registered path exists and is a Git checkout whose top-level directory and normalized `origin` match the expected repository. |
| `missing` | A checkout path is registered but does not exist. |
| `invalid` | The registered path exists but is not the expected repository. |

An inspection error is returned rather than converted into one of these states. Devdash checks only tracked paths and exact expected destinations. It never scans a workspace for arbitrary Git repositories.

## Clone safety

The default untracked destination is `<workspace.local_path>/repos/<repo>`. Clone processing is independent per selected repository:

- A valid checkout at an untracked exact destination is adopted and registered.
- A valid tracked checkout is skipped as already cloned.
- A missing tracked path is restored at that same tracked path.
- An invalid tracked path or conflicting exact destination is never overwritten.
- Existing checkouts are never fetched, pulled, reset, or updated.
- Failures are reported per repository; successful operations remain committed.
- A failed `gh repo clone` may leave an unregistered partial destination. Devdash reports that path instead of deleting it.

To recover a `missing` checkout, run `repo clone` with its exact `owner/name`. For `invalid` or conflicting state, move the path aside or repair its Git origin manually before retrying. Devdash will not replace it.

## Direct clone trace

For `devdash repo clone mqms owner/repo-a`, current behavior is:

1. Resolve `mqms`, load the stored `github` namespace, apply the default base URL, validate required configuration, and run `gh auth status`.
2. Discover repositories and transactionally upsert the integration, resources, and memberships. This refresh is additive and non-pruning.
3. Resolve the exact `owner/repo-a` selector from the refreshed workspace snapshot.
4. Derive state from its tracked path. If untracked, inspect only `<workspace.local_path>/repos/repo-a`.
5. Skip a valid tracked checkout, adopt a valid exact destination, restore a missing tracked path, or refuse an invalid/conflicting path.
6. Otherwise ensure the parent and run `gh repo clone owner/repo-a <destination>`.
7. Reinspect the destination. Only a valid expected checkout is transactionally registered as `local_checkout` and reported `cloned` or `restored`.
8. If clone fails after creating a destination, retain it unregistered and report the partial path.

# Workspaces

A workspace is a named local project context. SQLite stores an opaque workspace ID, a globally unique name, and `local_path`. Commands that accept `<name-or-id>` try exact ID first, then exact name. Lists are ordered by name.

## Lifecycle commands

```text
devdash workspace list
devdash workspace add <name> [path]
devdash workspace show <name-or-id>
devdash workspace remove <name-or-id>
```

Create the default `mqms` workspace:

```bash
devdash workspace add mqms
```

With no explicit path, Devdash creates and registers `~/devdash/mqms`. A default workspace name must be one path component and cannot escape `~/devdash`.

Register an existing directory instead:

```bash
devdash workspace add mqms /work/mqms
devdash workspace show mqms
devdash workspace list
```

An explicit path is resolved to an absolute cleaned path and must already be a directory. Devdash does not create an explicit path.

Remove the registry record:

```bash
devdash workspace remove mqms
```

Removal cascades workspace-owned database rows such as configuration, memberships, workspace-owned resource locations, and workspace-scoped aliases. It does not delete the filesystem directory or independent resources.

## Current layout

```text
~/devdash/mqms/
└── repos/
```

Repositories materialize under `<workspace.local_path>/repos/<repo>`. The `repos/` directory is created on demand during cloning, not by `workspace add`.

## Planned layout

```text
~/devdash/mqms/
├── repos/
├── wiki/
└── artifacts/
```

`wiki/` and `artifacts/` materialization are planned. Those directories and behaviors are not implemented.

## Ownership

`workspace_config` rows belong to one workspace. `workspace_resources` records membership, but a resource remains an independent logical identity and may belong to multiple workspaces. Repository checkout locations may also carry workspace ownership.

# Workspace Configuration

Workspace configuration is open-ended namespaced string storage. Each row belongs to one workspace and represents `namespace.key=value`.

## Key and value rules

Keys split on the first dot:

- namespace: `^[A-Za-z0-9_][A-Za-z0-9_-]*$`
- remaining key: `^[A-Za-z0-9][A-Za-z0-9._-]*$`

Matching is case-sensitive. Values are trimmed before storage and must be nonempty and single-line. List and edit output is sorted by namespace and key.

Generic set and edit operations accept undeclared namespaces and do not enforce integration definitions.

## Definitions and stored values

Integration code owns configuration definitions. A definition exposes a name, whether it is required, an optional default, and a description. `devdash config keys [provider]` discovers those definitions.

Definitions do not seed `workspace_config`. A row exists only after a value is stored. A known key therefore needs no seed row, a required value may still be absent, and defaults are applied only while resolving effective integration configuration.

Current GitHub definitions and example values are:

```text
github.base_url=https://github.com
github.org=example-org
```

`github.base_url` is optional and defaults to the shown value. `github.org` is required by GitHub operations. See [GitHub Integration](integrations/github.md).

### Planned integration examples

Generic configuration storage accepts these strings today:

```text
jira.base_url=https://jira.example.com
jira.project=MQMS
confluence.base_url=https://wiki.example.com
confluence.space=MQMS
```

Jira and Confluence are planned. No current adapter defines, validates, or consumes these values. Generic storage also does not automatically resolve `jira.secret` or `confluence.secret`; see [Secrets](secrets.md).

## Commands

Definition discovery:

```text
devdash config keys
devdash config keys github
```

Workspace value operations:

```text
devdash workspace config list <workspace>
devdash workspace config get <workspace> <key>
devdash workspace config set <workspace> <key> <value>
devdash workspace config unset <workspace> <key>
devdash workspace config edit <workspace>
```

Concrete examples:

```bash
devdash workspace config set mqms github.org example-org
devdash workspace config get mqms github.org
devdash workspace config list mqms
devdash workspace config unset mqms github.base_url
EDITOR="code --wait" devdash workspace config edit mqms
```

`workspace config get` prints the raw stored value. `config keys` is discovery, not an allowlist for generic set/edit. Edit reads `key=value` lines through `$VISUAL`, then `$EDITOR`; blank lines and lines beginning with `#` are ignored. Replacement is transactional, so validation or persistence failure leaves prior values intact.

## Runtime validation

Integration commands follow:

```text
command -> load stored namespace -> apply defaults -> validate required keys/URL -> validate external tool/authentication -> execute
```

For example, a GitHub operation with no owner reports:

```text
GitHub configuration is incomplete for workspace "mqms".

Missing:
  github.org

Configure it with:
  devdash workspace config set mqms github.org <organization>

Show supported GitHub keys:
  devdash config keys github
```

Generic storage does not perform this integration-specific runtime validation.

## Reserved internal configuration

A namespace beginning with `_` is Devdash-owned.

User set, unset, and edit operations reject reserved keys. Normal list and edit views hide them. Transactional user replacement preserves existing reserved rows. `ConfigService.SetInternal` and internal services may write them.

Illustrative reserved names are `_repo.last_refresh`, `_github.last_refresh`, and `_wiki.last_fetch`. They are conventions only; the current runtime does not persist those refresh markers.

User-operational keys remain editable even when their integrations are future work. Examples include `github.org`, `jira.project`, `jira.secret`, and `confluence.secret`.

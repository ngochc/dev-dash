# Integrations

Integrations translate provider behavior into the provider-neutral resource model. The generic lifecycle is:

```text
define config -> resolve effective workspace config -> validate -> discover remote objects -> upsert provider-neutral resources -> associate workspace -> materialize locally when applicable
```

Integration code owns configuration definitions, default and required-value resolution, provider validation, and external behavior. Workspaces own stored values. `integrations` rows identify provider instances used by provider-backed resources; credential references remain separate from resource identity.

## Current: GitHub

[GitHub](integrations/github.md) is the only implemented provider workflow. It uses `gh` for external authentication, owner and repository discovery, and cloning. Discovery is mapped to provider-neutral repository resources and workspace memberships; valid local checkouts are registered as resource locations.

## Planned

Jira and Confluence are empty product placeholders: no current provider directories or files implement commands, configuration definitions, validation, discovery, materialization, or secret-reference resolution. Their integration behavior is planned.

Use the [Extension Guide](extension-guide.md) for the current registration and boundary rules. Adding an integration directory alone would not register provider behavior.

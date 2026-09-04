# Integrations

Integrations translate provider behavior into the provider-neutral resource model. The generic lifecycle is:

```text
define config -> resolve effective workspace config -> validate -> discover remote objects -> upsert provider-neutral resources -> associate workspace -> materialize locally when applicable
```

Integration code owns configuration definitions, default and required-value resolution, provider validation, and external behavior. Workspaces own stored values. `integrations` rows identify provider instances used by provider-backed resources; credential references remain separate from resource identity.

## Current providers

[GitHub](integrations/github.md) uses `gh` for authentication, owner and repository discovery, and cloning. Discovery maps to provider-neutral `repository` resources and `local_checkout` locations.

[Confluence](integrations/confluence.md) targets Data Center REST v1 with Bearer PAT authentication. Metadata-only refresh maps pages to provider-neutral `confluence_page` resources; explicit fetch commands create generated Markdown and `materialized_file` locations.

Provider namespaces are independent. An unconfigured provider is optional during `workspace check`; GitHub commands resolve only GitHub configuration and wiki remote commands resolve only Confluence configuration.

## Planned

Jira provider behavior remains planned. Relations, attachments, Confluence write-back, stale detection, hierarchy reconstruction, Cloud v2 support, background jobs, and AI features are not implemented.

Use the [Extension Guide](extension-guide.md) for the current registration and boundary rules. Adding an integration directory alone would not register provider behavior.

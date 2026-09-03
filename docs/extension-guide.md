# Extension Guide

Extend Devdash through explicit provider boundaries while keeping core identity and orchestration provider-neutral. Review [Architecture](architecture.md), [Integrations](integrations.md), and the [Data Model](data-model.md) before changing a boundary.

## Provider checklist

1. **Define configuration.** Add `configdef.Definition` values for name, required/default behavior, and description; register them in `internal/app.configDefinitions`.
2. **Resolve effective values.** Load the provider namespace at runtime, apply defaults, check required values, and return provider-specific configuration errors before external work.
3. **Validate the boundary.** Check required tools, URLs, credentials or authentication, and provider prerequisites explicitly.
4. **Register vocabulary only when needed.** Add resource types or relation types before persisting objects that use them. Do not register speculative types.
5. **Implement the provider client.** Put external API or command execution under `internal/integration/<provider>` with `context.Context` and testable interfaces.
6. **Wire the use case.** Compose dependencies in `internal/app`; add a storage adapter only where the behavior requires persistence. Files under an integration directory do not register a provider by themselves.
7. **Map discovery.** Convert provider objects into provider-neutral resources with opaque Devdash IDs, provider-native identity in `provider_id` or `external_key`, and provider details only where needed.
8. **Associate workspaces.** Use `workspace_resources` rather than putting workspace identity on the logical resource.
9. **Register materialization.** Use `resource_locations` for local physical instances and give workspace ownership when the instance is workspace-specific.
10. **Prove observable behavior.** Add focused service/client fakes and tests for success, validation, partial failure, and persistence invariants; update the authoritative user and contributor documentation.

Do not force `repository.Store` onto providers that materialize non-repository resource types. Define the smallest boundary that matches the provider's actual objects and operations.

## Schema extension priority

Use this order exactly:

1. resource type;
2. relation type;
3. metadata;
4. provider-specific extension table keyed by `resources.id`;
5. core schema change only for a universal concept.

A frequently queried or constrained provider attribute may justify an extension table. Provider-specific columns in `resources` do not.

## Engineering rules

- Pass dependencies and `context.Context` explicitly; do not introduce global mutable state.
- Keep provider names and clients out of core packages.
- Use opaque string IDs. Keep stable provider identity separate from mutable names and URLs.
- Store credential references, never credentials, in integration configuration.
- Add a new numbered migration; never rewrite an applied migration.
- Update `updated_at` explicitly when mutating rows.
- Use transactions for writes spanning integrations, resources, memberships, locations, or other related records.
- Preserve causes with `%w` and add operation context to errors.
- Validate provider failures independently where partial success is an observable contract.

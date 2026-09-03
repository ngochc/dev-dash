# ADR 0004: Workspace Configuration

## Status

Accepted

## Context

Workspaces need provider and operational settings that can grow without adding core columns for every integration. Values also need independent updates and namespace lookup.

## Decision

Store configuration as normalized `workspace_config(workspace_id, namespace, key, value)` rows rather than provider columns or one JSON blob.

## Consequences

Integrations can add keys without a schema migration, values can be updated independently, and namespaces can be loaded efficiently. Definitions, defaults, required-key rules, and validation remain in code; stored values remain strings. See [Workspace Configuration](../configuration.md).

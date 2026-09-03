# ADR 0005: Reserved Internal Configuration

## Status

Accepted

## Context

Implementation-owned workspace state and user-operational configuration share namespaced storage but require distinct ownership and editing rules.

## Decision

Reserve namespaces beginning with `_` for Devdash-owned state.

## Consequences

User APIs reject and hide internal keys, user replacement preserves them, and internal services write them through `SetInternal`. Names such as `_repo.last_refresh` are conventions, not evidence that current refresh markers are persisted. See [Workspace Configuration](../configuration.md).

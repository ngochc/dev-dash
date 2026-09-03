# ADR 0003: Universal Resource Model

## Status

Accepted

## Context

Repositories, tickets, documents, builds, and future provider objects need shared identity, workspace membership, physical instances, and graph relationships without provider-specific core tables for every object.

## Decision

Use `resource_types`, `resources`, `workspace_resources`, and `resource_locations` as the core model instead of provider-specific identity tables.

## Consequences

The model supports a shared graph, extensible vocabulary, many-to-many workspace membership, and separation of logical identity from physical instances. Every provider must map native objects into the generic model. Provider details stay in metadata or provider extension tables only when justified. See [Data Model](../data-model.md).

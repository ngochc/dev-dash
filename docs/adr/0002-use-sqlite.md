# ADR 0002: Use SQLite

## Status

Accepted

## Context

Devdash needs durable local graph and workspace state with transactions and referential integrity, but requiring a separately operated database service would conflict with local CLI use.

## Decision

Use SQLite as the local persistence adapter, with foreign keys enabled and no external database service.

## Consequences

The database is easy to back up and inspect. Goose migrations are embedded, WAL is enabled, and each process caps the connection pool at one. The database and WAL/SHM sidecars are sensitive because secrets are application-readable. Persistence code remains SQLite-specific behind repository interfaces. See [Database](../database.md).

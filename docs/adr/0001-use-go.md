# ADR 0001: Use Go

## Status

Accepted

## Context

Devdash is a local CLI distributed across supported macOS and Linux architectures. It needs predictable deployment, filesystem and subprocess support, and maintainable provider and persistence boundaries without requiring a runtime service.

## Decision

Use Go. It produces a single deployable binary, provides a strong standard library, supports straightforward cross-compilation, and leaves concurrency available when a use case justifies it.

## Consequences

Contributors need the project-managed Go toolchain through `asdf`. The release workflow can produce static platform binaries. Implementation should remain synchronous and simple until concurrency has explicit ownership, cancellation, and shutdown behavior. See [Development](../development.md).

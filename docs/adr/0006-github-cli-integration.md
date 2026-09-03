# ADR 0006: Use GitHub CLI Integration

## Status

Accepted

## Context

Initial GitHub.com and GitHub Enterprise behavior needs authentication, owner and repository discovery, and cloning. Implementing and maintaining an OAuth flow and API transport would expand the first provider boundary substantially.

## Decision

Use the installed `gh` CLI for authentication validation, discovery, and cloning.

## Consequences

Devdash reuses the user's GitHub.com or GHES authentication and reduces internal transport scope. It requires a compatible installed `gh`, depends on command and JSON behavior, and must surface external-command failures. Authentication and global `gh` configuration remain outside Devdash. See [GitHub Integration](../integrations/github.md).

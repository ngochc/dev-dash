# Confluence Integration

Devdash supports Confluence Data Center through REST API v1. Authentication is a Data Center personal access token sent as `Authorization: Bearer <PAT>`. Atlassian Cloud v2, OAuth, basic authentication, cookies, attachments, comments, write-back, stale detection, hierarchy reconstruction, relations, background jobs, and AI features are not implemented.

## Configuration

| Key | Requirement | Meaning |
| --- | --- | --- |
| `confluence.base_url` | Required | Data Center base URL, including any context path |
| `confluence.space` | Required | Exact Confluence space key |
| `confluence.secret` | Required | `secret:<key>` reference containing the PAT |
| `confluence.auth_type` | Optional; default `pat` | Authentication mode; only exact `pat` is supported |
| `confluence.root_page` | Optional | Decimal page ID limiting discovery to the root and its descendants |

The base URL must be an absolute HTTP or HTTPS URL with a host and no userinfo, query, or fragment. Resolution trims trailing slashes while preserving context paths such as `https://wiki.example.com/confluence`. The full normalized URL identifies the provider instance, so two context paths on one host do not collide.

Definitions are code-owned and do not seed workspace rows:

```bash
devdash config keys confluence
```

## Guided setup

```bash
devdash workspace add mqms
devdash workspace setup mqms
devdash workspace check mqms
```

At provider selection, choose Confluence. Setup prompts for URL and space, reads the PAT without echo, optionally accepts a root page ID, and validates the Data Center endpoint, PAT, space, and root before storing a new or replacement PAT or any Confluence configuration. Existing valid secrets can be reused. The default secret key is `confluence.pat`.

An empty top-level provider selection cancels without changing either provider. GitHub and Confluence are configured independently and selected providers are processed in GitHub-then-Confluence order.

## Manual setup

```bash
printf %s 'confluence-pat' | devdash secret set confluence.pat
devdash workspace config set mqms confluence.base_url https://wiki.example.com/confluence
devdash workspace config set mqms confluence.space MQMS
devdash workspace config set mqms confluence.secret secret:confluence.pat
```

Optionally limit discovery:

```bash
devdash workspace config set mqms confluence.root_page 123456
```

Secret values are application-readable and unencrypted in SQLite. Workspace configuration stores only the reference. Devdash does not put the PAT in resources, generated files, logs, or errors.

## Refresh, list, and fetch

```bash
devdash wiki refresh mqms
devdash wiki list mqms
devdash wiki fetch mqms 123456
devdash wiki fetch mqms "Architecture Overview"
devdash wiki fetch mqms --all
```

`wiki refresh` validates remotely and pages through current page metadata with `expand=space,version`. It does not request `body.storage`, create files, prune unseen pages, or create locations. With `confluence.root_page`, discovery includes the root and its descendant pages.

`wiki list` is offline. It reads cached generic resources and registered file locations without calling Confluence.

Every fetch performs metadata refresh first. A selector resolves an exact page ID before an exact case-sensitive title; a title must be unique. Selector order is preserved and duplicate resources are fetched once. `--all` must be explicit and is the sole selector; all-page order is title then page ID. Independent page failures do not stop successful pages, and the command prints every result before returning an aggregate failure.

## Generated Markdown

The first fetch creates a flat filename:

```text
<wiki-root>/<unicode-slug>-<page-id>.md
```

The slug lowercases Unicode letters and digits, collapses punctuation and whitespace to `-`, and cannot contain traversal components. Storage XHTML is converted to readable Markdown for headings, paragraphs, breaks, lists, links, inline code, preformatted blocks, Confluence code macros, and basic tables. Unsupported elements retain readable text.

Front matter is emitted in this order:

```yaml
---
devdash_resource_id: "<opaque-resource-id>"
confluence_page_id: "123456"
confluence_space: "MQMS"
source_url: "https://wiki.example.com/..."
title: "Architecture Overview"
confluence_updated_at: "2026-09-04T00:00:00.000Z"
---
```

`confluence_updated_at` is omitted when the server supplies no version timestamp. Values are quoted. Local content is generated and may be overwritten.

## State and path safety

List state is derived from the registered location:

- `not-fetched`: no location is registered;
- `fetched`: the tracked path exists as a regular file;
- `missing`: a tracked regular file is absent and can be restored by fetching again.

A first fetch refuses an existing destination and never scans or adopts arbitrary Markdown. After registration, refetch retains the tracked filename even if the remote title changes and overwrites only that direct child of the workspace `wiki` directory. A tracked symlink, non-regular file, relative path, or path outside that directory is rejected. The `wiki` root itself must be a real directory. Writes use a same-directory temporary file and atomic rename with mode `0644`; a new file is removed if location registration fails.

## Generic resource mapping

No `confluence_pages` table exists. Each page uses:

- integration provider: `confluence`;
- integration name and base URL: normalized full Data Center base URL;
- resource type: `confluence_page`;
- provider ID: decimal page ID;
- external key: `<space>/<page-id>`;
- name and URL: current page title and source URL;
- metadata: only `confluence_updated_at` when supplied;
- location type: `materialized_file`.

Provider-ID identity keeps the opaque resource ID stable across title and URL changes. Refresh is additive, and the same provider page can belong to multiple workspaces.

## Readiness and provider isolation

`workspace check` treats an empty Confluence namespace as `not configured` and makes no Confluence call. For an active namespace, missing required keys are `incomplete`; invalid values, missing referenced secrets, authentication or authorization failures, endpoint errors, and local inspection errors are `degraded`. Cached missing files are informational. GitHub commands never resolve Confluence configuration, and wiki remote commands never resolve GitHub configuration.

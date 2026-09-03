-- +goose Up

PRAGMA foreign_keys = ON;


-- ============================================================
-- 1. WORKSPACES
-- ============================================================

CREATE TABLE workspaces (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    local_path TEXT,

    metadata TEXT CHECK (
        metadata IS NULL OR json_valid(metadata)
    ),

    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);


-- ============================================================
-- 2. INTEGRATIONS
--
-- Configured provider instances.
--
-- Examples:
--   local-git
--   github
--   github-enterprise
--   gitlab
--   jira
--   confluence
--   jenkins
--   slack
-- ============================================================

CREATE TABLE integrations (
    id TEXT PRIMARY KEY,

    provider TEXT NOT NULL,
    name TEXT NOT NULL,

    base_url TEXT,

    -- Reference only. Never store credentials directly here.
    --
    -- Examples:
    --   keychain:github-work
    --   env:JIRA_TOKEN
    credential_ref TEXT,

    config TEXT CHECK (
        config IS NULL OR json_valid(config)
    ),

    enabled BOOLEAN NOT NULL DEFAULT 1
        CHECK (enabled IN (0, 1)),

    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    UNIQUE(provider, name)
);

CREATE INDEX idx_integrations_provider
    ON integrations(provider);


-- ============================================================
-- 3. RESOURCE TYPE REGISTRY
--
-- Extensible resource types.
--
-- Examples:
--   repository
--   git_branch
--   jira_issue
--   confluence_page
--   github_pr
--   build
--   deployment
-- ============================================================

CREATE TABLE resource_types (
    name TEXT PRIMARY KEY,

    display_name TEXT NOT NULL,
    owner TEXT,
    description TEXT,

    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);


-- ============================================================
-- 4. RESOURCES
--
-- Universal identity for anything participating in project
-- context.
-- ============================================================

CREATE TABLE resources (
    id TEXT PRIMARY KEY,

    type TEXT NOT NULL
        REFERENCES resource_types(name),

    integration_id TEXT
        REFERENCES integrations(id)
        ON DELETE SET NULL,

    -- Immutable provider-native ID when available.
    provider_id TEXT,

    -- Human-readable provider-native identity.
    --
    -- Examples:
    --   org/frontend
    --   PROJ-842
    --   feature/login
    --   123
    external_key TEXT,

    name TEXT,
    url TEXT,

    metadata TEXT CHECK (
        metadata IS NULL OR json_valid(metadata)
    ),

    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen_at DATETIME
);

CREATE UNIQUE INDEX idx_resource_provider_id
    ON resources(integration_id, type, provider_id)
    WHERE provider_id IS NOT NULL;

CREATE INDEX idx_resources_type
    ON resources(type);

CREATE INDEX idx_resources_integration
    ON resources(integration_id);

CREATE INDEX idx_resources_external_key
    ON resources(type, external_key);


-- ============================================================
-- 5. WORKSPACE <-> RESOURCE MEMBERSHIP
--
-- Resources exist independently of workspaces.
-- A resource may belong to multiple workspaces.
-- ============================================================

CREATE TABLE workspace_resources (
    workspace_id TEXT NOT NULL
        REFERENCES workspaces(id)
        ON DELETE CASCADE,

    resource_id TEXT NOT NULL
        REFERENCES resources(id)
        ON DELETE CASCADE,

    -- Optional semantic membership:
    --   primary
    --   dependency
    --   related
    --   documentation
    role TEXT,

    metadata TEXT CHECK (
        metadata IS NULL OR json_valid(metadata)
    ),

    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (
        workspace_id,
        resource_id
    )
);

CREATE INDEX idx_workspace_resources_resource
    ON workspace_resources(resource_id);


-- ============================================================
-- 6. RESOURCE LOCATIONS
--
-- Physical/local representation of a logical resource.
--
-- Examples:
--   local_path
--   git_worktree
--   artifact
--   cache
--   generated_file
-- ============================================================

CREATE TABLE resource_locations (
    id TEXT PRIMARY KEY,

    resource_id TEXT NOT NULL
        REFERENCES resources(id)
        ON DELETE CASCADE,

    workspace_id TEXT
        REFERENCES workspaces(id)
        ON DELETE CASCADE,

    location_type TEXT NOT NULL,
    path TEXT NOT NULL,

    metadata TEXT CHECK (
        metadata IS NULL OR json_valid(metadata)
    ),

    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    UNIQUE(location_type, path)
);

CREATE INDEX idx_resource_locations_resource
    ON resource_locations(resource_id);

CREATE INDEX idx_resource_locations_workspace
    ON resource_locations(workspace_id);


-- ============================================================
-- 7. ALIASES
--
-- Semantic names for humans and AI agents.
--
-- workspace_id = NULL means global alias.
-- ============================================================

CREATE TABLE aliases (
    id TEXT PRIMARY KEY,

    name TEXT NOT NULL,

    workspace_id TEXT
        REFERENCES workspaces(id)
        ON DELETE CASCADE,

    resource_id TEXT NOT NULL
        REFERENCES resources(id)
        ON DELETE CASCADE,

    is_primary BOOLEAN NOT NULL DEFAULT 0
        CHECK (is_primary IN (0, 1)),

    metadata TEXT CHECK (
        metadata IS NULL OR json_valid(metadata)
    ),

    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Alias name unique within a workspace.
CREATE UNIQUE INDEX idx_alias_workspace_name
    ON aliases(workspace_id, name)
    WHERE workspace_id IS NOT NULL;

-- Global alias name unique globally.
CREATE UNIQUE INDEX idx_alias_global_name
    ON aliases(name)
    WHERE workspace_id IS NULL;

-- One primary alias per resource per workspace.
CREATE UNIQUE INDEX idx_alias_primary_workspace
    ON aliases(resource_id, workspace_id)
    WHERE is_primary = 1
      AND workspace_id IS NOT NULL;

-- One global primary alias per resource.
CREATE UNIQUE INDEX idx_alias_primary_global
    ON aliases(resource_id)
    WHERE is_primary = 1
      AND workspace_id IS NULL;

CREATE INDEX idx_alias_resource
    ON aliases(resource_id);


-- ============================================================
-- 8. RELATION TYPE REGISTRY
--
-- Extensible semantic relation vocabulary.
-- ============================================================

CREATE TABLE relation_types (
    name TEXT PRIMARY KEY,

    display_name TEXT,

    -- Semantic inverse:
    --
    --   fixes       -> fixed_by
    --   depends_on  -> dependency_of
    inverse_name TEXT,

    symmetric BOOLEAN NOT NULL DEFAULT 0
        CHECK (symmetric IN (0, 1)),

    owner TEXT,
    description TEXT,

    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);


-- ============================================================
-- 9. RESOURCE GRAPH
-- ============================================================

CREATE TABLE relations (
    id TEXT PRIMARY KEY,

    source_id TEXT NOT NULL
        REFERENCES resources(id)
        ON DELETE CASCADE,

    target_id TEXT NOT NULL
        REFERENCES resources(id)
        ON DELETE CASCADE,

    relation_type TEXT NOT NULL
        REFERENCES relation_types(name),

    metadata TEXT CHECK (
        metadata IS NULL OR json_valid(metadata)
    ),

    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CHECK (source_id <> target_id),

    UNIQUE (
        source_id,
        target_id,
        relation_type
    )
);

CREATE INDEX idx_relations_source
    ON relations(source_id, relation_type);

CREATE INDEX idx_relations_target
    ON relations(target_id, relation_type);

CREATE INDEX idx_relations_type
    ON relations(relation_type);


-- ============================================================
-- 10. TAGS
-- ============================================================

CREATE TABLE tags (
    id TEXT PRIMARY KEY,

    name TEXT NOT NULL UNIQUE,
    description TEXT,

    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);


CREATE TABLE resource_tags (
    resource_id TEXT NOT NULL
        REFERENCES resources(id)
        ON DELETE CASCADE,

    tag_id TEXT NOT NULL
        REFERENCES tags(id)
        ON DELETE CASCADE,

    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (
        resource_id,
        tag_id
    )
);

CREATE INDEX idx_resource_tags_tag
    ON resource_tags(tag_id);


-- +goose Down

DROP TABLE IF EXISTS resource_tags;
DROP TABLE IF EXISTS tags;

DROP TABLE IF EXISTS relations;
DROP TABLE IF EXISTS relation_types;

DROP TABLE IF EXISTS aliases;

DROP TABLE IF EXISTS resource_locations;
DROP TABLE IF EXISTS workspace_resources;

DROP TABLE IF EXISTS resources;
DROP TABLE IF EXISTS resource_types;

DROP TABLE IF EXISTS integrations;
DROP TABLE IF EXISTS workspaces;

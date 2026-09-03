-- +goose Up

CREATE TABLE workspace_config (
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    namespace TEXT NOT NULL,
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (workspace_id, namespace, key)
);

CREATE INDEX idx_workspace_config_namespace
    ON workspace_config(workspace_id, namespace);

-- +goose Down

DROP INDEX IF EXISTS idx_workspace_config_namespace;
DROP TABLE IF EXISTS workspace_config;

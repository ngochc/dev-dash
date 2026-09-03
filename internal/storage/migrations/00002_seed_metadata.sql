-- +goose Up


-- ============================================================
-- RESOURCE TYPES
-- ============================================================

INSERT INTO resource_types (
    name,
    display_name,
    owner,
    description
)
VALUES
    (
        'repository',
        'Repository',
        'core',
        'Source code repository'
    ),
    (
        'external_resource',
        'External Resource',
        'core',
        'Generic external URL or external resource'
    ),

    -- Git
    (
        'git_branch',
        'Git Branch',
        'git',
        'Git branch'
    ),
    (
        'git_commit',
        'Git Commit',
        'git',
        'Git commit'
    ),
    (
        'git_tag',
        'Git Tag',
        'git',
        'Git tag'
    ),
    (
        'git_worktree',
        'Git Worktree',
        'git',
        'Local Git worktree'
    ),

    -- GitHub
    (
        'github_pull_request',
        'GitHub Pull Request',
        'github',
        'GitHub pull request'
    ),
    (
        'github_issue',
        'GitHub Issue',
        'github',
        'GitHub issue'
    ),
    (
        'github_workflow',
        'GitHub Workflow',
        'github',
        'GitHub Actions workflow'
    ),
    (
        'github_workflow_run',
        'GitHub Workflow Run',
        'github',
        'GitHub Actions workflow execution'
    ),

    -- Jira
    (
        'jira_issue',
        'Jira Issue',
        'jira',
        'Jira issue or ticket'
    ),

    -- Confluence
    (
        'confluence_page',
        'Confluence Page',
        'confluence',
        'Confluence page or document'
    ),

    -- CI/CD
    (
        'pipeline',
        'Pipeline',
        'core',
        'CI/CD pipeline'
    ),
    (
        'build',
        'Build',
        'core',
        'Build or CI execution'
    ),
    (
        'deployment',
        'Deployment',
        'core',
        'Deployment of a build or service'
    ),

    -- Application / infrastructure
    (
        'service',
        'Service',
        'core',
        'Application or infrastructure service'
    ),
    (
        'environment',
        'Environment',
        'core',
        'Runtime environment such as development, staging, or production'
    ),

    -- Documentation / artifacts
    (
        'document',
        'Document',
        'core',
        'Generic document'
    ),
    (
        'artifact',
        'Artifact',
        'core',
        'Generated or stored artifact'
    );


-- ============================================================
-- RELATION TYPES
--
-- Inverse relations describe semantics only.
-- Do not automatically create inverse rows unless graph logic
-- explicitly requires materialized inverse edges.
-- ============================================================

INSERT INTO relation_types (
    name,
    display_name,
    inverse_name,
    symmetric,
    owner,
    description
)
VALUES

    -- --------------------------------------------------------
    -- STRUCTURE
    -- --------------------------------------------------------

    (
        'belongs_to',
        'Belongs To',
        'contains',
        0,
        'core',
        'Source resource logically belongs to target resource'
    ),
    (
        'contains',
        'Contains',
        'belongs_to',
        0,
        'core',
        'Source resource logically contains target resource'
    ),

    -- --------------------------------------------------------
    -- IMPLEMENTATION
    -- --------------------------------------------------------

    (
        'implements',
        'Implements',
        'implemented_by',
        0,
        'core',
        'Source resource implements target resource'
    ),
    (
        'implemented_by',
        'Implemented By',
        'implements',
        0,
        'core',
        'Source resource is implemented by target resource'
    ),

    (
        'fixes',
        'Fixes',
        'fixed_by',
        0,
        'core',
        'Source resource fixes target resource'
    ),
    (
        'fixed_by',
        'Fixed By',
        'fixes',
        0,
        'core',
        'Source resource is fixed by target resource'
    ),

    -- --------------------------------------------------------
    -- DEPENDENCIES
    -- --------------------------------------------------------

    (
        'depends_on',
        'Depends On',
        'dependency_of',
        0,
        'core',
        'Source resource depends on target resource'
    ),
    (
        'dependency_of',
        'Dependency Of',
        'depends_on',
        0,
        'core',
        'Source resource is a dependency of target resource'
    ),

    (
        'blocks',
        'Blocks',
        'blocked_by',
        0,
        'core',
        'Source resource blocks target resource'
    ),
    (
        'blocked_by',
        'Blocked By',
        'blocks',
        0,
        'core',
        'Source resource is blocked by target resource'
    ),

    -- --------------------------------------------------------
    -- DOCUMENTATION
    -- --------------------------------------------------------

    (
        'documents',
        'Documents',
        'documented_by',
        0,
        'core',
        'Source resource documents target resource'
    ),
    (
        'documented_by',
        'Documented By',
        'documents',
        0,
        'core',
        'Source resource is documented by target resource'
    ),

    -- --------------------------------------------------------
    -- SOURCE CONTROL
    -- --------------------------------------------------------

    (
        'branch_of',
        'Branch Of',
        'has_branch',
        0,
        'git',
        'Git branch belongs to a repository'
    ),
    (
        'has_branch',
        'Has Branch',
        'branch_of',
        0,
        'git',
        'Repository contains a Git branch'
    ),

    (
        'commit_of',
        'Commit Of',
        'has_commit',
        0,
        'git',
        'Git commit belongs to a repository'
    ),
    (
        'has_commit',
        'Has Commit',
        'commit_of',
        0,
        'git',
        'Repository contains a Git commit'
    ),

    (
        'based_on',
        'Based On',
        'base_for',
        0,
        'git',
        'Source resource is based on target resource'
    ),
    (
        'base_for',
        'Base For',
        'based_on',
        0,
        'git',
        'Source resource is the base for target resource'
    ),

    -- --------------------------------------------------------
    -- REVIEW / CHANGE
    -- --------------------------------------------------------

    (
        'merges_into',
        'Merges Into',
        'merged_from',
        0,
        'core',
        'Source change merges into target resource'
    ),
    (
        'merged_from',
        'Merged From',
        'merges_into',
        0,
        'core',
        'Source resource receives changes from target resource'
    ),

    (
        'reviews',
        'Reviews',
        'reviewed_by',
        0,
        'core',
        'Source resource reviews target resource'
    ),
    (
        'reviewed_by',
        'Reviewed By',
        'reviews',
        0,
        'core',
        'Source resource is reviewed by target resource'
    ),

    -- --------------------------------------------------------
    -- CI / BUILD
    -- --------------------------------------------------------

    (
        'builds',
        'Builds',
        'built_by',
        0,
        'core',
        'Source resource builds target resource'
    ),
    (
        'built_by',
        'Built By',
        'builds',
        0,
        'core',
        'Source resource is built by target resource'
    ),

    (
        'triggered_by',
        'Triggered By',
        'triggers',
        0,
        'core',
        'Source resource was triggered by target resource'
    ),
    (
        'triggers',
        'Triggers',
        'triggered_by',
        0,
        'core',
        'Source resource triggers target resource'
    ),

    -- --------------------------------------------------------
    -- DEPLOYMENT
    -- --------------------------------------------------------

    (
        'deploys',
        'Deploys',
        'deployed_by',
        0,
        'core',
        'Source resource deploys target resource'
    ),
    (
        'deployed_by',
        'Deployed By',
        'deploys',
        0,
        'core',
        'Source resource is deployed by target resource'
    ),

    (
        'deployed_to',
        'Deployed To',
        'hosts',
        0,
        'core',
        'Source resource is deployed to target environment'
    ),
    (
        'hosts',
        'Hosts',
        'deployed_to',
        0,
        'core',
        'Source environment hosts target resource'
    ),

    -- --------------------------------------------------------
    -- GENERIC
    -- --------------------------------------------------------

    (
        'references',
        'References',
        'referenced_by',
        0,
        'core',
        'Source resource references target resource'
    ),
    (
        'referenced_by',
        'Referenced By',
        'references',
        0,
        'core',
        'Source resource is referenced by target resource'
    ),

    (
        'generated_by',
        'Generated By',
        'generates',
        0,
        'core',
        'Source resource was generated by target resource'
    ),
    (
        'generates',
        'Generates',
        'generated_by',
        0,
        'core',
        'Source resource generates target resource'
    ),

    (
        'related_to',
        'Related To',
        'related_to',
        1,
        'core',
        'Generic symmetric relationship between resources'
    );


-- +goose Down

DELETE FROM relation_types
WHERE name IN (
    'belongs_to',
    'contains',
    'implements',
    'implemented_by',
    'fixes',
    'fixed_by',
    'depends_on',
    'dependency_of',
    'blocks',
    'blocked_by',
    'documents',
    'documented_by',
    'branch_of',
    'has_branch',
    'commit_of',
    'has_commit',
    'based_on',
    'base_for',
    'merges_into',
    'merged_from',
    'reviews',
    'reviewed_by',
    'builds',
    'built_by',
    'triggered_by',
    'triggers',
    'deploys',
    'deployed_by',
    'deployed_to',
    'hosts',
    'references',
    'referenced_by',
    'generated_by',
    'generates',
    'related_to'
);

DELETE FROM resource_types
WHERE name IN (
    'repository',
    'external_resource',
    'git_branch',
    'git_commit',
    'git_tag',
    'git_worktree',
    'github_pull_request',
    'github_issue',
    'github_workflow',
    'github_workflow_run',
    'jira_issue',
    'confluence_page',
    'pipeline',
    'build',
    'deployment',
    'service',
    'environment',
    'document',
    'artifact'
);

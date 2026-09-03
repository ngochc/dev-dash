# Database

## Path resolution

Devdash resolves its SQLite path in this order:

1. `DEVDASH_DB`, when nonempty.
2. `~/.devdash/devdash.db`.

An override is passed through `filepath.Clean`; a relative `DEVDASH_DB` remains relative rather than becoming absolute.

## Opening and permissions

`sqlite.Open` creates the database parent with mode `0700` when needed. `os.MkdirAll` does not tighten permissions on an existing parent directory. The database file is created and explicitly changed to mode `0600` on every open.

Each process limits the database handle to one open connection with `SetMaxOpenConns(1)`. It then enables:

```text
foreign_keys = ON
journal_mode = WAL
busy_timeout = 5000
```

Goose migrations are embedded from `internal/storage/migrations` and applied automatically whenever `sqlite.Open` succeeds. Applied migrations must not be rewritten; add a new numbered migration.

## Filesystem state

SQLite stores registry data. Workspace directories and repository checkouts remain outside SQLite. Removing a workspace row or database does not remove those directories.

Secret values are stored as application-readable text. Treat the database and its WAL and SHM sidecars as sensitive. See [Secrets](secrets.md) for exposure rules and [Data Model](data-model.md) for tables and relationships.

## Inspection

Open and validate an isolated database:

```bash
DEVDASH_DB=/tmp/devdash-test.db devdash doctor
```

List tables with a read-only inspection command:

```bash
sqlite3 ~/.devdash/devdash.db '.tables'
```

Do not manually edit production rows. Use Devdash commands and migrations so constraints, transactions, and timestamp updates remain intact. Back up the database together with relevant `-wal` and `-shm` sidecars when they exist.

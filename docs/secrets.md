# Secrets

Devdash stores named secret values in SQLite and provides explicit raw and masked read commands.

## Commands

```text
devdash secret set <key>
devdash secret get <key>
devdash secret show <key>
devdash secret list
devdash secret delete <key>
```

Secret keys must match `^[A-Za-z0-9][A-Za-z0-9._-]*$`.

## Set

Interactive input is preferred:

```bash
devdash secret set confluence.pat
```

When standard input is a terminal, Devdash prompts with `Secret (input hidden; Enter to submit):` and reads without echo. When standard input is not a terminal, it reads every byte until EOF:

```bash
printf %s 'token' | devdash secret set confluence.pat
```

Do not use `echo` for this path: its trailing newline becomes part of the stored secret. Empty values are rejected.

## Read and delete

- `secret get` prints the raw value. Its output is sensitive.
- `secret show` prints the key and a mask. Values of eight Unicode code points or fewer are fully masked; longer values expose the first and last four with `…` between them.
- `secret list` prints alphabetically sorted keys without values.
- `secret delete` removes one key and reports an error when it does not exist.

Examples:

```bash
devdash secret show confluence.pat
devdash secret list
devdash secret get confluence.pat
devdash secret delete confluence.pat
```

## Storage and exposure

Values are stored as application-readable text in SQLite. Devdash does not encrypt them. Treat the database, WAL/SHM sidecars, backups, and raw `secret get` output as sensitive. Secret values must not appear in logs or error messages.

## Configuration references

`secret:<key>` is the agreed convention for referring to a secret from configuration:

```text
confluence.secret=secret:confluence.pat
jira.secret=secret:jira.token
```

Confluence resolves this reference at runtime and never stores the PAT in workspace configuration, resources, files, logs, or errors. Guided `workspace setup` reads a PAT without echo and validates it before storing a new or replacement value. GitHub uses external `gh` authentication; Jira reference resolution remains planned.

Values remain application-readable and unencrypted in SQLite. Possible future storage may add encryption or OS-backed secret stores; no compatibility behavior is promised.

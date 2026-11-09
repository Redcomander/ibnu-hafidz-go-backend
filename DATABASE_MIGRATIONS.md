# Database migration rollout: preserve existing data

This release replaces startup AutoMigrate and ad hoc SQL with an explicit,
versioned adoption baseline. No command has been run against your configured
database by the coding agent.

## What changes

- Normal server startup and `--check-database` only read migration metadata and
  table definitions. No schema changes, membership backfills, permission seeding,
  or type conversions run at startup.
- `cmd/migrate` defaults to read-only status. Its explicit baseline command creates
  only `app_schema_migrations` and inserts version 001, the SQL checksum, timestamp,
  and a JSON snapshot of table definitions. Existing business rows, columns and
  constraints remain untouched.
- Repeating baseline verifies history without rewriting it. Missing, unexpected,
  incomplete, or mismatched history blocks startup instead of being repaired.
- A database-scoped MySQL advisory lock prevents simultaneous baseline commands.
- cPanel checks a candidate binary before stopping the running service or replacing
  its binary. Deployment never registers the baseline automatically.

This adopts an existing schema; it is not a fresh-install initializer or a full
compatibility audit. A marker does not prove every column/type is correct. Empty
databases are rejected. MySQL metadata-read access is required.

## First rollout

Do not deploy/restart this release until the baseline is registered; otherwise
startup deliberately fails. Preserve the old binary. Use Go 1.24 or newer.

1. Pause user writes/background jobs and take a database dump plus a backup of the
   actual upload storage. Store them privately outside the repository/web root.
2. Restore both into a new isolated environment. Verify student records, historical
   attendance, laundry/billing records and uploaded files; compare counts and values.
3. Point the migration CLI at that restored database using environment variables.
   A copied production environment must not point the rehearsal at production.
   The CLI loads `.env` only for variables not already set.
4. Run the read-only status command (missing baseline is expected initially):

   ```sh
   go run ./cmd/migrate -command=status
   ```

5. Once backup restoration has been verified, register the baseline. Replace the
   database name with the exact value selected by `DB_DATABASE`:

   ```sh
   go run ./cmd/migrate -command=baseline -backup-verified -confirm-database=your_database_name
   go run ./cmd/migrate -command=status
   ```

6. Compare existing table definitions and rows before/after. Only the migration
   metadata table should differ. Exercise schedules, substitute assignments,
   memberships and `Dinas`/`Dinas Luar` statuses in the isolated application.
7. After rehearsal succeeds, take/verify a current production backup, load the
   production environment, run the explicit baseline command for that database,
   then deploy. No production operation is automatic.

The flags are operator attestations, not automatic proof of a valid backup.

## Backup and restore example

Use matching MySQL client tools and private options files for credentials rather
than command-line passwords. These are Linux/cPanel examples; replace paths and
database names. Use a new private backup directory to avoid overwriting backups.

```sh
umask 077
mysqldump --defaults-extra-file=/private/mysql-backup.cnf \
  --single-transaction --quick --hex-blob --no-tablespaces \
  --routines --events --triggers your_database_name > /private/backup/database.sql
tar -czf /private/backup/uploads.tar.gz -C /actual/uploads/path .
sha256sum /private/backup/database.sql /private/backup/uploads.tar.gz \
  > /private/backup/SHA256SUMS
```

Check every command's exit status, available disk space and dump warnings.
`--single-transaction` covers transactional InnoDB tables; nontransactional tables
need a consistent locked backup or downtime. Avoid schema changes during the dump.
Back up remote object storage separately if applicable.

An operator must create a new empty test database, then restore into that target
only. Never use the production database as a restore target:

```sh
mysql --defaults-extra-file=/private/mysql-restore-test.cnf isolated_restore_db \
  < /private/backup/database.sql
tar -xzf /private/backup/uploads.tar.gz -C /private/isolated-upload-directory
```

Inspect the dump before restoring: routines/events/triggers may have side effects.
Keep the restored environment disconnected from production WhatsApp, OCR and other
live integrations. File checksums prove integrity, not a successful recovery;
validate the restored records and files through the isolated application.

## Legacy changes intentionally not replayed

No drops of `lessons.name`, `created_by_id` or `submitted_by_id`; no time/enum type
conversions or foreign-key changes; no `kelas_siswa` membership copying; no permission
seeding/reassignment. If the current app requires a missing legacy schema feature,
diagnose it in the restored copy and prepare a separate reviewed migration. A
baseline preserves existing problems too; it does not repair or certify the schema.

## Future versions and recovery

Version 001 is immutable once applied. Future releases must add a separate numbered
migration, checksum/history validation, tests and a reviewed execution command.
This release provides the safe adoption baseline only: no arbitrary SQL command,
automatic data backfill, or down migration is exposed. Prefer additive schema
changes and review data backfills separately. MySQL DDL can commit implicitly;
transaction rollback cannot be assumed to undo it.

If metadata table creation succeeds but insertion fails, startup remains blocked.
Inspect the error and history; do not automatically delete or reset migration
metadata. Preserve backups before manual recovery. Older application binaries may
replay their destructive startup SQL, so rollback needs a compatible build with
those mutations disabled.

## Verification

Run `go test ./...` for isolated regressions. The preservation tests compare all
sample rows and table definitions, including binary data, null memberships,
legacy columns and attendance statuses. SQLite tests do not substitute for the
MySQL backup/restore rehearsal described above.

[Database CI](.github/workflows/database-tests.yml) provisions disposable MySQL 8
and runs the MySQL preservation test as well. Locally that test skips unless
`IBNU_MIGRATION_TEST_DSN` is set. It refuses writes unless the selected database
name starts with `ibnu_migration_test_` and contains no tables. It never loads
`.env` or the application's production connection settings. Use a new empty
database for each run and discard the isolated instance afterwards.

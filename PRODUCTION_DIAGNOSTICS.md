# Production diagnostics

Sign in as `super_admin`, then open **Log Request Sistem > Error & Debug**.
Ordinary administrators/staff cannot read or manage these diagnostics through
either the dashboard or API. The existing **Audit request** tab remains available.

## Investigating an incident

1. Ask the user for the ID laporan displayed after a server failure, or copy the
   `X-Request-ID` response header. The server generates its own ID; incoming IDs
   are not trusted. Search for that ID in Error & Debug.
2. Review the registered route template, handler, HTTP status, duration, fingerprint,
   and linked activity-log ID when available. Search the fingerprint to find
   similar failures. Request IDs also appear in server console logs.
3. Enable **Aktifkan debug 15 menit** if necessary. This temporarily adds rejected
   4xx requests and requests taking at least 1000 ms. It does not capture payloads
   or enable GORM SQL debugging. Server 5xx responses are captured without debug.
4. Reproduce the issue, inspect the new diagnostic, and investigate the named
   handler/dependency. Known returned MySQL errors have safe numeric classifications
   such as missing column/table, duplicate key, deadlock, and connection exhaustion.
   Panics include code stack frames, without local variables or the panic value.
5. After fixing/deploying/verifying the underlying issue, select **Tandai ditangani**.
   **Buka kembali** reopens that event. These operations change only diagnostic
   metadata; they do not fix code or modify/delete business records or audit logs.

## Privacy and error responses

Diagnostics intentionally omit raw exception text, SQL, request/response bodies,
query strings, authorization headers, cookies, IPs, emails and other user fields.
Routes use registered templates such as `/api/public/arrivals/:token/search`;
unmatched paths become `(unmatched)`. New audit rows also use templates. Historical
audit rows are not rewritten; they may contain old raw paths or personal data.
Audit responses preload only the user's ID/name rather than the full profile.

Unhandled/returned 5xx errors and direct 5xx JSON responses receive a generic safe
message plus request ID. Maintenance's intentional 503 message remains intact.
Console request logging omits raw URLs/queries. These changes do not inspect or
sanitize arbitrary pre-existing manual `log.Print` calls or infrastructure logs.

## Storage and retention

Set `DIAGNOSTICS_DIR` to a private, persistent directory outside the web root and
uploads. cPanel deployment defaults it to `$RUNTIME_DIR/diagnostics`; ordinary
local runs default to `logs/diagnostics` (ignored by Git). No new database table,
column, migration, or existing-data rewrite is required.

Events are private JSON files, written via temporary-file replacement. At most
1000 events from the last seven days are retained. Cleanup affects only files
owned by this diagnostics store; existing audit/business records are untouched.
Debug expiry, resolved status and resolving user ID persist across restarts.
Expired debug sessions stay off. Include this directory in private operational
backups if incidents must be retained longer.

If initialization fails, bounded memory capture is used and the dashboard shows
storage unhealthy. File failures also show this warning; debug/resolution writes
report failure instead of pretending to persist. Inspect permissions, disk space
and server stderr. Startup failures before HTTP initialization still require
the hosting/process log.

This store supports one backend process per directory. Replicas need separate
directories or a centralized logging service; it is not a multi-process database.
The dashboard reads its current process's retained events. Retention prunes while
capturing/reading events and on startup. Filesystem/process failures cannot be
guaranteed to preserve every event.

## API

All endpoints require JWT authentication and the current `super_admin` role:

- `GET /api/system-logs`: paginated events, summary and debug/storage settings.
  Filters: `search` (request ID, route, handler, fingerprint), `status`, `severity`
  (`error`/`warning`), `resolved` (`true`/`false`), `page`.
- `PUT /api/system-logs/debug`: `{ "enabled": true }`; expires after 15 minutes.
- `PUT /api/system-logs/:id/resolution`: `{ "resolved": true }` (false reopens).

Log reads/control requests are excluded from diagnostic capture to prevent
feedback loops. Authorization still depends on database availability. This is
request-level server diagnostics, not a profiler, arbitrary command runner,
frontend JavaScript crash reporter, or complete infrastructure-monitoring system.

## Validation

Go tests cover capture, panic recovery, secret omission, access control, filtering,
file persistence, resolution/reopening, debug expiry and bounded retention.
Playwright covers the operator flow, filters, denied access, failed debug updates
and user-visible report IDs on desktop/mobile. Tests use temporary storage or
mocked APIs, not production data. No live debug session was enabled during setup.

# Maintenance mode

Sign in with the `admin` or `super_admin` role, then open Dashboard > Pengaturan.
The Mode Pemeliharaan panel controls activation, scope and a visitor message of up
to 500 characters. Save to apply; toggle off and save to reopen. Administrator
access is checked against current database roles, not browser state.

Default: disabled. Reading status does not create a setting. The first save writes
one reserved `maintenance_mode` entry in the existing `cms_settings` table; no
schema migration or other business-record changes are required. Ordinary content
editors cannot change this key through generic CMS endpoints.

- Whole application: API calls from visitors and non-admin staff receive HTTP 503
  with `error: maintenance`, `Retry-After: 60` and `Cache-Control: no-store`.
- Public website only: only `/api/public/*` is blocked for non-admins; authenticated
  staff continue using the dashboard.
- Login, refresh, logout, current-user lookup, status and the authenticated control
  endpoint remain available. Only administrators can update the setting.
- The frontend checks on navigation, reacts to maintenance API responses, and
  offers retry plus automatic status checks every 30 seconds on the maintenance
  page. An already open idle page changes when navigating or making an API call.

HTTP endpoints: `GET /api/maintenance` is public and returns enabled, scope,
message and can_bypass. `PUT /api/maintenance` requires an administrator bearer
token and JSON `{ "enabled": true, "scope": "all", "message": "..." }`.
Scope can be `all` or `public`. Messages are rendered as text, never HTML.

Keep the backend and database running while this mode is active. It is an
application availability control, not a database freeze: administrators, auth,
audit logging, already-running requests/streams, and direct database clients can
still write. Static assets/uploads and `/health` remain available. For database
outages or a full offline backup window, use a maintenance page at the reverse
proxy/hosting layer and stop writers separately. Existing SSE streams are not
terminated by enabling this switch.

No live mode was activated during implementation. Go and browser tests use
isolated databases or mocked HTTP responses.

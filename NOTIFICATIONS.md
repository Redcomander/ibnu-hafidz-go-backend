# Notifications

The notification bell now uses the server's total unread count, not just the
loaded page. Older pages can be loaded from the dropdown. Read failures remain
visible and do not pretend notifications were read. Notification rows are scoped
to the authenticated user; another user's ID returns 404.

## Delivery and recovery

- Authenticated POST `/api/notifications/stream/ticket` issues an opaque, single-use
  ticket valid for 30 seconds. GET `/api/notifications/stream?token=...` consumes
  that ticket and independently checks the current user and maintenance state.
  Stream tickets cannot authenticate ordinary API requests.
- Streams flush an initial heartbeat, send keepalives every 15 seconds, buffer up
  to 32 live events and reconnect after five minutes to revalidate access.
- Failed ticket acquisition and disconnected streams retry with exponential delay
  capped at 30 seconds. Connections use the same `/api` origin as other requests.
- Visible authenticated tabs poll the database every 30 seconds even when a
  stream appears connected. Reopening a tab or going online also refreshes data.
  Polling recovers dropped/buffered events, cross-tab read changes and events from
  another backend process. Live events are deduplicated by notification ID.
- Account changes/logout close streams and timers, clear notification state and
  invalidate pending HTTP responses/tickets. Access-token refresh alone does not
  tear down the stream or race a second connection.

Configure any proxy/CDN not to buffer `text/event-stream`; the backend sets
`X-Accel-Buffering: no`. On hosting that cannot stream, periodic polling still
works. Poll delivery may be delayed up to the next refresh. Stream clients and
single-use tickets are process-local: load-balanced deployments need sticky
routing or a shared broker for reliable live streaming; polling remains the
durable fallback.

## Halaqoh workflow

Teachers submitting membership changes notify each active admin/super-admin once.
Approval/rejection notifies the requesting teacher and links to their Halaqoh
history. Repeated review attempts return conflict without generating duplicates.
The request/review and notification records commit in the same database transaction;
broadcast occurs only afterward. Notification persistence failure rolls back the
operation rather than silently claiming successful delivery.

No new schema or historical-data migration is needed: this uses the existing
`notifications` table. Existing notifications are preserved. Other modules still
need explicit notification producers; this is not a promise that every CRUD action
generates a notification. These are in-app notifications, not browser push/email
or WhatsApp messages, and do not deliver while all app tabs are closed.

Whole-app maintenance blocks staff stream connections as well as ordinary APIs;
administrators can still connect. Existing streams expire within five minutes.

## Verification

Backend tests cover global unread totals, bounded pagination, cross-user read
isolation, buffered delivery, one-time/expired tickets, workflow notifications,
duplicate review and atomic rollback. Browser tests cover badge totals, older
pages, reconnect after ticket failure, polling recovery, event deduplication,
read failures and logout during pending connection setup, on desktop and mobile.
Testing uses isolated databases and mocked browser streams; production proxy
streaming still needs verification after deployment.

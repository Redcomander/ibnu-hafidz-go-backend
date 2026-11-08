# Security upgrade: SQL, articles, and refresh sessions

Deploy the backend and frontend together. The backend startup migration creates
`refresh_sessions`; its database account needs permission to create this table
and indexes. No production data migration was executed during development.

Existing refresh JWTs are intentionally rejected after deployment. Users must
sign in again. New refresh cookies are random opaque tokens, stored only as
SHA-256 digests on the server. They rotate on each refresh, expire seven days
after the original login, and are revoked as a family on replay or logout.
Password changes through self-service or user management revoke all refresh
sessions for that user. A password fingerprint also rejects sessions following
password updates performed outside these handlers.

Access JWTs remain valid for their existing 15-minute lifetime. This change
revokes refresh sessions; it does not add per-request access-token revocation.

Logout now authenticates using the refresh cookie, so it works even after an
access token expires. The cookie remains HttpOnly, SameSite=Lax, and Secure in
production. Deploy with APP_ENV=production and HTTPS as before. The frontend
shares concurrent refresh requests and serializes refreshes across browser tabs
using Web Locks when supported.

Article HTML is allowlist-sanitized on save and on read. Existing stored articles
are protected without rewriting historical rows. Public author responses contain
only ID, display name, and avatar. Article thumbnails must be actual JPEG, PNG,
or WebP images, at most 5 MB and 20 megapixels. Gallery selection uses gallery IDs;
legacy gallery paths are accepted only when they match an existing local upload.
Shared gallery images are not deleted when an article changes. File operations
on article-owned thumbnails are confined to the uploads root with os.Root.

Scalar route/form IDs cannot become raw GORM conditions. Invalid numeric IDs
match no record. Sort keys for users, students, activity logs, classes, and rooms
are restricted to known model columns and use quoted structured ordering.

Verification:

```text
go test ./...
go vet ./...
npm test                     (frontend repository)
npm run build                (frontend repository)
```

The frontend also includes `tests/article-html.browser.html`, served through
Vite, to exercise the real DOM sanitizer against script, event-handler, unsafe
link, SVG, and form payloads. Other findings from the security review, including
role-assignment authorization and construction-module permissions, are outside
this patch. This patch upgrades the JWT parser and adds HTML sanitizers; it does
not attempt the separate broad dependency-upgrade project.

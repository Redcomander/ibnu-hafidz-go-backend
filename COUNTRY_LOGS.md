# Country in request logs

The logger records country headers such as `CF-IPCountry` when the reverse proxy
provides them. If they are absent, opening request logs or device login history
resolves missing countries from recorded public IP addresses in cached batches.
Successful matches are saved to those log rows, including older entries. Normal
application requests do not wait for geolocation.

The default fallback is `https://api.country.is/`. Only IP addresses are sent;
usernames, cookies, and request paths are not sent. Set
`IP_COUNTRY_LOOKUP_ENABLED=false` to disable it, or set `IP_COUNTRY_API_URL` to a
self-hosted compatible API. Restart the backend after changing the API URL.
Lookups have a short timeout and cache; failure does not prevent viewing logs.
Private, loopback, and reserved/documentation addresses remain unknown.

The reverse proxy must preserve the actual client IP (or set the country header).
If logs contain only a proxy's IP, the result describes that proxy. IP geolocation
describes the network exit location, so VPN users may show the VPN's country.

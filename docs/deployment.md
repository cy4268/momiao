# Portal deployment

Build the Go binary and `web/dist/` from the same source commit. Deploy an immutable release containing `bin/momiao` and `web/`, record file hashes, and point a `current` symlink at that release. Run with an unprivileged service identity.

The deployment topology for this slice is:

```text
HTTPS ingress -> private portal Unix socket -> private native API Unix socket
                         |
                    built SPA assets
```

No public application port is required. The portal process needs read access to the build, permission to create its own listener, and connection permission on the upstream socket. Keep both runtime directories private. A service can use a private network namespace and `RestrictAddressFamilies=AF_UNIX`; Unix sockets still connect across namespaces through the shared filesystem.

## Environment

```text
MOMIAO_WEB_DIR=/srv/momiao/current/web
MOMIAO_LISTEN_SOCKET=/run/momiao/portal.sock
MOMIAO_NEWAPI_SOCKET=/run/native-api/http.sock
```

Do not additionally set `MOMIAO_LISTEN_ADDR`. New listener paths are mode 0600. Existing objects are never unlinked at startup; use a service-owned runtime directory with a managed lifetime. Stop the service normally before replacing a listener. A killed process may leave a stale path which must be inspected before removal.

The native service must independently support the [pinned API contract](../contracts/native-api.md). Set native `SESSION_COOKIE_SECURE=true`, `SESSION_COOKIE_TRUSTED_URL` to the exact public HTTPS origin, and native `ServerAddress` to that origin. Preserve incoming Origin and the native refresh cookie unchanged. The portal does not create users, reset passwords, or migrate accounts at startup.

Use service hardening (read-only releases, no added capabilities, private runtime directory). If an ingress service and portal share a UID for 0600 socket access, deny the portal access to the ingress credential directories; do not give the application a tunnel or DNS API token.

## Acceptance and rollback

1. Verify the new private socket's owner/mode and `GET /healthz` before changing ingress.
2. Verify HTML and built assets, then the fixed upstream `GET /api/status` through the private socket.
3. Change only the intended hostname's origin service. Keep unrelated routes, DNS records, old service and databases untouched.
4. Through public HTTPS, verify actual login, self, temporary bounded-key creation/reveal/status/delete, personal logs, refresh and logout. Do not retain test keys or put their values in logs.
5. Check desktop/mobile layout, service status, and host network/firewall invariants. A green health endpoint alone does not pass this gate.
6. Roll back ingress to the previous origin if acceptance fails. Retain the previous origin and release until this verification is complete.

This revision intentionally has no automated database migration, automatic upstream fallback, or egress policy changes. Model traffic, 2FA enrollment, trusted client-IP propagation, rate-limit policy and load behavior need their own explicit acceptance; successful UI/account operations do not establish those results.

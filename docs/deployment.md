# Portal deployment

## Optional wallet database

Wallet reads and explicit zero initialization use a separate PostgreSQL database. Set `MOMIAO_WALLET_DSN_FILE` (absolute regular file, at most 8192 bytes) and `MOMIAO_PUBLIC_ORIGIN` (exact HTTPS origin) together with the existing complete portal configuration. Protect the DSN file with a restricted group and do not print it, put it in command arguments, or commit it. No credentials are bundled.

Use a non-login schema owner and a distinct runtime role. Runtime needs schema USAGE, SELECT on account_refs/wallet_balances/wallet_ledger, INSERT(newapi_user_id) on account_refs and INSERT(newapi_user_id,asset_type) on wallet_balances only. Do not grant wallet UPDATE, balance_units INSERT, ledger mutation, owner membership or DDL. A2a does not expose Apply.

For Unix-socket deployments, protect the socket directory and require SCRAM authentication for **all** local connections. Do not expose a trust-authenticated native PostgreSQL socket through a proxy. An isolated, network-disabled platform PostgreSQL instance avoids changing the native service's authentication or lifecycle.

Build `./cmd/momiao-migrate` as a separate administration tool. It reads only `MOMIAO_MIGRATION_DSN_FILE`, performs explicit checksum-checked migration and prints generic success/failure; it is not invoked by portal startup. Preserve applied SQL bytes (repository LF policy); validate migration twice, restore a backup into a separate temporary database, and test the runtime role's denied operations before enabling the wallet.

The runtime uses a lazy pool: a connection outage returns wallet 503 without preventing native portal startup. Invalid configuration remains a startup error. Test database restart, portal restart, preserved initialized balances, and credential-restricted recovery in the actual environment. Such tests are not whole-host reboot or load-capacity evidence. A portal rollback must retain the independent database and initialized users, not delete them.

Wallet paths and payloads: [wallet-api.md](../contracts/wallet-api.md). Native authentication and database operations share a 5-second context budget. Request reads are separately subject to the HTTP server's 10-second read timeout. These are engineering ceilings, not throughput guarantees.

## Existing native portal transport

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

This revision has no automated database migration or automatic upstream fallback. Configuring a channel alone does not establish model traffic: verify one bounded synthetic request, streamed termination and native consumption records. 2FA enrollment, trusted client-IP propagation, rate-limit policy and load behavior need their own acceptance.

## Native model traffic

The portal forwards the exact `/pg/chat/completions` route to the same native socket with a five-minute total deadline; it does not forward arbitrary `/pg/` paths. Public API clients retain `/v1/` and their own API keys. Model calls require a configured, reachable and priced native channel.

For an outbound-isolated native namespace, a fixed-destination transport can keep that isolation intact: a loopback-only namespace listener connects through a private Unix socket to a host-side TLS connection for one upstream. Verify certificate trust and hostname/SNI, preserve the correct upstream HTTP Host, and keep the Unix directory/socket private. Use a dedicated unprivileged identity; do not grant the portal access to the transport or credentials. Reconcile the bridge when the native namespace changes, stopping it when the namespace is not ready. Do not add a generic forward proxy or change Docker/LXD firewall rules merely to serve the portal.

This transport is deployment-specific, not automatically installed by momiao. Its destination, ports, authentication and native metering must be derived from the actual environment. Example bounds or one successful request are not capacity measurements. Rolling back a portal release does not roll back channel settings; retain a separate record of explicit channel/rate changes and disable the new channel before removing its transport.

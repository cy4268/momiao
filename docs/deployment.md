# Portal deployment

## Optional wallet database

Wallet reads and explicit zero initialization use a separate PostgreSQL database. Set `MOMIAO_WALLET_DSN_FILE` (absolute regular file, at most 8192 bytes) and `MOMIAO_PUBLIC_ORIGIN` (exact HTTPS origin) together with the existing complete portal configuration. Protect the DSN file with a restricted group and do not print it, put it in command arguments, or commit it. No credentials are bundled.

Use a non-login schema owner and a distinct runtime role. Runtime needs schema USAGE, SELECT on account_refs/wallet_balances/wallet_ledger, INSERT(newapi_user_id) on account_refs and INSERT(newapi_user_id,asset_type) on wallet_balances only. Do not grant wallet UPDATE, balance_units INSERT, ledger mutation, owner membership or DDL. A2a does not expose Apply.

For Unix-socket deployments, protect the socket directory and require SCRAM authentication for **all** local connections. Do not expose a trust-authenticated native PostgreSQL socket through a proxy. An isolated, network-disabled platform PostgreSQL instance avoids changing the native service's authentication or lifecycle.

Build `./cmd/momiao-migrate` as a separate administration tool. It reads only `MOMIAO_MIGRATION_DSN_FILE`, performs explicit checksum-checked migration and prints generic success/failure; it is not invoked by portal startup. Preserve applied SQL bytes (repository LF policy); validate migration twice, restore a backup into a separate temporary database, and test the runtime role's denied operations before enabling the wallet.

The runtime uses a lazy pool: a connection outage returns wallet 503 without preventing native portal startup. Invalid configuration remains a startup error. Test database restart, portal restart, preserved initialized balances, and credential-restricted recovery in the actual environment. Such tests are not whole-host reboot or load-capacity evidence. A portal rollback must retain the independent database and initialized users, not delete them.

Wallet paths and payloads: [wallet-api.md](../contracts/wallet-api.md). Native authentication and database operations share a 5-second context budget. Request reads are separately subject to the HTTP server's 10-second read timeout. These are engineering ceilings, not throughput guarantees.

## Master profile extension

Schema migration 2 adds own Master profiles and append-only name history; the reserved-name baseline is versioned in application code. Never edit already applied migration 1. Master shares the existing protected platform DSN and native identity transport. Apply the new migration explicitly with the separate migration tool, then grant only profile-specific SELECT/INSERT/UPDATE columns and history INSERT to the runtime role. Do not widen economic writes, database ownership or credential access.

Back up initialized wallets before migration, migrate twice, and compare all pre-existing identity/economy rows exactly. Test the new application with the runtime role against a separate acceptance database, including name conflict, stale versions, rename cooldown, no-op updates and immutable history. Never choose and permanently save a real user's public display name just to complete a test.

Roll back the portal binary and built assets together if needed, retaining schema 2 and every real profile/history/wallet record. The older migration executable intentionally rejects unknown newer schema versions; it is not a downgrade tool. Verify the previous portal's wallet reads against schema 2 separately. Whole-host recovery, broad moderation and public-game profile gates remain independent acceptance items. See [Master API](../contracts/master-profile-api.md) and [slice decision](decisions/0006-master-profile.md).

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

### Schema 3: wallet actions
Apply the explicit migrator once before the portal release, with a database backup for the changed constraints/new daily table. No database restart is needed. Add only these runtime object grants (retain existing wallet/profile grants):

```sql
GRANT UPDATE(balance_units,ledger_seq,version,updated_at) ON economy.wallet_balances TO momiao_wallet;
GRANT SELECT,INSERT ON economy.asset_transactions,economy.wallet_ledger,platform_meta.mutation_idempotency_records TO momiao_wallet;
GRANT USAGE ON SCHEMA rewards,platform_meta TO momiao_wallet;
GRANT SELECT,INSERT ON rewards.daily_checkins TO momiao_wallet;
```
Runtime still has no UPDATE/DELETE on transaction, ledger, key history or daily claims; immutable triggers remain. The prior portal remains compatible with schema 3, so application rollback retains new data without a destructive down migration. For verification use a private clone for positive claims/exchanges; production balances are not test fixtures.

## Schema 4: one-way native quota activation

See [quota-transfer-api.md](../contracts/quota-transfer-api.md) for source pins, unit conversion and recovery semantics. This is an optional deployment-specific bridge, not a generic native quota mutator.

1. Verify the pinned native image/source and actual `users` columns; back up both affected databases and native configuration once. Keep old service and domain separate.
2. Use native's supported DB-only accounting: empty `REDIS_CONN_STRING` and `BATCH_UPDATE_ENABLED=false`. Preserve the Redis container/data, but do not reuse its cached quota after DB-only writes.
3. Explicitly install `internal/platform/native_quota.sql` in **native** PostgreSQL. It is default-disabled. Use a non-login owner for this schema/tables/functions with only `SELECT(id,quota,status,deleted_at)` and `UPDATE(quota)` on `public.users`, plus public-schema USAGE. The login runtime gets only schema USAGE and EXECUTE on `read_quota(bigint)`, `query_operation(uuid,bigint)`, `credit(uuid,bigint,bigint)`. No owner membership or direct users/table/credential access. Do not grant arbitrary native administrative access to the portal.
4. Protect the native transport. A Unix-to-namespace-loopback PostgreSQL proxy still needs SCRAM for **every** TCP role, not just the runtime role. Check actual HBA ordering: the image may have loopback `trust` before the catch-all SCRAM rule. Replace loopback trust with SCRAM, reload configuration, and test that a wrong password is rejected. Keep native SQL credentials valid; no database restart is required. Never expose the container's trust-authenticated Unix socket.
5. Apply platform migration 4 explicitly and retain prior runtime grants. Add only:

```sql
GRANT SELECT,INSERT ON economy.quota_transfers TO momiao_wallet;
GRANT UPDATE(status,reason,native_before,native_after,updated_at) ON economy.quota_transfers TO momiao_wallet;
```

6. Complete an isolated native-image/cross-database flow with restricted roles. After verified accounting mode, enable native `momiao_quota.settings.enabled`. Set `MOMIAO_NATIVE_QUOTA_DSN_FILE` to a private regular file (max 8192 bytes), never a command-line secret. The portal uses a lazy native pool and background worker. No configuration means no worker; it does not auto-install or auto-enable SQL.
7. Keep the native PostgreSQL proxy bound to the verified isolated network namespace. Stop it if namespace/image/accounting mode drifts. No host TCP listener or public database port is needed. On native upgrades, disable new requests and drain/reconcile outstanding original operations before changing mode or quota scale.

Rollback the portal release while **retaining both journals and all balances**; migration 4 is additive. Keep native DB-only accounting during an application rollback. Do not restore a database snapshot or re-enable an old Redis cache after credits have been written: that could erase receipts or resurrect stale quota. Target disabled/unreachable leaves unknown transfers PENDING; re-enable only after the original operation IDs are reconciled. NEEDS_REVIEW requires explicit operator repair from both journals; no automatic manual-review resolution is provided in this slice.

## Schemas 5-9: incremental runtime grants

Apply the reviewed migrations as the existing non-login schema owner, retaining the 1-4 grants above. [Runtime template](../deploy/sql/runtime-grants-0005-0009.psql) and [separate bootstrap deployer template](../deploy/sql/bootstrap-deployer-grant.psql) are opt-in psql inputs, not portal startup hooks. With `apply_grants` omitted/false they issue no SQL changes. Use a protected `PGSERVICE`/passfile, `psql -X`, and trusted role-name parameters; never put passwords in arguments:

```sh
psql -X --set=schema_owner=SCHEMA_OWNER --set=runtime_role=RUNTIME_ROLE --file=deploy/sql/runtime-grants-0005-0009.psql
# Only after review, add --set=apply_grants=true to apply that template.
# Separate bootstrap template additionally requires --set=bootstrap_deployer=DEPLOYER_ROLE.
```

The existing roles must be distinct: runtime/deployer have no owner membership, superuser, CREATEDB, CREATEROLE, replication or BYPASSRLS powers. Bootstrap's three SECURITY DEFINER functions must retain the same controlled schema owner. No role creation, schema CREATE, ownership transfer, native `users` access, bootstrap invocation or notice publication is included. The bootstrap template grants only schema USAGE plus the narrow function EXECUTE; revoke that EXECUTE and remove its credential after the approved attempt/result reconciliation.

Column grants follow the actual store SQL: announcements/job/audit writes (5), receipt/grant workers including deferred issuance reads (6), catalog sync/editorial writes (7), no runtime bootstrap/history/closure grants (8), notice SELECT plus ACK-key INSERT only (9). `UPDATE(updated_at)` on principals is required for their `SELECT FOR UPDATE`; it is not authority-field access. Announcement placement guards similarly need one UPDATE column for locking, with actual mutation still rejected by their immutable trigger. No 5-9 sequence grants are needed. No broad/default table grants are installed, and inherited pre-existing grants are not silently repaired by these templates.

These templates have not been applied to production. Catalog grants were derived from the current M3c storage SQL, not an imported WIP build. Reconcile against its final integrated SHA, then perform one combined acceptance using the actual low-privilege runtime identity: announcement writes/jobs, receipt ingestion/grant completion, sync/metadata/publication, notice ACK/replay; separately verify denial of DDL, authority/closure/history mutation, bootstrap EXECUTE, notice-fact mutation and direct native `users` access. Schema-owner tests do not establish runtime acceptance. Retain all records on application rollback.

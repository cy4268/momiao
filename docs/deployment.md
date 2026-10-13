# Portal deployment

## Local identity migration (in progress)

Migration 0048 adds `identity.users`, `identity.two_fas`, and `identity.two_fa_backup_codes`. Authentication code is adapted from **unpatched official QuantumNous/new-api, revision f116414284162ad15d8925f7bca494c109b83e93**, not the VPS's momiao login patches. It retains bcrypt hashes, TOTP/backup-code formats and the existing five-attempt factor lockout.

Build `go build ./cmd/momiao-migrate` and `go build ./cmd/momiao-import-native`. Apply schema with the existing migration tool and `MOMIAO_MIGRATION_DSN_FILE`, then run the importer with that target DSN file and `MOMIAO_IMPORT_NATIVE_DSN_FILE` pointing to the native source. Use the isolated dev databases first. The importer reads a consistent read-only source snapshot and writes the three empty target tables in one transaction; it preserves IDs, deleted/disabled users, factor state and consumed backup codes, then advances each identity sequence. User IDs also retain the source sequence high-water mark and existing account references, so deleted users' historical IDs are not reused. The importer locks and checks all three target tables first: any existing row causes the import to fail rather than append or overwrite passwords. Command failures print a fixed message, not connection strings.

The default `-stage identity` reports counts for these three tables only; `-stage keys` is a separate import described below. Neither stage merges Reserve, migrates sessions, imports channels/prices/logs, or switches the running web login. Do not use this partial importer for production cutover. Before running the new local authentication entry point, apply `deploy/sql/runtime-grants-0048-local-identity.psql` as the owner with `platform_runtime_role` and `poker_authority_role`; it grants platform table-level CRUD and Poker SELECT on local users without changing existing business-table grants.

The local Redis session adapter keeps only user ID, creation time and CSRF token, using the existing `__Host-chaldea_session` cookie and 7-day idle / 30-day absolute lifetimes. Its `chaldea:session:local:*` keys use the existing session Redis role; add `+pexpire` to that role in `bundle/redis/users.acl` and reload Redis before using the adapter. User indexes expire automatically after their newest session's absolute lifetime; old native-backed sessions are not read or migrated. The new HTTP entry point checks Origin/CSRF on writes and reads the local user's enabled status on each protected request. Password login holds the `identity.users` row lock through session issuance; five-minute factor flows are browser-bound, stored separately from login cookies, and included in user-wide revocation. Password/status mutations must use the same row lock when revoking sessions. Business handlers use a typed local user/session context, not native credential headers. The main program shares the wallet PG pool with identity; anonymous login-CSRF sessions expire after 15 minutes. Dev has the 0048/0049 grants on its wallet runtime role. Local password/Discord login, account password/2FA, the frontend, self/Reserve and admission reads are wired; History, Bot and Poker now use local identity; Ops still requires migration before the integrated dev rollout. Handler tests use real dev PostgreSQL/Redis but mock Discord provider responses; no website image or live provider acceptance is implied.

Set `MOMIAO_DISCORD_CONFIG_FILE` to private JSON containing `client_id` and `client_secret`; the callback is always `MOMIAO_PUBLIC_ORIGIN` + `/oauth/discord`. Registration uses the existing `MOMIAO_ADMISSION_ENABLED` flag and additionally requires `guild_id`, `role_id`, and `policy_version`. Only registration requests the `guilds.members.read` scope. Existing Discord accounts use normal login/2FA without membership rechecks or new grants. New members need the configured role; account, account reference, receipt and counter commit together, then the existing local grant worker handles credit. Login and password proof still request only `identify`.

Local Ops maintenance requires migration `0050_local_ops_maintenance.sql` and `deploy/sql/runtime-grants-0050-local-ops.psql` with `platform_runtime_role`. `GET/PUT /api/v1/ops/maintenance` uses the local Cookie, Origin/CSRF and live `identity.users.role >= 10`. PUT accepts `enabled` (required), `message` and `scopes` (omitted means all). Save reuses the existing maintenance tables/admission locks, ends previous active windows and appends one audit row in the same transaction. Old internal reasons are never published; the separate public message is optional. Migration 0050 cancels scheduled maintenance/jobs but retains active windows and historical rows. Scheduling/Fresh execution is removed; the remaining campaign worker keeps running. Ops navigation is role-based; unmigrated child pages retain their existing bootstrap boundary until their direct-save replacements are finished. This is not a completed Ops rollout, and no local-to-native authorization adapter is introduced.

Ops audit reads use migration `0051_ops_audit_target_type.sql` plus `runtime-grants-0051-ops-audit.psql`. Direct audit writes now include `target_type`; existing direct entries are maintenance. The global read model uses one SQL union over `ops.audit_log` and the retained `audit.audit_events`, without copying history or dual-writing. `/api/v1/ops/audit` accepts page/page_size (default 1/20, maximum size 100), actor_user_id, target_type, target_id, action, audit_id and RFC3339 from/to (inclusive/exclusive). Local role >= 10 is required; no authz_epoch. Existing audit deep links open the same page's details dialog. The operations list is replaced by this page; individual legacy receipts remain until their writers are migrated. Source-only rollback does not downgrade schema 0051; the pre-0051 direct writer omits the new required target_type, so do not deploy that older writer against schema 0051 as a runtime rollback.

Local key management requires migration `0052_local_api_keys.sql` and `runtime-grants-0052-local-api-keys.psql` (`platform_runtime_role`). With the existing source/target DSN-file variables, run `momiao-import-native -stage keys` once into an empty `relay.api_keys`; `-stage identity` remains the separate identity import, not a prerequisite to rerun on dev. Only live tokens are copied with their existing key/ID, usage, expiry, model restrictions and purpose; deleted token IDs still advance the sequence. Finite quota_limit is used_quota + remaining quota; NULL means unlimited. Dev had 68 live keys (82 rows including 14 deleted), all unlimited with no IP/model restrictions; 26 GENERAL, 39 ROLEPLAY, 3 UNCLASSIFIED. Native IP-restricted tokens are not part of this dev dataset; their restrictions need an explicit data/model decision before importing a different dataset.

The local `/api/token/` owner-scoped handlers retain list pagination/masked output and numeric ID/basic quota fields; create returns the existing string ID. Creation requires purpose, while `PUT ?purpose_only=true` saves purpose directly. Keys always use the owning user's group. All writes/reveal use local Origin/CSRF; maintenance admission is retained. Official f1164142 token masking, 48-character cryptographic key generation and ownership semantics are reused, not the VPS customized token projection. The browser no longer performs a separate purpose transaction or response-schema verification. Old purpose HTTP/outbox worker, native purpose RPC methods and native token projection handlers are removed; historical purpose tables remain until final native cleanup. This is not relay/Reserve cutover: keep the running site image and production native services unchanged until the remaining billing/relay and full dev acceptance are complete.

Local usage/groups/models require migration `0053_local_relay_workspace.sql` and `runtime-grants-0053-local-relay-workspace.psql` with the same runtime role. Run `momiao-import-native -stage workspace` once into empty `relay.groups/channels/logs`, using the existing DSN-file variables. It copies all log metadata and preserves channel keys, models/groups, overrides, multi-key state and provider settings in one transaction; existing users, keys and balances are untouched. Dev contains 4 groups, 17 channels and 29279 logs. Native raw quota and Reserve both use 500000 units per credit, so historical log cost is copied unchanged, not credited to wallets. All historical log types remain (including admin/login rows); only consume/error rows count toward self request totals. Log content, IPs and audit payloads are not imported; old error content becomes a generic label and unknown HTTP status stays 0. Cache-write totals prefer the official normalized `cache_write_tokens` field; 1h counts are also preserved. Imported updated_at uses the native channel creation timestamp because native has no configuration-update timestamp.

`GET /api/log/self` retains owner-scoped pagination, type, inclusive Unix-time bounds and exact model matching (explicit `%` follows official f1164142 wildcard semantics). `/api/user/self/groups` returns only the account's group and ratio; `/api/user/models` lists distinct models from enabled channels in that group, rejecting a different requested group. Both API-only and web routers use local sessions, never the native projections. `/api/user/self` aggregates local log costs/request counts alongside Reserve. Old workspace/log proxy handlers and their response validators are removed. Relay adapters and the integrated dev release remain unfinished; the pricing/settlement layer is described below; do not rerun completed identity/key/workspace import stages.

The pricing/settlement layer requires `0054_local_relay_billing.sql` and `runtime-grants-0054-local-relay-billing.psql`. `momiao-import-native -stage prices` materializes final numeric prices into an empty `relay.prices` without rerunning previous stages. Input/output/cache rates use Reserve units per million tokens; per-request rates use units per request. Official f1164142 completion defaults/locks and model-name matching are copied unchanged into the importer, not a runtime ratio chain. Numeric JSON settings go directly through PostgreSQL numeric arithmetic. Dev imported 297 effective prices: 32 per-request, one duration-based, the remainder token-based. Eight channel aliases have no configured price (three CPA, four workbuddy and one vertex alias); an additional obsolete thinking-wildcard setting does not resolve under official model matching. They remain unpriced rather than receiving invented/free prices; the importer reports `unpriced_models`.

The local relay admission/dispatcher uses these existing tables without another migration. `PrepareRelay` checks current key/user eligibility, key model restrictions, Reserve, `CHALDEA_USER_WRITES` maintenance, channel availability and configured price before upstream work. It preserves official priority/weighted selection (all-zero weights are equal), enabled multi-key selection and the selected polling channel's cursor in a short transaction. Mapping and raw override/settings metadata remain available to the protocol adaptor. Key state is queried directly rather than cached at the site's current scale. `GET /v1/models` is now local in both portal router modes; it intersects the account group with key model restrictions and ignores caller-supplied identity/group overrides. Discovery requires a valid key, not a positive balance. Other `/v1` paths are not yet replaced: do not release this source slice as a completed relay migration. The protected dev site still runs the previous image; no import, data merge or production change accompanies this slice.

The existing `grok-imagine-video` expression is linear in duration, so the schema additionally stores `per_second` and `default_seconds`, avoiding a billing DSL. Its official v1 expression conversion yields 35000 Reserve units/second and defaults to 8 seconds. Token accounting separates uncached input, cached reads, generic/5m writes and the 1h subset (official 1h multiplier 1.6), rounds the final group-adjusted amount once, and preserves the official minimum of one unit for positive sub-unit charges. Missing token usage bills zero; fixed per-request/duration pricing remains independent of token counts. Protocol adapters must normalize prompt totals to include cache reads/writes before `RelayCost`; no token estimation is performed.

`SettleRelay` owns one wallet transaction: shared account lock, request-ID replay check, capped Reserve debit through existing `applyInTx` (`API_USAGE`), key usage update, full-cost log and actual-debit attribution. Zero debit creates no zero-valued ledger row. In-flight requests retain key/name/purpose snapshots, including when the key has been deleted. Local attribution source UUID `8be7cf11-88df-46e9-a6ce-e81bf166e1d4` uses local log IDs; imported native IDs/cursors remain untouched. RP usage and the ranking SQL aggregate both sources; bigint identity/key/charged-amount ceilings replace obsolete int32 bounds. A single rollback-only real-dev test checks pricing, replay, late-statement rollback, capped debit, error logging and both readers. This is not end-to-end relay acceptance: `/v1`, official protocol adapters, stream/retry handling, admission, source-health cleanup and the integrated release are still pending. Keep production native and the running dev site image unchanged.

Before migration 0049, freeze old registrations, let the receipt worker catch up and compare native receipts, the platform inbox and cursor. The migration copies the inbox into local `momiao_admission.registration_receipts` and seeds its counter. Apply the same `runtime-grants-0048-local-identity.psql` after 0048/0049; retain existing account-ref and grant-worker permissions. `MOMIAO_REGISTRATION_READER_KEY_FILE` is no longer read. The imported dev users/receipts already exist: do not reimport them or reissue grants. Production native services and the native database remain retained until the final approved cutover.

History HTTP routes now use the shared local-session middleware and typed user context. History startup uses the existing wallet pool, not the old session service; dedicated History reader/worker and Poker-history reader settings remain unchanged. Bot identity resolution reads `identity.users` through the same platform wallet pool and retains account/profile/notice/game-admission checks. Remove the obsolete `MOMIAO_BOT_GAMES_NATIVE_DSN_FILE` variable and its exclusive mount when releasing the new platform image; only socket/token/quote-key paths remain in Bot listener configuration. Existing 0048 identity grants already cover this read, so no extra database connection, role, migration or grant is needed. Older Bot documentation describes the pre-migration native reader, not this local-identity configuration.

Poker now uses the same local Redis session for ticket minting, ticket acceptance, socket commands and password-access deadlines. Session references are the cookie's 64-character SHA-256 digest; they contain neither the cookie nor a Redis prefix. Existing ticket/control metadata stays at version 1 / epoch 0 without storing or checking an authentication version; revocation is Redis key deletion plus the live `identity.users` status. The standalone Poker process only GETs the session and its TTL; it does not renew browser idle lifetime.

Before the integrated dev release, reapply `runtime-grants-0048-local-identity.psql` with the existing platform runtime role and Poker authority-reader role. Add `MOMIAO_POKER_SESSION_REDIS_CONFIG_FILE` to the Poker container, using the same JSON shape as the platform session Redis file (`addr`, `username`, `password`, `ca_file`, `server_name`) and the same Redis database. Give this reader `+hello +ping +get +pttl` on `~chaldea:session:local:*`; no session write permission is needed. The platform reuses its existing session client and wallet pool. Remove `MOMIAO_POKER_SESSION_READER_KEY_FILE`, the Poker container's `MOMIAO_NEWAPI_SOCKET` and their exclusive mounts; keep the existing ticket/lease configuration and service-signing keys. These new grants, reader credentials/ACL and compose edits are deployment prerequisites, not changes already applied by the handler test. That test used existing dev platform session/wallet credentials and real dev ticket Redis, with no table or wallet mutations. The legacy `internal/poker/authbridge` package remains used by old Ops/BFF composition only and will leave with that remaining dependency.

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

The portal no longer offers a browser text playground or forwards `/pg/chat/completions`. Public API clients retain `/v1/`, their own API keys and the five-minute total relay deadline. Model calls require a configured, reachable and priced native channel.

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
# Model family cover uploads

Apply migration `0040_model_family_covers.sql` and the additive `deploy/sql/runtime-grants-0040-model-family-covers.psql` using the separated schema owner and platform runtime roles. Retain older grants; do not give runtime DDL, DELETE, asset-history UPDATE or audit UPDATE.

Platform-only optional settings:

```env
MOMIAO_CATALOG_ASSET_R2_ACCOUNT_ID=<32-character-account-id>
MOMIAO_CATALOG_ASSET_R2_BUCKET=<public-assets-bucket>
MOMIAO_CATALOG_ASSET_R2_CREDENTIALS_FILE=/run/secrets/catalog-r2.json
```

The credentials file is a regular, absolute-path, read-only deployment secret containing exactly `{"access_key_id":"<bucket-scoped-access-key>","secret_access_key":"<secret>"}`. Limit its filesystem access to the service user and R2 permissions to object writes in the selected assets bucket. Never commit or log it. Missing all settings disables only uploads; partial or invalid configuration fails startup. The poker process rejects this configuration. Use explicit credentials: ambient AWS credentials and arbitrary endpoints are not consulted.

Only `POST /platform/v1/ops/models/family-covers/upload` needs proxy body allowance of **at least 9 MiB** and request/response waits of **at least 95 seconds**. This authorized route has a 90-second server deadline, a 60-second R2 HTTP timeout and at most two SDK attempts. Other routes retain their existing deadlines. Do not globally increase body limits or timeouts.

Uploads accept static PNG/JPEG/WebP up to 8 MiB, 8192 pixels per side and 16,777,216 total pixels. The backend validates original bytes and writes `assets/models/uploads/<family>/<sha256>.<ext>` with the verified MIME and `Cache-Control: public, max-age=31536000, immutable`. Following [Cloudflare's Go SDK guidance](https://developers.cloudflare.com/r2/examples/aws/aws-sdk-go/), the S3 endpoint is derived from the account and uses region `auto`. Ensure the existing custom domain/cache rules cover this path and JPEG; verify an actual CDN GET, response headers and SHA-256 before claiming live acceptance. Viewing uses `VITE_ASSET_BASE_URL` directly, never the business server.

An uploaded file has a public address immediately; publishing controls the catalog binding, not confidentiality. Upload alone never replaces a cover. Preview and explicit confirmation with `models.publish` update one family for every public model. Repeated content retains its original metadata. Restore-default increments the version and retains all objects and audit records. A PUT acknowledged before database failure may leave an unbound object: retry the same operation/content; do not delete objects or rewrite audit history. Roll out forward migration/grants before server/frontend; the pre-0040 binary rejects a database containing migration 0040. The source-copy rollback does not downgrade schemas and is not a runnable old-binary rollback on the upgraded database. Prefer a forward-compatible fix; any recovery to an older release requires a separately reviewed matching database backup and reconciliation of later writes. Never drop these tables or rewrite migration history as an automatic rollback.

The portal's runtime CSP must separately allow the same CDN origin used at build time. Set `MOMIAO_ASSET_CDN_ORIGIN=https://assets.example.com` (origin only, no path or trailing slash) alongside `VITE_ASSET_BASE_URL=https://assets.example.com`. Only `img-src` gains that explicit HTTPS origin; script, connection, frame and native-auth policies remain unchanged. Missing runtime configuration deliberately keeps images self-only. Reverse proxies must not add a conflicting self-only image CSP.

### 本地渠道和分组运营

`/ops/channels` 与 `/ops/groups` 使用本地管理员会话和 CSRF，直接写 `relay.channels/groups`；已有 0053 运行授权即可，无新迁移。旧 `/admin/channels` 跳转，旧 `/api/channel/` 返回 404。密钥只写不读，留空保留；导入的多 Key、参数与 Vertex 配置保持原值。分组改名同步用户/渠道，仍被引用的分组拒绝删除；注册默认组 `default` 名称固定。

手动渠道测试复用中转协议适配，对首个模型发一条输出上限为 1 token 的请求，不重试、不扣用户 Reserve，仍消耗真实上游额度。模型拉取使用已保存配置，结果只填表，保存后才生效。Vertex 没有兼容发现地址时手动填写模型列表。模型矩阵仅表示启用渠道的模型/组匹配，不代表上游健康或价格可用。渠道保存后的下一次请求直接读数据库，无缓存刷新步骤。


### 本地用户运营与调用日志

`/ops/users`、`/ops/users/:id`、`/ops/logs` 使用本地 Redis 会话、CSRF 和 role >= 10，不读取旧 Ops bootstrap/epoch，不走 prepare/execute。用户写入事务重新核对操作者状态/角色；沿用官方 f1164142 的「root 或严格高于目标角色」规则，仅 root 修改角色，至少保留一位启用的 root。分组先锁 relay.groups，再锁用户行，与组改名保持同一顺序。角色/状态变更、重置 2FA 和强制下线复用现有用户会话撤销；组变更由后续请求实时读取。

Reserve 调账接受有符号 Credit 十进制字符串和备注，复用 ParseAmount / applyInTx，最小单位 0.000002，不透支、不四舍五入。浏览器在当前表单保留 request_id，失败后同一输入重试；业务号、账务幂等键同源。一次成功写入 OPS_ADJUST 流水和一条审计，重复请求不新增审计；备注由同事务审计关联到流水。不自动为尚未初始化的用户新建钱包。Key 只列掩码，停用/删除保留调用历史。

全局日志只列 kind 2/5 的消费/错误尝试。用户、Key、模型、渠道、日志分组和起止时间筛选后，用 request_id 合并逻辑请求；历史无 request_id 的每条日志独立计算。筛选内所有尝试均失败才计为错误请求，因此筛选单渠道或「仅错误」会改变该口径。Token 为 input + output（不重复叠加已包含的缓存输入），费用用 PostgreSQL numeric / 500000 转十进制字符串。Top 20 按当前维度请求数排序，现场 SQL GROUP BY，不建预聚合表或缓存。沿用 schema 57 和现有授权，无新迁移。

### 本地全局游戏记录

`/ops/records` 已移出旧 Ops bootstrap；本地 role >= 10 即可读取。GET `/api/v1/ops/records` 默认跨用户，`user` 为可选正整数；其余类型/模式/游戏/时间/结果/状态/ID/limit/cursor 沿用个人 History 契约。默认不夹带 Poker 手牌，选择 POKER_HAND 时逐参与者列出。详情 `/api/v1/ops/records/{rounds|sessions|hands|roulette|transactions}/{id}` 和适用的 `/verify` 必须带该行的 `user`，返回 `{subject_user_id,record_type,record}`。用户详情链接为 `/ops/records?user=ID`。

部署顺序：0058 migration → `deploy/sql/runtime-grants-0058-local-ops-records.psql`（schema owner，platform_runtime_role）→ 新源码。全局 SQL 只授权 platform；原 History reader 仍只有正用户的 `games.history_list`，继续复用同一投影函数，不授权索引原表。分页最后加参与者 ID，避免同一 Poker/轮盘局的其他参与者跨页丢失；个人游标格式与读取边界保留。

每次授权读取前，在事务内重读本地角色并写现有 ops.audit_log：RECORD_READ_LIST/DETAIL/VERIFY，after_state 包含 subject_user_id（0=全局）和查询条件。这里记录访问尝试，不代表源记录成功返回。旧 audit.record_access_events 原样保留，不再写 epoch/scopes。详情和证明复用原 domain reader，以实际 operator 为 viewer，不冒充目标参与者、不放宽暗牌/种子公开时点。

开发库是历史数据副本，但创建 dev 时重新生成了游戏/Poker 加密密钥。现有 Blackjack 历史在 openSeed 失败；Poker 手牌和轮盘 reader 也返回 HISTORY_UNAVAILABLE。列表、Dice 历史与证明、Poker 会话、交易详情可以验收；这不等于那些加密详情通过。必须在联合验收前解决开发数据与密钥的一致性（旧版本号相同不表示密钥相同），不跳过解密校验或把错误当空记录。源码回退不降 schema 58，不删除访问审计；本切片没有发布网站。


### 本地公告运营直接保存

`/ops/announcements` 已移出旧 Ops bootstrap。公告接口在有/无静态网站目录的两种路由下均使用本地 Redis 会话；公共 GET 接受匿名，登录后弹窗与已读写入使用当前本地用户。管理员 role >= 10 可读取、创建、发布/预约、保存正文/展示渠道、重新通知、撤回和归档；写入沿用 CSRF/Origin，在事务内重新核对本地角色并锁公告行。

POST `/platform/v1/ops/announcements` 直接接受 `announcement_id`、`action` 和对应内容/时间/渠道，返回保存后的完整公告；没有 operation_id、authz_epoch、expected_version、reason、prepare/execute 或影响预览。撤回/归档/重新通知只用浏览器确认。保留净化后的正文预览、不可变内容与通知版本、已读语义、独占展示时间窗口与预约任务；正文/渠道更新不重置已读，重新通知才增加通知版本。

每个成功管理员写入在同一事务写一行 ops.audit_log（ANNOUNCEMENT_*，actor、目标及改前改后）。旧预览和 admin_operations 历史保留，未改动的预约 worker 仍写自己的 SYSTEM 任务记录。沿用 schema 58 与既有运行角色授权，不新增迁移/导入。本切片仅源码与独立 dev 最小验证，不是网站发布或完整联合验收。

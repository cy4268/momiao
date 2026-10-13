# Portal deployment

## Local identity migration (in progress)

Migration 0048 adds `identity.users`, `identity.two_fas`, and `identity.two_fa_backup_codes`. Authentication code is adapted from **unpatched official QuantumNous/new-api, revision f116414284162ad15d8925f7bca494c109b83e93**, not the VPS's momiao login patches. It retains bcrypt hashes, TOTP/backup-code formats and the existing five-attempt factor lockout.

Build `go build ./cmd/momiao-migrate` and `go build ./cmd/momiao-import-native`. Apply schema with the existing migration tool and `MOMIAO_MIGRATION_DSN_FILE`, then run the importer with that target DSN file and `MOMIAO_IMPORT_NATIVE_DSN_FILE` pointing to the native source. Use the isolated dev databases first. The importer reads a consistent read-only source snapshot and writes the three empty target tables in one transaction; it preserves IDs, deleted/disabled users, factor state and consumed backup codes, then advances each identity sequence. User IDs also retain the source sequence high-water mark and existing account references, so deleted users' historical IDs are not reused. The importer locks and checks all three target tables first: any existing row causes the import to fail rather than append or overwrite passwords. Command failures print a fixed message, not connection strings.

The default `-stage identity` reports counts for these three tables only; `-stage keys` is a separate import described below. Neither stage merges Reserve, migrates sessions, imports channels/prices/logs, or switches the running web login. Do not use this partial importer for production cutover. Before running the new local authentication entry point, apply `deploy/sql/runtime-grants-0048-local-identity.psql` as the owner with `platform_runtime_role` and `poker_authority_role`; it grants platform table-level CRUD and Poker SELECT on local users without changing existing business-table grants.

The final `-stage quota` merges each positive Native `users.quota` into Reserve at **1:1 integer units**, using the existing `applyInTx` ledger with `NATIVE_QUOTA_MERGE`. Run it as the database owner only after stopping/draining the old platform, Poker, Native and external wallet writers; the importer checks unresolved quota transfers and API-to-chips exchanges, but does not stop services itself. Zero quota is skipped; negative quota aborts. Native-only users receive zero-initialized local wallet pairs in the same transaction before their credit is applied (the dev root user currently needs this). Native balances are not changed, and no gift/admission event is generated.

The business ID is `native-merge:<user_id>`; the wallet's transport idempotency key zero-pads the decimal user ID to 20 digits to satisfy its existing 16-character minimum without changing shared wallet validation. Replaying the same frozen snapshot reuses the existing entries; a different nonzero amount conflicts instead of adding twice. Output `users`, `zero_quota_users`, and `quota_units` describe the covered source snapshot, not a second credit on replay. Dev has only run this step inside a rolled-back transaction; the actual merge belongs in the new-image cutover, not while the old Native service remains usable.

The local Redis session adapter keeps only user ID, creation time and CSRF token, using the existing `__Host-chaldea_session` cookie and 7-day idle / 30-day absolute lifetimes. Its `chaldea:session:local:*` keys use the existing session Redis role; add `+pexpire` to that role in `bundle/redis/users.acl` and reload Redis before using the adapter. User indexes expire automatically after their newest session's absolute lifetime; old native-backed sessions are not read or migrated. The new HTTP entry point checks Origin/CSRF on writes and reads the local user's enabled status on each protected request. Password login holds the `identity.users` row lock through session issuance; five-minute factor flows are browser-bound, stored separately from login cookies, and included in user-wide revocation. Password/status mutations must use the same row lock when revoking sessions. Business handlers use a typed local user/session context, not native credential headers. The main program shares the wallet PG pool with identity; anonymous login-CSRF sessions expire after 15 minutes. Dev has the 0048/0049 grants on its wallet runtime role. Local password/Discord login, account password/2FA, the frontend, self/Reserve and admission reads are wired; History, Bot, Poker and Ops now use local identity. Native startup/source dependencies are removed, but the integrated dev rollout remains pending. Handler tests use real dev PostgreSQL/Redis but mock Discord provider responses; no website image or live provider acceptance is implied.

For the separate Poker process, set `MOMIAO_POKER_SESSION_REDIS_CONFIG_FILE` to its private `session-redis.json`, using the same Redis address/TLS CA/server name but a dedicated reader credential. Its ACL is `~chaldea:session:local:* -@all +hello +ping +get +pttl`; it neither renews nor writes sessions and does not access login-flow keys. Apply the two `poker_authority_role` statements in the existing 0048 grant template to its authority DSN role: identity schema usage and SELECT on `identity.users`. Platform logout/revocation then immediately invalidates Poker's session reference. Dev has these additive grants and the reader file/configuration prepared; only its Redis was restarted. Recreate Poker with the new image to consume the setting, removing the old Native socket/reader settings in that same image switch. This preparation is not a Poker image rollout or a completed real-game acceptance.

Set `MOMIAO_DISCORD_CONFIG_FILE` to private JSON containing `client_id` and `client_secret`; the callback is always `MOMIAO_PUBLIC_ORIGIN` + `/oauth/discord`. Registration uses the existing `MOMIAO_ADMISSION_ENABLED` flag and additionally requires `guild_id`, `role_id`, and `policy_version`. Only registration requests the `guilds.members.read` scope. Existing Discord accounts use normal login/2FA without membership rechecks or new grants. New members need the configured role; account, account reference, receipt and counter commit together, then the existing local grant worker handles credit. Login and password proof still request only `identify`.

Local Ops maintenance requires migration `0050_local_ops_maintenance.sql` and `deploy/sql/runtime-grants-0050-local-ops.psql` with `platform_runtime_role`. `GET/PUT /api/v1/ops/maintenance` uses the local Cookie, Origin/CSRF and live `identity.users.role >= 10`. PUT accepts `enabled` (required), `message` and `scopes` (omitted means all). Save reuses the existing maintenance tables/admission locks, ends previous active windows and appends one audit row in the same transaction. Old internal reasons are never published; the separate public message is optional. Migration 0050 cancels scheduled maintenance/jobs but retains active windows and historical rows. Scheduling/Fresh execution is removed; the remaining campaign worker keeps running. Ops navigation and the account-menu entry use the local role. All direct-save pages are independent of the removed legacy bootstrap boundary. This is not a completed Ops rollout, and no local-to-native authorization adapter is introduced.

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

Before the integrated dev release, reapply `runtime-grants-0048-local-identity.psql` with the existing platform runtime role and Poker authority-reader role. Add `MOMIAO_POKER_SESSION_REDIS_CONFIG_FILE` to the Poker container, using the same JSON shape as the platform session Redis file (`addr`, `username`, `password`, `ca_file`, `server_name`) and the same Redis database. Give this reader `+hello +ping +get +pttl` on `~chaldea:session:local:*`; no session write permission is needed. The platform reuses its existing session client and wallet pool. Remove `MOMIAO_POKER_SESSION_READER_KEY_FILE`, the Poker container's `MOMIAO_NEWAPI_SOCKET` and their exclusive mounts; keep the existing ticket/lease configuration and service-signing keys. These new grants, reader credentials/ACL and compose edits are deployment prerequisites, not changes already applied by the handler test. That test used existing dev platform session/wallet credentials and real dev ticket Redis, with no table or wallet mutations. The legacy `internal/poker/authbridge` package and its unused BFF revocation RPC are now deleted; live Redis/session and local-user checks remain.

Rankings Ops requires `0059_local_rankings_actors.sql`: job/publication actors reference local users without requiring a Master wallet. Existing 0031 runtime grants remain sufficient. `/api/v1/ops/rankings` and snapshot details use local role >= 10; POST `/rebuilds` queues the existing worker and POST `/snapshots/:id/publish` publishes the candidate, each with one transactional audit row. There is no epoch, client pointer-version check or generic operation receipt. Immutable aggregates, separate build/publish steps, maintenance admission and publication history remain. Jobs and candidates accept `domain`, RFC3339 `from`/`to` (creation time, inclusive/exclusive), and `page` (20 rows); current aggregation state stays visible. Apply only to isolated dev before integrated release; RP/asset source dependencies still need the remaining native cleanup.


Runtime Ops reads require `0063_local_runtime_jobs.sql` and `runtime-grants-0063-local-runtime.psql` for the platform runtime role. `/api/v1/ops/jobs`, `/attention` and `/service-health` now use local role >= 10, without legacy bootstrap or per-queue permissions. Jobs/attention return global `summary` plus paginated metadata `items`, `total`, `page`, `page_size` (20); filters are `kind`, `status`, and inclusive/exclusive RFC3339 `from`/`to` on updated time (announcement jobs use creation time). Summary totals remain unfiltered. The existing eight queues and business status rules are reused, without exposing command payloads or running jobs; obsolete purpose/quota/maintenance/remote rows are labeled legacy, not claimed stopped. Health still reads one latest snapshot for the configured environment, including actual dependency names and expired/unknown state; it is not historical monitoring or full migration acceptance. The unused `/api/v1/ops/native-runtime` proxy is removed. Apply only to isolated dev before the integrated release; the health writer retains its existing behavior and now starts with the platform store and Ops environment, independent of the old session service; the legacy Ops remote dispatcher is removed. Other legacy workers remain until their native dependencies are cleaned up. Migration 0063 retains its applied CRLF bytes through `.gitattributes`, as do the older checksum-preserved migrations.

## Optional wallet database

Wallet reads and explicit zero initialization use a separate PostgreSQL database. Set `MOMIAO_WALLET_DSN_FILE` (absolute regular file, at most 8192 bytes) and `MOMIAO_PUBLIC_ORIGIN` (exact HTTPS origin) together with the existing complete portal configuration. Protect the DSN file with a restricted group and do not print it, put it in command arguments, or commit it. No credentials are bundled.

Use a non-login schema owner and a distinct runtime role. Runtime needs schema USAGE, SELECT on account_refs/wallet_balances/wallet_ledger, INSERT(newapi_user_id) on account_refs and INSERT(newapi_user_id,asset_type) on wallet_balances only. Do not grant wallet UPDATE, balance_units INSERT, ledger mutation, owner membership or DDL. A2a does not expose Apply.

For Unix-socket deployments, protect the socket directory and require SCRAM authentication for **all** local connections. Do not expose a trust-authenticated native PostgreSQL socket through a proxy. An isolated, network-disabled platform PostgreSQL instance avoids changing the native service's authentication or lifecycle.

Build `./cmd/momiao-migrate` as a separate administration tool. It reads only `MOMIAO_MIGRATION_DSN_FILE`, performs explicit checksum-checked migration and prints generic success/failure; it is not invoked by portal startup. Preserve applied SQL bytes (repository LF policy); validate migration twice, restore a backup into a separate temporary database, and test the runtime role's denied operations before enabling the wallet.

The runtime uses a lazy pool: a connection outage returns wallet 503 without preventing portal startup. Invalid configuration remains a startup error. Test database restart, portal restart, preserved initialized balances, and credential-restricted recovery in the actual environment. Such tests are not whole-host reboot or load-capacity evidence. A portal rollback must retain the independent database and initialized users, not delete them.

Wallet paths and payloads: [wallet-api.md](../contracts/wallet-api.md). Local identity and wallet reads share the existing request context; wallet database operations use a 5-second budget. Request reads are separately subject to the HTTP server's 10-second read timeout. These are engineering ceilings, not throughput guarantees.

## Master profile extension

Schema migration 2 adds own Master profiles and append-only name history; the reserved-name baseline is versioned in application code. Never edit already applied migration 1. Master shares the existing protected platform DSN and local identity context. Apply the new migration explicitly with the separate migration tool, then grant only profile-specific SELECT/INSERT/UPDATE columns and history INSERT to the runtime role. Do not widen economic writes, database ownership or credential access.

Back up initialized wallets before migration, migrate twice, and compare all pre-existing identity/economy rows exactly. Test the new application with the runtime role against a separate acceptance database, including name conflict, stale versions, rename cooldown, no-op updates and immutable history. Never choose and permanently save a real user's public display name just to complete a test.

Roll back the portal binary and built assets together if needed, retaining schema 2 and every real profile/history/wallet record. The older migration executable intentionally rejects unknown newer schema versions; it is not a downgrade tool. Verify the previous portal's wallet reads against schema 2 separately. Whole-host recovery, broad moderation and public-game profile gates remain independent acceptance items. See [Master API](../contracts/master-profile-api.md) and [slice decision](decisions/0006-master-profile.md).

## Local portal and Poker transport

Build the Go binary and `web/dist/` from the same source commit. The portal reads local identity/relay/wallet data and uses the existing local Redis sessions. Neither ordinary startup nor recovery-lock readiness opens or probes a Native socket. `/api/v1/rankings/me` uses the local session on both router configurations, never legacy credential headers.

Before deploying the integrated dev image, remove `MOMIAO_NEWAPI_SOCKET` from both platform and Poker environments, and remove only its exclusive mount. Poker still uses `MOMIAO_POKER_REMOTE_SOCKET` with its existing fixed Unix transport, signed requests, peer keys and route deadlines; do not remove that shared business connection. The legacy bootstrap CLI, its grant installer, Native source/adapter/deployment directories and Native-auth web build script are removed. Runtime Ops authority comes from local `identity.users.role`, not old SUPER_ADMIN principals. Preserve historical migration 0008 and its records/functions; do not install new bootstrap grants. The one-time importer remains for migration, and upstream notices live in `docs/upstream/new-api/`. Source changes do not stop workers in the currently running old image.

The ingress may connect to the portal's existing private Unix listener or its configured TCP address. With Unix, set `MOMIAO_LISTEN_SOCKET` instead of `MOMIAO_LISTEN_ADDR`; listener ownership and normal shutdown behavior are unchanged. The model relay needs outbound network access, so do not carry over a Native-only `AF_UNIX` network restriction.

```text
MOMIAO_WEB_DIR=/srv/momiao/current/web
MOMIAO_LISTEN_SOCKET=/run/momiao/portal.sock
MOMIAO_WALLET_DSN_FILE=/run/secrets/wallet.dsn
MOMIAO_PUBLIC_ORIGIN=https://dev.momiao.win
MOMIAO_SESSION_ENABLED=true
MOMIAO_SESSION_REDIS_CONFIG_FILE=/run/secrets/session-redis.json
```

Complete acceptance on the restricted dev hostname first: local login/2FA/Discord, account revocation, existing model key non-streaming/streaming requests with Reserve settlement, local logs, Ops and ongoing games. `/healthz` is liveness and `/readyz` lists configured local dependencies; neither alone proves migration acceptance. No production switch is implied by handler tests. Keep production Native containers and database for the planned rollback until the full dev checklist is complete.

### Schema 3: wallet actions
Apply the explicit migrator once before the portal release, with a database backup for the changed constraints/new daily table. No database restart is needed. Add only these runtime object grants (retain existing wallet/profile grants):

```sql
GRANT UPDATE(balance_units,ledger_seq,version,updated_at) ON economy.wallet_balances TO momiao_wallet;
GRANT SELECT,INSERT ON economy.asset_transactions,economy.wallet_ledger,platform_meta.mutation_idempotency_records TO momiao_wallet;
GRANT USAGE ON SCHEMA rewards,platform_meta TO momiao_wallet;
GRANT SELECT,INSERT ON rewards.daily_checkins TO momiao_wallet;
```
Runtime still has no UPDATE/DELETE on transaction, ledger, key history or daily claims; immutable triggers remain. The prior portal remains compatible with schema 3, so application rollback retains new data without a destructive down migration. For verification use a private clone for positive claims/exchanges; production balances are not test fixtures.

## Schema 4: historical quota transfers

The quota-transfer/refill mutation code and Native quota SQL installer are removed. Keep migration 4 and the existing historical tables/grants; Ops still reads old receipts, but the new program does not create, settle or replay Native transfers. Do not reinstall the deleted Native adapter. Before integrated cutover, resolve historical pending transfers/exchanges and run the planned one-time remaining-quota merge; this source cleanup neither changes balances nor completes that step.

When deploying the new image, remove `MOMIAO_NATIVE_QUOTA_DSN_FILE`, `MOMIAO_NATIVE_QUOTA_KEY_FILE`, `MOMIAO_REFILL_SOCKET`, `MOMIAO_REFILL_KEY_FILE`, `MOMIAO_REFILL_LOW`, `MOMIAO_REFILL_TARGET`, `MOMIAO_REFILL_MAX` and their exclusive mounts. The new configuration ignores these variables and starts no refill listener or quota worker. Keep shared account-lock names, historical transactions and production Native containers/database; the still-running old dev image is not changed by deleting source code.

The activation SPA route and public Native quota/transfer APIs return 404. The wallet and reward pages link to local wallets/API keys instead; exchange availability is only the selected local balance. Old `API_CHIPS_EXCHANGE` receipt metadata remains readable. No schema or runtime grant changes are needed for this deletion.

## Schemas 5-9: historical runtime grants

Keep the applied migrations, historical tables and [runtime grant template](../deploy/sql/runtime-grants-0005-0009.psql) unchanged except for current explanatory comments. Later local-identity/Ops migrations and their grants define the current role-based behavior; the old principal/closure/history tables no longer grant application authority.

The deployment-only `momiao-bootstrap`, bootstrap grant installer and their dedicated tests are removed. Do not recreate a SUPER_ADMIN principal or grant the historical bootstrap function to runtime. Migration 0008 and denial of runtime execution remain, so existing records and rollback data are preserved. The runtime-grant test keeps its profile/admission/notice and denial checks without creating a separate bootstrap deployer; it is not evidence of full current-schema deployment acceptance.

No grants or databases are changed by this source cleanup. Apply the relevant local migration/grant set only in the isolated dev database before the integrated release, verify the actual runtime role, and retain all historical records on application rollback.

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

Master 页签使用 GET `/api/v1/ops/users/:id/master`，POST `/:id/master-rename-required`（`required`）和 `/:id/master-force-rename`（`display_name`）。写入沿用上述角色规则，在同一事务更新资料并写一条审计；复用原昵称规范化、保留名、唯一约束及改名历史，不占用用户七天冷却。强制改名清除改名要求，同值保存正常记审计；未完成 Master 的账号不创建资料或钱包。客户端不传版本，保存后的读取失败只刷新读取，不重放写入。沿用既有 Master 表权限，无新迁移或授权，旧 Master Ops 绑定已删除。

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

### 本地经济与奖励运营

- `/ops/economy`、`/ops/rewards` 由本地 Redis 会话与角色（role >= 10）保护，不读取旧 Ops bootstrap。账变默认全局，支持账户、资产、业务类型与游标分页；账户区展示六项本地统一资产：Reserve、可用筹码、Poker 桌面、Poker 底池、单人游戏在途、轮盘托管，以及合计与观察时间；总览仍仅统计两类钱包余额。
- Reserve 调账复用 `/ops/users/:id` 的原入口；筹码调账提交 `POST /api/v1/ops/users/:id/chips`（amount、note、request_id），复用同一 writer、业务号、角色层级与审计，只改可用筹码。负数保存前浏览器确认，保存后读取失败仅重查，不重放写入；不保留单位数/版本/预览/执行表单。旧额度划转保留只读凭据，删掉恢复旧 native 转账的操作。
- `POST /api/v1/ops/rewards/claims/:id/retry` 仅将原注册奖励任务重新排队；保留已有领取与发放幂等逻辑。管理员角色、claim/job 行锁、更新与 `ops.audit_log` 在一个事务中；已完成的任务或领取拒绝重试。页面收到排队成功后，列表读取失败不自动重放 POST。实际发放金额读 registration_issuances（包括上限裁减为零）。
- 统一资产/救济/奖励与游戏收益上限读取同库六项资产，不观察 Native 额度或旧跨库在途；兑换复用本地双钱包事务，Reserve 不足直接拒绝，旧 API→筹码回执只读。发布前应用 `0064_local_relief_assets.sql`：只放宽旧 relief_claims.native_observed_at 的 NOT NULL，新领取写 NULL、旧额度字段写 0，历史行原样保留，无新增 grant。应先处理完历史 Native 划转/兑换在途并按迁移计划完成额度合并；源码回退保留该兼容性 schema 变更。
- 删除独立 Poker 的反向 Native 经济 RPC 和 `MOMIAO_ECONOMY_READ_SOCKET`；继续保留原平台→Poker 签名命令/票据及同库资产投影函数，游戏/Poker/轮盘算法和资金锁不变。当前 Native 启动、划转/refill 与旧 worker 的其余引用尚未清理，此批不代表整体已脱离 Native。
- 赌怪一次性活动 `/ops/gambler-campaign` 接本地会话、role >= 10、CSRF，GET/POST `/api/v1/ops/economy/gambler-campaigns/01995000-2026-7000-8000-000000000927` 直接读取/保存；不再走 OpsLegacyBoundary、通用审批或二次认证。发布前应用 0065（accepted_by 外键转 identity.users），不新增 grant、不改历史记录。后台任务按本地执行人当前角色、连续维护和冻结资产检查点推进；操作人确认已受理模型请求排空，不把该确认当自动计数。保留原固定活动、独立重置、永久勋章与持久游标；已完成活动不会随部署重复触发。generic Ops 启动已移除；其他 Native 依赖仍待后续清理。

### Poker 运营直接操作

`/ops/poker` 使用本地 cookie/Redis 身份与 CSRF，role >= 10；牌桌/会话列表支持全局游标分页、时间与状态筛选，名称/账户/牌桌是可选条件，详情在弹窗中。`POST /api/v1/ops/poker/tables/:id` 直接提交 `command_type` 与对应参数，服务器绑定当前 actor/table；不再接收审批版本、原因、operation_id 或生成操作回执。成功后刷新，读取失败不重放写入。

保留原 Poker 单桌 actor、手牌边界离桌/关桌、资金处理、恢复与聊天规则，内部签名 Unix RPC 仍承载独立 Poker 进程。只有 USER_TEXT 可以隐藏；牌桌、手牌和玩家的游戏版本/恢复状态仍是游戏业务数据，不是运营审批流程。每次成功管理员写入在同一事务锁本地角色，并写一条 ops.audit_log（前后桌面/玩家/目标禁言与消息状态）；原 poker.audit_events/admin_operations 历史保留，近期事件合并读取新审计。

应用 `0061_local_poker_ops_actor.sql` 后，以 owner 执行 `deploy/sql/runtime-grants-0061-local-poker-ops.psql`，变量 `poker_runtime_role` 为现有 Poker domain writer。仅为该角色增加本地用户 id/role/status/deleted_at 的读取、id 列 UPDATE（供 FOR SHARE 锁使用）及 ops.audit_log SELECT/INSERT，不授予凭据/角色修改。本切片未发布网站，独立 dev 的无资金回退事务不代替在途手牌、观众断开及多进程联合验收。


### 本地支持工单与事件运营

应用 `0062_local_support_incidents.sql` 后，`/api/v1/ops/support-cases` 与 `/api/v1/ops/incidents` 使用本地 Redis 会话和管理员角色。POST 列表路径新建，PUT `/:id` 保存 `next_state`，POST 工单 `/:id/facts` / 事件 `/:id/events` 追加事实或处理记录；写入与 `ops.audit_log` 同事务。列表默认全局，每页 20 项，支持创建时间 `from`（含）/`to`（不含）、状态、关键词，以及工单账户/类别或事件程度。详情保留原核验事实和事件时间线。

工单的核验/处理状态和事件的研判/解决状态仍是业务记录，不会触发账号变更；移除的是通用 Ops 预览审批、客户端版本、原因和回执。历史 operation 引用保留为可空列，旧行和不可变历史仍保留；新写入仅用本地用户外键及现有权限，无新增运行授权。Master 改名管理已并入本地用户运营，不再保留旧绑定。

### 通用 Ops 管道已移除

`/api/v1/ops/bootstrap`、`admin-principals`、`operations/*`、`fresh/*` 和 `/ops/access-control`、`/ops/operations` 不再提供；没有旧路径兼容代理。运营模块保留本地 Cookie/Redis、role >= 10 与写入 CSRF，角色修改仍受 root 规则约束。删除通用审批注册表、细粒度权限、远端派发器、二次验证弹窗和操作回执；保留 `/ops/audit` 的新旧审计联合查询及共用事实展示。

本批无迁移或 grant。旧 admin_operations、权限/审计及任务表保留，不删除或自动执行历史任务；jobs/attention 仍列出旧流程未完成元数据。新代码不启动 Ops remote dispatcher，正在运行的旧 dev 镜像尚未变化。健康快照保持原 15 秒更新/45 秒过期，不隐藏尚存的 Native 依赖；旧 Session/BFF 及内部 fresh 代码已删除。发布与生产切换仍等待完整开发验收。

### Local session startup

Enable local sessions with `MOMIAO_SESSION_ENABLED=true` and `MOMIAO_SESSION_REDIS_CONFIG_FILE`, alongside the existing `MOMIAO_WALLET_DSN_FILE` and `MOMIAO_PUBLIC_ORIGIN`. Authentication shares the platform pool and existing Redis TLS configuration; readiness uses `platform_database` and `session_cache`, not a separate session database. `MOMIAO_OPS_ENVIRONMENT` remains an independent Ops setting. The official f1164142 credential logic is unchanged.

When deploying the new image, remove the obsolete `MOMIAO_SESSION_DSN_FILE`, `MOMIAO_SESSION_READER_KEY_FILE`, `MOMIAO_SESSION_OPS_KEY_FILE`, `MOMIAO_SESSION_SEAL_KEY_FILE`, `MOMIAO_SESSION_CREDENTIAL_SEAL_KEY_FILE`, `MOMIAO_SESSION_OPS_SOCKET`, `MOMIAO_SESSION_ENVIRONMENT` and their exclusive mounts. Do not remove shared mounts or change the still-running old image's configuration. This cleanup has no migration or new grants; historical session bindings and production Native containers/database remain retained.

Both routers return 404 for unregistered `/api/` paths instead of proxying to Native. Recovery-lock auth probes use the same local identity store/Redis client; the existing allowlist still denies credential writes and starts no workers. Old `internal/session`, `internal/bffauth` and Poker authbridge code is deleted. Quota/refill startup is also removed; remaining Native business readers, transport/readiness and deployment dependencies still need cleanup. This source slice does not replace live dev-browser, recovery-process or in-flight Poker acceptance and has not published a website image.

### Local RP usage and rankings

Apply `0066_local_rp_source.sql` before deploying this source. `/api/v1/usage/rp` uses the local session user in both routers; usage and RP rankings read `economy.request_attributions` directly, without a Native checkpoint or sync worker. Keep `MOMIAO_RANKING_ACTIVATION_TIME`; remove `MOMIAO_ATTRIBUTION_SOURCE_INSTANCE_ID` when deploying. The `request_attribution` readiness dependency is removed. Usage `observed_at` is the query time, not a synchronization watermark.

The dev restore contains 29926 duplicate historical requests under different source IDs. Both readers count each `(newapi_user_id, logical_request_id)` once, choosing the latest `imported_at` with `source_instance_id` as tie-breaker; raw historical rows stay unchanged. Migration 0066 retains the existing function signature and EXECUTE ACL, ignores the old source argument, and needs no new grants or reimport. Source rollback leaves this compatible schema change in place. Existing RP scheduling, late-request rebuilds and snapshot history remain; only isolated dev received the migration, and the running site image and production Native services remain unchanged pending integrated acceptance.

### External Discord history bridge

Deploy the Bot bridge version that resolves Discord accounts from Platform `identity.users`, with only `WEBSITE_PLATFORM_DSN`; remove `WEBSITE_NATIVE_DSN` together with the bridge image replacement. Apply `deploy/sql/runtime-grants-bot-local-identity.psql` as schema owner with `bot_reader_role` set to its dedicated read-only role. This grants only the four identity columns and the existing history function, not passwords, 2FA, or wallet writes. The Discord bot itself still receives no database DSN. First validate with an independent dev credential; keep production bridge/Native configuration unchanged until the final cutover.

### Integrated local runtime cutover

Prepare the new platform/Poker image together; keep the old running configuration until cutover. Add `MOMIAO_DISCORD_CONFIG_FILE`, retain the local session Redis and Poker reader configuration, and remove obsolete Native/session-bridge/quota/refill environment entries. Platform now needs the dev egress network for direct model and Discord calls. Remove Native service dependencies from the candidate compose, but retain old containers and databases for rollback. When switching, stop/drain the old writers, perform the quota stage once, then recreate platform **and edge** together (edge shares its network namespace), plus Poker. A candidate configuration or recovery-lock readiness check does not replace the full dev acceptance list.

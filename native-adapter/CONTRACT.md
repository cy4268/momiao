# M2a native admission contract (candidate, 2026-09-06)

Target: official new-api `f116414284162ad15d8925f7bca494c109b83e93` only. Default off. No frontend, platform profile/wallet migration, Reserve grant, quota algorithm, Redis-mode, journal or guard changes. Native increment migration is explicitly PostgreSQL-only and is not added to generic AutoMigrate. All existing behavior remains unchanged while admission is off.

## Public API

All responses use a bounded JSON envelope with stable error codes; token/provider errors never include raw provider bodies. All routes set no-store. Mutations require the configured exact HTTPS public Origin. Private portal-to-native HTTP/Unix transport does not need TLS and forwarding headers are never trusted as authority. Tests exercise the same private HTTP semantics with a synthetic public origin.

| Method | Path | Fixed purpose / authority |
|---|---|---|
| POST | `/api/momiao/auth/discord/login/start` | anonymous login, identify only |
| POST | `/api/momiao/auth/discord/registration/start` | anonymous registration, identify + guilds.members.read |
| POST | `/api/momiao/auth/discord/fresh/start` | native authenticated user + session + auth_version; identify |
| POST | `/api/momiao/auth/discord/password-reset/start` | native authenticated user + session + auth_version; identify; same binding |
| GET | `/api/momiao/auth/discord/callback` | consume saved server purpose; never accept purpose from query |
| POST | `/api/momiao/auth/2fa` | complete an admission challenge with native TOTP/backup-code validation |
| GET | `/api/momiao/account` | native session; safe DTO only |
| POST | `/api/momiao/account/password/set` | native session, empty password, fresh same-binding proof |
| POST | `/api/momiao/account/password/change` | native session and old password |
| POST | `/api/momiao/account/password/reset` | native session + single-use recovery proof, same bound Discord, native 2FA if enabled |

`/oauth/discord` is the portal SPA callback entry, not a direct top-level proxy to native JSON. Discord registers this exact public HTTPS URL. The portal captures code/state in memory, immediately removes them from the address with history.replaceState, then performs an XHR to the native callback. Callback success is a native bundle + HttpOnly refresh cookie; the existing ApiClient applies it only if its logout/account epoch still matches the request. A require_2fa result keeps only the short-lived flow token in memory and submits the native 2FA endpoint. Stable error codes return the user to a retry/login view without provider payloads. No code, state, token or sensitive proof is persisted or added to any URL. Frontend wiring remains M2b.

A 32-byte HttpOnly Secure SameSite=Lax `__Host-momiao_oauth` cookie scoped to `/` binds browser state; native refresh cookies keep their existing scope. The nonce is backed by a ten-minute native auth_flow. Its opaque cookie is retained until that expiry plus the native session TTL (currently 30 days), solely so logout can revoke a session even if its response arrives after the ceremony expires. Authentication still expires in the database after ten minutes. Successful session issuance links the native SID to that browser flow under its transaction lock. The link remains until the session expires, including after native cleanup removes the original ceremony. Logout idempotently closes the ceremony and revokes its linked sessions; a retry can finish interrupted revocation. A delayed callback cannot link a session after closure; a cookie arriving after logout refers to a revoked SID.

Each anonymous login/registration start rotates and closes an existing browser ceremony and revokes its linked sessions. Concurrent rotations of an existing live ceremony have one successor; the loser restarts. Sensitive fresh/reset starts reuse a live ceremony or rotate an expired one, preserving only their separately validated current native session. Native auth_flows hold per-operation 32-byte state, at-most-ten-minute expiry and single-use consumption. Payload contains only browser nonce HMAC, operation UUID, config fingerprint, user/session/auth-version bindings; never code, token or secret. Start routes reject client-selected intents and redirects. JSON accepts only the documented string fields, with exact application/json media type, UTF-8, an 8 KiB bound, no duplicate/unknown keys, and no duplicate authority headers or relevant cookies.

Discord endpoints are fixed to `https://discord.com/oauth2/authorize`, `https://discord.com/api/oauth2/token`, `https://discord.com/api/users/@me`, and `https://discord.com/api/users/@me/guilds/{guild}/member`. Tokens stay server-side. Existing binding lookup precedes admission gates. An unbound login/reset cannot create a user. Existing bound accounts skip guild and role checks even on a registration route. New registration requires the configured guild and role. Missing membership/role, upstream/configuration failures, and throttling have distinct results. Existing native 2FA is required before session issuance or sensitive proof issuance.

## Transaction and recovery

Nonempty native `users.discord_id` is unique across live and soft-deleted records. Migration rejects historical duplicates by aggregate count only and never repairs them. New user, binding, consumed state and immutable receipt commit in one native transaction. New users have empty password, quota zero and no invite side effects. Username conflicts use bounded random-name retries; creation is established by this transaction, never by MaxID.

Receipts contain a UUID operation id, native user id, Discord subject, `NEW_DISCORD_REGISTRATION` source, policy version, timestamp and cursor ordinal. A singleton transactional counter lock serializes receipt ordinal assignment through commit; sequence allocation or timestamps alone are forbidden. UPDATE/DELETE/TRUNCATE are denied by immutable triggers. A failed transaction rolls back account, binding, state and receipt together. A subsequent OAuth login resolves an already committed user after callback/session response loss. A receipt or replayed state never produces an anonymous session.

Password operations lock the user and validate the expected auth_version and session inside the same transaction as password hash mutation and proof consumption. They increment native auth_version, revoke prior sessions, and rotate/create the current native session only from the validated authority. If issuance fails after commit, fresh native authentication is required; old credentials and proofs stay revoked. First set requires an empty password. Change requires the old native password. Reset requires same-binding Discord recovery and any enabled native 2FA.

## Private read API

`GET /internal/momiao/registrations?after=<ordinal>&limit=<1..100>` requires a separate at-least-32-byte private bearer from the restricted configuration file and an actual loopback TCP peer or Unix listener. It must be excluded from public proxy routing. Return receipts in ascending ordinal plus next_cursor. No grant write API is exposed. The migration revokes PUBLIC schema/table access; deployment must separately provision a SELECT-only SQL role if direct SQL reads are selected. This candidate supplies the authenticated read API and creates no platform database role. API and SQL cursor semantics are identical. Endpoint authentication is enforced in native code, independent of proxy ACL and forwarding headers.

## Mode and configuration

`MOMIAO_ADMISSION_ENABLED=false` by default. When true, `MOMIAO_ADMISSION_CONFIG_FILE` supplies client id, secret, exact redirect URI, public origin, guild id, role id, policy version and private reader key. Only local restricted regular files on the Linux deployment target are accepted (no group/other mode bits, no symlinks, bounded size); enabled file loading on Windows fails closed because POSIX mode bits do not prove ACL confidentiality. Windows tests use synthetic in-memory configuration. Values never enter repository artifacts or logs. Invalid configuration or absent migration fails closed. Configuration is immutable for the process lifetime.

Mode guards reject native non-admission registration/admin creation, generic OAuth creation/self-binding, email reset, self username rename, native password update outside these routes, and binding mutations that would violate admission. Native password login, keys/logs/models/channels and relay routes retain their behavior. Bootstrap/setup must already be complete before enabling mode. Mode does not silently enroll historical accounts.

## Verification plan

1. Reproducible source provenance and isolated PostgreSQL/runtime preparation.
2. Failing native model integration tests: uniqueness including deleted users; duplicate migration rejection; atomic rollback; same-subject/state concurrency; immutable receipts; commit-safe pagination; zero native quota/invite effects.
3. Model and migration implementation, then real PostgreSQL green run.
4. Failing handler/service tests: scope/purpose/browser binding; old-user gate bypass; provider errors; replay and response loss; 2FA; password/session races and revocation; private endpoint and alternate-route rejection.
5. Minimal native implementation and green tests, source patch replay/build, candidate commit and review evidence. No production action or claim of full M2/Beta completion.

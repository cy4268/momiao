# Native release gates and sequence

This is a manual review checklist, not an installer. No production command was
run by this preparation batch. Local image gates passed; see acceptance.json.
Fill exact target paths and immutable identities
only from an approved current read-only inspection; historical snapshots may drift.

## 1. Preconditions before a release window

- Record current native image **reference and image ID**, Compose rendering,
  native/namespace/PostgreSQL container labels and IDs, health, namespace inode,
  existing mount ownership and both HTTP/quota proxy guard source hashes.
  Do not dump environment contents, credentials or account records into reports.
- Build the candidate image and record its actual image ID and, if separately
  published, registry digest. Verify both executable hashes inside the image.
  Root must serve the authentic JS asset hash; a placeholder page is not acceptance.
- In isolated local containers, use no external network and disposable synthetic
  databases. Verify root/default-off behavior, static assets, native shutdown,
  all four switches, separate reader secrets, UID/0600 permission rejection and
  shutdown. These container gates passed locally. Historical binary session/2FA
  coverage was preserved, not rerun or relabeled as full-container OAuth proof.
- Preserve `REDIS_CONN_STRING=''`, `BATCH_UPDATE_ENABLED=false`, private
  namespace sharing, SCRAM loopback PostgreSQL transport and both quota journals.
  Do not recreate PostgreSQL, Redis or the namespace to replace only the app.
- The old quota guard intentionally closes for a different image. After image
  acceptance and explicit release approval, change only its exact immutable image
  expectation (and verify the engine image identity), retaining all health/mode/
  namespace/inode checks. No wildcard, tag-only trust or guard disabling. Until
  the identity is approved, the candidate fails closed; the current old-image
  service's existing expectation stays unchanged.

## 2. Secrets, switches and public boundaries

- Admission is off when `MOMIAO_ADMISSION_ENABLED` is unset or `false`.
  Enable only with `true` plus a restricted JSON file at
  `MOMIAO_ADMISSION_CONFIG_FILE`. Validate secure native cookies, existing setup,
  stable session secret, exact HTTPS public origin and callback `/oauth/discord`.
  The required environment key is `SESSION_COOKIE_TRUSTED_URL` (singular).
- Catalog is independent: absent `MOMIAO_CATALOG_CONFIG_FILE` is off, or use a
  restricted file with `enabled=false`. To enable, use `enabled=true` and
  `reader_secret=<independent 64-character hex secret>`; optional
  `public_group=default`. Both configurations load on restart, not hot reload.
- Admission `reader_key` and catalog `reader_secret` must differ. Keep OAuth
  client secret, native session secret and reader credentials in distinct custody.
  No secret goes in the image, build context, command-line flags, options API,
  source, screenshot or report. Reference approved private file paths only.
- Bind each regular file read-only; prohibit symlinks and automatic creation of
  missing host files. **0600 must be readable by the actual container UID 65534**,
  not just host root. Validate ownership/traversal and successful load as UID
  65534 inside the real container; verify 0644 fails. A root-owned 0600 bind mount
  is not adequate for this non-root service. Do not relax to 0644 to fix access.
- Public HTTP ingress must never route `/internal/momiao/registrations` or
  `/internal/momiao/catalog` to the native socket, regardless of method, bearer,
  query or path encoding. Place explicit denials before any catch-all/native
  proxy if one is introduced. Deny normalized exact paths and descendant variants;
  reject ambiguous/double-encoded traversal rather than forwarding it.
- The reviewed portal forwards only `/api/*`, `/v1/*` and exact
  `/pg/chat/completions`, not `/internal/*`. Keep that allowlist. The raw private
  Unix-to-TCP native bridge has no HTTP path filter: its 0600 socket is not a
  public ingress. Confirm the external connector targets the portal socket, not
  this unrestricted native bridge. Test denials externally and through the portal
  with synthetic reader credentials, and confirm native reader handlers were not hit.

## 3. Backup, migration, app replacement and enablement

1. Agree a bounded maintenance window. Stop accepting new admission ceremonies
   and coordinate pending quota operations without changing their stable IDs or
   treating unknown outcomes as failures. Preserve recovery workers/receipts.
2. Record coordinated **current** platform/native backups and a restorable
   snapshot of Compose, guard, restricted config metadata and image identities.
   Keep credentials private. A historical backup is not a current rollback point.
3. Run the standalone admission migrator's default readiness check using a native
   schema-owner DSN supplied privately. It does not apply anything by default.
   A failed check is a gate, not permission to initialize/reset the native database.
4. During explicit migration authorization, check duplicate nonempty Discord IDs
   including soft-deleted users (report only counts); apply the dedicated native
   increment with `--apply`, then recheck. It locks `users` and transactionally
   creates its unique index, receipt counter/table and immutability triggers.
   No automatic duplicate repair, old provisioner rerun or destructive down SQL.
5. Verify native runtime ownership/grants for admission schema reads/inserts and
   counter updates. The quota bridge's restricted runtime role is not the native
   schema owner and must not be broadened to migrate/read passwords. Catalog
   read-only validation does not mean making the entire native application DSN
   read-only; login/session/admission still require native writes.
6. Replace only the native app with both controls off, preserving native data,
   session secret, DB-only flags and the existing namespace/DB services. Pair the
   reviewed exact image expectation with the approved image replacement; any
   mismatch must keep quota transport closed until verified, never be bypassed.
7. Verify health, private HTTP, authentic root assets, quota guard readiness and
   namespace identity before reopening dependent traffic. Check funding recovery
   by observation and synthetic isolated tests, not formal-account credit operations.
8. Enable catalog only after private readers/public exclusions pass. Enable
   admission only after its schema, grants, secure cookies, platform callback UI,
   restricted Discord egress and real OAuth acceptance pass. These are separate
   switches; catalog readiness does not imply admission readiness.

## 4. Restricted Discord egress and real callback acceptance

The reviewed network-isolated target has only a fixed model-upstream bridge.
It is not a Discord route. The native client requests only `discord.com:443`:
`/api/oauth2/token`, `/api/users/@me`, and
`/api/users/@me/guilds/<configured guild>/member`. It uses the default HTTPS
transport and rejects redirects. A separately reviewed fixed-destination egress
bridge is required; do not switch the namespace to general public networking.

The private compatibility review compares a fixed L4 bridge plus exact hostname
mapping with a narrowly allowlisted CONNECT proxy. Whichever is approved must
retain end-to-end TLS hostname/SNI/certificate verification, resolve the fixed
destination outside the isolated namespace, reject any other target/port, and
close on stale namespace, transport failure or certificate failure. No disabled
certificate check, wildcard destination, public proxy or shared reader credential.

Real manual acceptance, with an explicitly permitted test account only:

- HTTPS callback lands on the platform `/oauth/discord` UI, which completes the
  native `/api/momiao/auth/discord/callback` ceremony with the correct browser
  binding. The older portal baseline alone does not implement this UI route.
- Qualified registration checks the configured guild/role; ineligible/cancelled/
  replayed/wrong-browser flows fail without identity creation or secret disclosure.
- Native account creation has zero native quota and one immutable registration
  receipt. Platform initial-credit behavior belongs to its separately approved
  batch; no reward or quota action is automatically authorized by this checklist.
- Existing bound-user login, password operations and native 2FA preserve their
  expected challenge/session behavior. Refresh/logout revoke access; a delayed
  callback or response does not restore the closed session.
- Real browser cookies are Secure/HttpOnly with the intended origin; reader
  credentials never appear in browser requests/storage, callbacks, referrers or logs.
- Stop/restart the isolated candidate and its egress fixture to prove fail-closed
  behavior. Do not use a production outage or a formal account for this test.

## 5. Rollback is code/config rollback, not history erasure

Disable admission and catalog independently and restart the app if needed.
This does not remove identities, changed passwords, native sessions/auth flows,
registration receipts or already consumed platform receipt history. To revert
the binary, restore the reviewed prior image **together with its exact guard
expectation**, retaining DB-only mode, namespace, current databases and stable
session-secret policy. Verify health and guards before reopening traffic.

Keep `momiao_admission` objects, unique binding protection, auth-flow/session
history, platform/native quota journals and pending recovery operations. No
truncation, cascade removal, cache restoration, wholesale old database restore,
quota refund guessing or replaying initial grants. Any data restore requires a
separate coordinated recovery plan accounting for transactions after the backup.

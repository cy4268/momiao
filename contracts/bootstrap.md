# Initial SUPER_ADMIN deployment contract (M4a)

Source: Implementation Spec §§79, 104, 111, 177, 423, 550 and Technical Design §§578–580, 939–940. This is only the deployment-only, one-shot initial administrator. It is not a general administrator CLI, an HTTP bootstrap route, or the full Level 3 Access Control implementation.

## Exact trust chain

1. An operator supplies `--environment`, canonical positive BIGINT `--newapi-user-id`, `--expected-username`, and `--expected-empty`. No account ID is hardcoded.
2. A privately mounted deployment manifest binds environment, actual platform database name, reviewed native source tree, fixed private Unix socket, and the immutable executable build identity. The build identity must be embedded with `-ldflags '-X main.releaseBuild=RELEASE_BUILD'` and exactly match the manifest. Empty/default builds fail closed.
3. For PRODUCTION, display environment/target/username/release/database and require an actual TTY typing `PRODUCTION BOOTSTRAP_SUPER_ADMIN TARGET_ID`. Piped confirmation, a different target, EOF, and missing confirmation fail.
4. Read a short-lived native **dashboard access JWT** and its session ID from a private mounted file. The shared existing portal credential-kind gate rejects PAT, relay tokens, refresh/security-proof credentials, malformed or mismatching claims. These unverified claims do not authenticate or confer role.
5. Use the shared fixed Unix transport to `GET http://unix/api/user/self`, Host `localhost`, no redirects/cookie jar/proxy/fallback. Native signature, lifetime, live-session and user checks authenticate. The returned current DB row must have the exact input ID, exact username, and enabled status `1`. Native role is not decoded or mapped.
6. Only after source verification read the private DSN, connect, and verify `current_database()` matches the independently mounted manifest. Source failures perform no platform DB reads or writes.
7. Begin READ COMMITTED, enter the deployment-only database function, advisory lock plus `ADMIN_PRINCIPAL_SET FOR UPDATE`, require **all** principals absent plus no permanent closure/history/bootstrap audit, ensure only `identity.account_refs`, create one ACTIVE SUPER_ADMIN, append role history and SYSTEM_BOOTSTRAP audit, COMMIT. No wallet, grant, reward, or Master initialization occurs here.

The manifest is **operator-controlled deployment provenance**, not a remotely attested self-response. Deployment acceptance must verify the actual native release artifact and socket namespace/ownership against that manifest. Merely typing the known tree into a manifest does not verify a running native binary.

## Fixed source evidence

Read-only native checkout: `E:/Programs/vps/.codex-tmp/native-integration-20260906/native-src`.

- HEAD `1b19c300b46e025724016420454fe188992ce504`, complete tree `3c15a618fa7a528c06da92de7dbf2f2c843a9162`, fixed upstream `f116414284162ad15d8925f7bca494c109b83e93`.
- `router/api-router.go:91–97`: self route uses `UserAuth`, then `GetSelf`.
- `middleware/auth.go:45–68, 147–181`: dashboard classification calls `ParseDashboardAccessToken`, then `ValidateLoginSession`; the principal ID is server-established. Disabled users fail. PAT is a distinct fallback category, rejected by the shared client credential-kind gate before this request.
- `service/auth_token.go:107–137, 215–235`: native dashboard classifier and signed token parser validate the fixed issuer/audience/use and HS256; the native service, not this tool, owns the signing key.
- `service/auth_session.go:114–133`: session/user/version/status/expiry validation.
- `controller/user.go:483–519`: `GetSelf` uses the authenticated context ID, calls `GetUserById`, and emits exact current `id`, `username`, `status`. Native role is deliberately ignored by this tool.
- `model/user.go:493–504`: direct `DB.Omit("password", "access_token").First(...)` with the model's GORM soft-delete scope. A deleted/nonexistent row yields a failed response, not a usable identity. Reading `GetSelf` does not retrieve passwords or management PATs.

The source code was inspected read-only. Local Unix HTTP tests use an explicitly synthetic server; they prove the client transport and failure behavior, not a fresh execution of the entire native authentication stack. Actual deployed source/session/identity verification remains a release acceptance gate.

## Persistent closure and history

Migration `0008_bootstrap.sql` follows the reviewed contiguous 0001–0007 manifest without editing prior migrations. `ops.bootstrap_closure` is an append-only singleton, set whenever any principal is inserted, regardless of role or ACTIVE/DISABLED status. Migration backfills it from any existing principal or surviving SYSTEM_BOOTSTRAP audit. Bootstrap also checks principal count, any role history, and surviving bootstrap audit.

Disabling/deleting/truncating live principals does not erase that historical fact or reopen bootstrap. Role history and closure intentionally retain immutable historical IDs instead of cascading through a mutable principal FK. Role-history → operation FK is deferred to permit the specified history-before-audit order and require a matching committed audit. UPDATE/DELETE/TRUNCATE are rejected on history and closure. Existing `ops.admin_operations` append-only protection is retained unchanged.

The shared guard trigger serializes principal/scope mutations before principal row locks; it does not replace Level 3 actor/proof/version/last-super-admin validation. Normal runtime is not granted principal/scope mutation or bootstrap EXECUTE. Owner-tampering tests deliberately simulate deletion/disable to verify permanent closure; they are not newly exposed admin operations. Historical administrator existence erased **before** this migration with no surviving principal/audit cannot be reconstructed from absent evidence and requires deployment history review.

Audit actor is SYSTEM with action/details actor SYSTEM_BOOTSTRAP, not the target user masquerading as an administrator actor. Records contain environment, string target ID, expected username, created principal, timestamp, release/build, role-history and operation identities. Credentials, passwords, DSNs, raw source bodies and raw dependency errors are never audit or output fields.

## Inputs and invocation

Three absolute private mounted paths are passed via environment, never raw secrets:

```text
MOMIAO_BOOTSTRAP_DEPLOYMENT_FILE=/run/secrets/bootstrap-deployment.json
MOMIAO_BOOTSTRAP_CREDENTIAL_FILE=/run/secrets/bootstrap-native-session.json
MOMIAO_BOOTSTRAP_DSN_FILE=/run/secrets/bootstrap-platform-dsn
```

Non-secret manifest shape (replace operator-controlled placeholders):

```json
{
  "environment": "PRODUCTION",
  "database": "PLATFORM_DATABASE",
  "native_source_tree": "3c15a618fa7a528c06da92de7dbf2f2c843a9162",
  "native_socket": "/run/momiao/native.sock",
  "release_build": "RELEASE_BUILD"
}
```

Private native credential file shape: `{"access_token":"SHORT_LIVED_DASHBOARD_ACCESS_JWT","session_id":"ITS_SESSION_ID"}`. Do not mount a password, signing key, OAuth secret, management PAT or refresh token. Obtain the session through the existing native login path outside this tool. All test values are synthetic. Linux file mode must exclude group/other access; directory, namespace and ownership isolation are deployment responsibilities. Windows test ACLs do not stand in for Linux production mount verification.

```sh
momiao-bootstrap --environment PRODUCTION --newapi-user-id TARGET_ID \
  --expected-username EXPECTED_USERNAME --expected-empty
```

Successful output is a committed receipt (`admin_principal_id`, `operation_id`, `created_at`). There are no `force`, `reset`, `replace-admin`, role-management, DDL, migration, or HTTP modes.

## Privileges and rollback

See `bootstrap-grants.sql` for an operator-reviewed grant template. The dedicated deployment login must be NOSUPERUSER/NOCREATEDB/NOCREATEROLE/NOINHERIT with no owner/runtime memberships; it gets only database CONNECT, schema ops USAGE and the one bootstrap function's EXECUTE. A separate NOLOGIN function owner gets only the specific schema/table privileges required by this function and its guard/closure triggers. PUBLIC EXECUTE is revoked and search_path is fixed to pg_catalog with schema-qualified tables. Runtime never receives the function's EXECUTE or principal/history/closure DML. Do not run the CLI using a migration, owner, superuser or runtime DSN.

After success or uncertainty: revoke the temporary deployment grant/access and remove the credential mount. Revoke the short-lived native session through existing native controls when appropriate. This tool never refreshes/revokes a session on its own.

`BOOTSTRAP_COMMIT_OUTCOME_UNKNOWN` means the COMMIT or output receipt was lost. Do not change target or attempt a reset. Inspect the immutable audit/closure/role history using a separate operational read authority. A repeat of bootstrap is permanently closed if the prior transaction committed; it creates one set only if the prior transaction definitely rolled back. No automatic retry is performed.

Application rollback leaves migration/history/audit data in place. No down migration/DROP/history rewrite is provided. Subsequent administrators and last-super-admin protection remain the original formal Level 3 Access Control work, not this M4a CLI.

## Release gates still separate

- Complete reviewed migration chain and exact checksums; least-privilege local PostgreSQL verification.
- Actual Linux native/portal release artifacts, private socket ownership/namespace and matching source manifest.
- Fresh target identity, short-lived session, and production TTY confirmation.
- Production function ownership/grants accepted and temporary bootstrap credential removed.
- No production account or platform role is modified by local candidate tests.

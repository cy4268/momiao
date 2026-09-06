# M2a native admission review candidate — 2026-09-06

The fixed-source M2a implementation is ready for the controller's independent code review. It is a native patch, dedicated PostgreSQL migration, Windows/Linux migration utility and native controller test binaries. The candidate is not a full M2/Beta acceptance or a production deployment.

## Source and change boundary

- Official upstream: `QuantumNous/new-api@f116414284162ad15d8925f7bca494c109b83e93`.
- Original archive SHA-256: `e7403fdb02a4b837207d6ad191998856c9da4b4b9d63935de6e1e7e8fad302ec`.
- Local native snapshot: `c2f5d9ddf6a3c451335b00c70712a93ffb178e8c` (local root snapshot, not a claimed upstream Git commit).
- Platform clone base: `da74dc4e8bca89f9fef38e519184db2ab6239c04`, branch `codex/m2-native-admission-20260906`.
- Patch: `patches/0001-momiao-native-admission.patch`, 29 native files, 2993 insertions and 1 deletion. SHA-256: `a3c33db56d7adb2584193ee6f3e2a1e4a548545b20126d71946c13329de23a95`.
- The platform candidate modifies only this `native-adapter` package. It does not change frontend, platform schema/profile/wallet, migration sequencing, quota arithmetic, quota cache mode, journal, guard, relay or channel behavior.

The native patch adds 23 files and touches 6 existing files. New code lives in `controller/momiao_*`, `model/momiao_*`, `service/momiao_*`, `setting/momiao_setting`, `middleware/momiao_*`, the dedicated migration command and router test. Existing changes are the strict JSON wrapper, native transaction-aware auth-flow creation wrapper, login audit classification, startup activation/recovery, conditional route registration and query-log redaction. The manifest lists every file and its before/after normalized hash.

## Implemented acceptance boundaries

| Requirement | Implementation and evidence |
|---|---|
| Purpose and browser authority | Separate server-fixed login/registration/fresh/reset starts; native 32-byte single-use state; ten-minute database expiry; browser nonce HMAC; sensitive native user/SID/auth_version/session-version binding; unknown fields, duplicate JSON/headers/cookies and untrusted Origin fail closed. |
| Official Discord and old users | Fixed authorize/token/user/member endpoints; identify only except registration's identify + guilds.members.read; bound user lookup before membership; unbound login cannot create; eligibility/provider/config/429/5xx classified separately; no real provider tokens stored. |
| Native creation and uniqueness | Unique nonempty Discord binding includes soft-deleted users; aggregate-only duplicate migration rejection; user, binding, state consumption and immutable receipt in one native transaction; bounded username/affiliate collision retry; native initial quota and invite grants remain zero. |
| Recovery and pagination | Immutable UPDATE/DELETE/TRUNCATE receipts with operation UUID; transactional ordinal counter lock held through commit; actual PostgreSQL blocked transaction observed before page advancement; state/receipt replay cannot issue an anonymous session. |
| Native passwords and 2FA | Safe account DTO; empty-password first set, old-password change and same-binding Discord reset; native password hash, auth_version and session fences; exact next-version session advancement; native TOTP and single-use backup codes gate OAuth session and sensitive proof issuance. |
| Delayed responses and logout | Browser flow locked when linking a created SID; logout closes flow and revokes links; revocation retries remain possible after closure or native ceremony cleanup; anonymous restart revokes prior linked SIDs; expired sensitive renewal preserves only independently validated current SID. Opaque cleanup cookie outlives ceremony authority but never authenticates. |
| Private integration | Reader bearer independent of browser/native sessions plus actual loopback/Unix peer check; ascending ordinal pagination; no public or native grant endpoint; no forwarded-header authority assumption. |
| Default-off and logging | Disabled startup requires no secrets/schema; native route existence preserved; enabled mode blocks alternate creation/binding/rename/password bypasses; credential paths omit queries from access logs; inner M2 recovery prevents Gin request/panic dumps on authentication paths. |

## Fresh verification

All raw logs and binaries remain in the private task evidence directory `E:\Programs\vps\.codex-tmp\m2-worker-20260906\evidence`. No raw log, real secret or large binary is committed. Synthetic tokens in negative-test diagnostics are private test fixtures, not real provider/account credentials.

The final replay was performed in a newly initialized isolated repository. `git apply --check`, verbose apply and reverse check succeeded. All 29 normalized source file hashes match the implementation; 28 raw file hashes match and the SQL file differs only by upstream Windows CRLF checkout policy. Hashes and exact build artifacts are in `source-manifest.json` and `verification-summary.json`.

| Raw evidence log | Result |
|---|---|
| `replay-affected-native-regression-v2.log` | Full model/controller/middleware/common/router/M2 setting suites: exit 0; 365 top-level tests pass, 2 pre-existing conditional migration tests skip; all 35 included M2 tests run and pass. Model 14.993 s; controller 7.940 s. |
| `replay-service-auth-regression.log` | Discord and native authentication/session regressions: exit 0; 14 pass, including 2 M2 tests; no skips. |
| `replay-binary-postgres-v2.log` | Executed Windows controller test binary against real PostgreSQL: exit 0; all 15 M2 controller tests pass. |
| `replay-build.log` | Windows/Linux amd64 migration utility and controller test binary: all four build commands exit 0. |
| `service-native-regression.log` | Full changed-source service suite: exit 1; 181 pass, 2 channel statistics tests fail. |
| `service-unmodified-baseline.log` | Same command on untouched fixed source: exit 1; 179 pass, the same 2 channel statistics tests fail with the same counts. |

There are 37 distinct M2 test functions, all executed and passing across the final replay suites. M2 model/controller cases use PostgreSQL 18.6 on the dedicated localhost test database. Native pre-existing tests also use their original SQLite/miniredis fixtures; this does not turn those tests into PostgreSQL acceptance. The two skipped tests are native `TestTokenMigrationFromChar48ToVarchar128MySQL` and `...Postgres`, whose separate environment variables were not supplied; M2 PostgreSQL tests did not skip.

The complete service failures are `TestObserveChannelAffinityUsageCacheByRelayFormat_MixedMode` (expected 2, actual 3) and `...UnsupportedModeKeepsEmpty` (expected 1, actual 4). Identical untouched-source reproduction establishes that this candidate did not introduce them. The cumulative counts are consistent with test isolation/shared cache interference; a definitive channel-module root-cause claim is outside M2a. No channel rewrite or test suppression was added.

Red-to-green logs include model/config/provider/password/API compile-red tests and concrete behavior-red tests: `logout-retry-red.log`, `strict-input-expiry-red.log`, `browser-renewal-red.log`, `native-flow-cleanup-red.log`, followed by their green runs and the final replay suites. `panic-redaction-red.log` records the missing security boundary before implementation; `panic-redaction-green.log` runs complete middleware regression after the fix. Initial harness errors are retained: the first replay test ran before a nested repository was initialized (missing patch directory), and the first direct test-binary invocation had unquoted dotted PowerShell arguments. Corrected runs use the `-v2` logs above; neither initial attempt is counted as acceptance.

## Review and remaining integration gates

1. Controller review of this exact patch and reconciliation with other native candidates that share router/startup hooks. No main merge, push or production mutation has occurred.
2. Root application/image build with authentic upstream frontend assets; this phase delivers the explicitly permitted native test binary alternative. Four binaries compile, the Windows test binary executes, but Linux runtime/file-permission acceptance is not yet claimed.
3. Construct the restricted Linux JSON configuration from separately held external values; verify actual registered Discord `/oauth/discord` callback, guild/role permissions, consent and network responses with real authorized test users. Existing private secret material was not copied into this candidate.
4. M2b SPA callback and epoch integration, account UI, platform profile/receipt consumer, exactly-once 1000 Reserve logic and cross-database recovery. This patch supplies the immutable native receipt and read API only. Keep the platform's existing contiguous migration validation intact.
5. Deployment must enforce the private reader route exclusion and independently provision any SELECT-only SQL role if that alternative is used. Shared local PostgreSQL remains available for M2b; this worker has not stopped it.

After the authorized candidate commit, freeze this package for review. Subsequent work requires the controller's next scoped batch.

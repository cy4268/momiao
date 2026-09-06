# M2a native admission adapter

This directory is a reviewable patch candidate for official new-api commit `f116414284162ad15d8925f7bca494c109b83e93`. It contains the native admission implementation, dedicated PostgreSQL increment and tests, with default-off activation. It does not contain a vendor copy, dependencies, real Discord credentials, production data or deployable frontend assets.

Read [CONTRACT.md](CONTRACT.md) for the frozen API and security contract, [M2a-native-review.md](M2a-native-review.md) for acceptance evidence and outstanding gates, [source-manifest.json](source-manifest.json) for source/patch hashes, and [verification-summary.json](verification-summary.json) for test counts and binary hashes.

## Apply to the fixed source

Use an isolated checkout of the exact upstream commit. Verify the upstream source/archive against the manifest and confirm `git rev-parse --show-toplevel` resolves to that checkout, rather than a parent workspace. An extracted archive needs its own `git init` before applying from inside another Git workspace. A successful exit from `git apply` in a parent repository can otherwise skip files outside its current prefix.

From the isolated upstream root, with the patch's absolute path assigned to `PATCH`:

```sh
git apply --check "$PATCH"
git apply --verbose "$PATCH"
git apply --reverse --check "$PATCH"
```

Compare every changed file with `source-manifest.json`. Content hashes use UTF-8 with CRLF normalized to LF; the upstream `.gitattributes` checks out the SQL file as CRLF on Windows. All 29 normalized file hashes were compared after the recorded replay.

This increment is independent of platform migration numbering. It does not alter the platform migration runner, M1 frontend or any later platform 0005/0006 migrations. Other native patches touching `main.go`, `router/api-router.go`, `middleware/logger.go`, `common/json.go`, `controller/user.go` or `model/auth_flow.go` require integration review and fresh tests.

## Reproduce tests and builds

Use Go 1.27.1 as recorded, with `GOTOOLCHAIN=local` and `GOWORK=off`. The existing upstream go.mod/go.sum are unchanged. Set `MOMIAO_TEST_POSTGRES_DSN` privately to a dedicated PostgreSQL database named `momiao_m2_test` on `127.0.0.1`. Tests intentionally reset only `m2_native_test`, `m2_controller_test` and `momiao_admission` schemas in that test database and coordinate with a PostgreSQL advisory lock. Do not point them at an application database. The M2 database tests skip without this variable, so a skipped run is not acceptance evidence.

```sh
go test ./model ./controller ./middleware ./common ./router ./setting/momiao_setting -run . -count=1 -v
go test ./service -run '^(TestMomiao|TestCreateLoginSession|TestPasswordResetDoesNotClearSessionIssuanceHistory|TestLoginSessionCreateRefreshAndRevoke|TestIndependentRedisSessionRevokeConvergesAfterCacheTTL|TestIndependentRedisAuthVersionAdvanceConvergesAfterCacheTTL|TestUserAuthVersionInvalidatesExistingSession|TestAccessToken|TestDashboardAccessTokenClassification|TestSecurityProof)' -count=1 -v
```

The complete `go test ./service -run . -count=1 -v` run has two pre-existing channel-affinity usage count failures on this Windows environment. The identical command against untouched fixed source reproduces both failures. They are retained in the review record; the full service suite is not reported as passing.

The delivered build commands, run from replayed source with `CGO_ENABLED=0`, `GOARCH=amd64` and `GOOS=windows` or `GOOS=linux`, were:

```sh
go build -trimpath -o <output> ./cmd/momiao-admission-migrate
go test -c -trimpath -o <output> ./controller
```

The Windows controller test binary was also executed against PostgreSQL. In PowerShell, pass Go's dotted flags as whole quoted strings, for example `'-test.run=^TestMomiao' '-test.v' '-test.timeout=60s'`. Linux artifacts were cross-compiled; this record does not claim Linux runtime execution. Root application/image building still requires the authentic upstream web/dist assets and remains an integration gate; no placeholder web assets were created.

## Activation and dedicated migration

Activation is disabled unless `MOMIAO_ADMISSION_ENABLED=true`. For enabled Linux deployment, `MOMIAO_ADMISSION_CONFIG_FILE` points to a local regular restricted JSON file with only these string fields: `client_id`, `client_secret`, `guild_id`, `role_id`, `public_origin`, `redirect_uri`, `policy_version`, `reader_key`. The callback must equal the HTTPS public origin plus `/oauth/discord`; the reader key must be independent and at least 32 bytes. Group/other permissions, symlinks, oversized/ambiguous JSON and enabled Windows file loading are rejected. The file is loaded once, outside native public options, and config errors do not disclose values.

Before enabling, an operator must explicitly run the migration tool with the native schema owner's private `SQL_DSN`. Its default invocation only checks readiness; `--apply` applies the dedicated increment. It does not create native base tables or initialize an account database. Historical duplicate nonempty Discord bindings, including deleted accounts, reject migration with an aggregate count. There is no automatic duplicate repair and no destructive down migration.

The native endpoint `/internal/momiao/registrations` requires the separate reader bearer plus an actual loopback peer/Unix listener. The public proxy must exclude it. The patch grants no platform database role and no credit; platform receipt consumption and Reserve granting belong to M2b. If a direct SQL reader is chosen, provision its SELECT-only role separately during that authorized integration.

Disabling the feature returns routing to native behavior, but is not a rollback of created identities, changed password hashes or immutable receipts. Preserve those records. No production activation or data rollback was performed for this candidate.

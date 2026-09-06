# Pinned native admission + catalog integration

Local candidate only. The reviewed M2a and catalog patches remain byte-for-byte in
their original directories; this package adds one small supplemental patch and
one combined replay entry. No upstream source copy, dependency tree, credentials,
database or compiled assets are included.

## One composition entry

Use PowerShell, Git, and a **disposable, clean source checkout** at upstream
`f116414284162ad15d8925f7bca494c109b83e93`. An archive-derived Git snapshot is also
accepted only when its complete tree matches the manifest. Supply absolute paths:

```powershell
$source = 'ABSOLUTE_NATIVE_CHECKOUT'
$output = 'ABSOLUTE_EVIDENCE_DIRECTORY_OUTSIDE_SOURCE'
& 'ABSOLUTE_APP_CHECKOUT/native-integration/replay.ps1' -NativeSource $source -OutputDirectory $output
# Apply only after successful preflight; changes are staged for review.
& 'ABSOLUTE_APP_CHECKOUT/native-integration/replay.ps1' -NativeSource $source -OutputDirectory $output -Apply
```

Preflight checks the exact repository root, clean worktree/index, pinned source,
all three SHA-256 values, whitespace and the **entire final Git tree** using a
temporary index. It leaves the source and its index untouched. `-Apply` applies
the same checked sequence, stages it, and verifies the resulting tree. It never
fetches, resets, cleans, commits, migrates or installs anything. Repeated
preflight is supported; reapplying to an already modified checkout is rejected.
On an exceptional apply failure, retain the disposable checkout for diagnosis;
there is no automatic destructive rollback.

Order: M2a in full; catalog except `router/api-router.go`; supplemental patch.
The only overlapping production hunk is the router. `main.go` hooks compose
normally. The supplemental production delta is two router lines (catalog import
and registration). Other supplemental lines are integrated acceptance tests and
test-database prefix/credential-redaction assertions. `go.mod`, `go.sum` and
`web/bun.lock` remain unchanged.

## Independent controls and authority

- Admission: `MOMIAO_ADMISSION_ENABLED` and its restricted JSON config file.
- Catalog: independent `MOMIAO_CATALOG_CONFIG_FILE` with `enabled` and
  `reader_secret`. Defaults remain off for both.
- Registration reader and catalog reader use distinct secrets. Neither reader
  accepts the other secret, a native user session, or a native PAT.
- Personal model prices accept a native session, not a PAT or unfinished 2FA
  flow. Native password, 2FA, refresh, logout and security-version fencing remain
  in force. The public proxy must exclude both internal reader paths.
- Linux config files require restrictive permissions (0600 accepted, 0644
  rejected). Enabled Windows config loading intentionally remains rejected.
- With admission off its internal route is absent. The full upstream app serves
  the normal SPA HTML fallback (200), not reader JSON. Catalog's registered
  disabled handlers return 404. Do not change the SPA fallback to fake parity.

The original admission and catalog `CONTRACT.md` files remain authoritative for
their API details. This integration changes no billing, quota guard, DB-only
policy, Redis policy, platform migrations or platform authorization mapping.

## Targeted reproduction after replay

Use the recorded Go 1.27.1 and Bun 1.4.2 without changing dependency locks.
The private test runner must provide synthetic loopback PostgreSQL DSNs for
separate `native_integration_*` databases. No DSN is a command-line argument or
committed file. Tests reset their dedicated schemas; never use an app database.

```powershell
# From $source; supply MOMIAO_TEST_POSTGRES_DSN and MOMIAO_CATALOG_TEST_DSN privately.
go test ./model ./controller ./middleware ./router ./common ./setting/momiao_setting ./internal/momiaocatalog -count=1 -json
go test ./service -run 'Test(CreateLoginSession|PasswordResetDoesNotClearSessionIssuanceHistory|CleanupAuthArtifacts|LoginSessionCreate|IndependentRedis|UserAuthVersion|AccessToken|DashboardAccessToken|SecurityProof|GetRequestAutoGroups|Momiao)' -count=1 -json
go vet ./model ./controller ./middleware ./router ./common ./setting/momiao_setting ./internal/momiaocatalog ./service
```

For the Linux-only router acceptance, provide `MOMIAO_INTEGRATION_TEST_DSN`
privately and run
`go test ./router -run '^TestMomiaoIntegratedFourSwitchesRealLinuxTransportAndSessionFences$' -count=1 -v`
on Linux. This test exercises all four controls, actual Unix-to-loopback HTTP,
independent reader authorities, native 2FA/session lifecycle and file permissions.
Its Windows skip is not Linux acceptance. The ordinary upstream conditional
MySQL/PostgreSQL migration tests retain their own separately configured gates.

For a full root build, first build the authentic frontend in `web` with
`bun install --frozen-lockfile` and `bun run build`, preserving the empty upstream
VERSION value when setting `VITE_REACT_APP_VERSION`. Then use `CGO_ENABLED=0`,
`GOARCH=amd64`, the selected `GOOS`, and `go build -trimpath -o OUTPUT .`.
No embed placeholder counts as full build acceptance. Run the Linux root process
in an isolated network namespace/private SQLite fixture for default-off runtime
checks; upstream root binds all interfaces inside that namespace. This candidate
does not add a Windows listener override or change firewall configuration.

See `REVIEW.md` and `verification-summary.json` for actual evidence and remaining
gates. A local candidate is not a container image, production activation, live
community callback, or completed platform M2/M3 release.

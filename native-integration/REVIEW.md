# Native combined candidate review

## Identity and scope

- Native revision: `f116414284162ad15d8925f7bca494c109b83e93`.
- Reviewed application inputs: M2a `733be2ec7b94b21924f1014484d700a922ac8fb7`,
  catalog `26604b681af246f0d98223f543e52d5b36c0f70c`.
- Both original patch SHA-256 values are preserved in `manifest.json`.
- Supplemental patch: 5 files, 307 additions / 5 deletions; only the router's
  import and registration are production additions. No new dependency/schema.
- Fresh replay produced complete tree
  `3c15a618fa7a528c06da92de7dbf2f2c843a9162`, identical to the tested integration.

## Verified acceptance

| Evidence | Actual result |
|---|---|
| Combined affected packages | 606 pass, 0 fail, 5 skip (391 top-level passes; counts include subtests) |
| Native service auth/group subset | 27 pass, 0 fail, 0 skip (20 top-level passes) |
| Original M2 test functions | All 37 have actual run/pass events, no fail/skip; matched by function name from the immutable M2 patch |
| Linux integrated acceptance | Parent test plus four switch subtests passed; real Unix proxy to loopback TCP, distinct readers, session/PAT rejection, password/2FA/refresh/logout and actual chmod checks |
| PostgreSQL | Isolated M2 model/controller tests and catalog read-only connection passed; the catalog fixture's actual write was rejected by PostgreSQL |
| Fresh replay smoke | 9 pass, 0 fail, 2 expected skip; source visibility/group/session tests and combined route registration |
| Replay entry | Wrong root, dirty tree, changed patch and second apply rejected; two preflights preserved source/index; applied tree exactly matched |
| Vet | All affected packages plus service exited 0 |
| Authentic upstream frontend | Frozen install and build exited 0; 221 files / 57,415,911 bytes; lock unchanged |
| Complete root builds | Windows amd64 and Linux amd64 exited 0 with actual upstream assets |
| Complete target-platform runtime | Linux root in an unprivileged user+network namespace, loopback only, private SQLite; root 200, served JS hash identical to upstream build, default-off catalog 404 and admission SPA fallback preserved |

The combined Windows run's five skips comprise the Linux-only acceptance and
four upstream conditional database migration cases. The Linux acceptance was
separately executed successfully; the four upstream cases remain skipped. The
fresh smoke run intentionally omitted the PostgreSQL DSN and ran on Windows,
so its PostgreSQL and Linux skips do not replace their earlier explicit real
acceptance evidence. Windows **root build** and Linux **root runtime** are
different claims; no Windows full-service runtime is asserted.

Historical complete service runs have two channel-affinity count failures,
reproduced on untouched fixed upstream and both standalone candidates. They are
not caused by this two-line composition and were not rerun or changed here. No
claim is made that the full service suite passes.

## Remaining release gates

Real community OAuth callback/role/account operations; target container-image
assembly and image-level acceptance; explicit production configuration, private
reader secrets/proxy exclusions, native admission migration readiness,
compatibility with the deployed quota guard/DB-only topology, rollback and
release review. Production activation or data rollback is not part of this batch.
Disabling admission is not a reversal of identities/passwords/immutable receipts.

Platform admission UI/receipt consumption/Reserve grants and platform catalog
metadata/UI remain separate batches. Existing production databases, shared
PostgreSQL service, billing, guard, Redis policy, original design documents and
other workspaces were not changed. Raw command logs and binaries remain in the
private batch evidence directory; no secrets or full upstream copy are packaged.

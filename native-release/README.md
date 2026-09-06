# Native Linux release candidate

Local `linux/amd64` image built from application input
`9520310d6d75feb0c30c70aaafe835e83b8ccff2`. No production deployment is implied.

## Exact immutable inputs

- Native clean commit: `1b19c300b46e025724016420454fe188992ce504`;
  tree: `3c15a618fa7a528c06da92de7dbf2f2c843a9162`.
- Both payloads: Go 1.27.1, Linux amd64, CGO off, trimpath; clean VCS metadata.
- Root binary: `76463aae3a51b54577c000e0c4d62ca4d6cc46d5e7c166868553d7db8d3adb1c`.
- Migration binary: `f2ae036113f5fc794ef9c712623d0f846ee40434df6ce10cc1001275812a502a`.
- The original reviewed frontend is reused unchanged: 221 genuine files,
  57,415,911 bytes, embedded in the root binary. No placeholder or dependency upgrade.
- The older root binary with modified VCS metadata remains preserved outside
  this candidate; it is not the release payload.

`manifest.json` pins every context payload and the Dockerfile. The Dockerfile
reuses the exact approved upstream runtime image, replaces `/new-api`, adds the
standalone migrator, and runs as UID/GID 65534. Original patches remain unchanged.

## One build entry

```powershell
& 'ABSOLUTE_APP_CHECKOUT/native-release/prepare.ps1' `
  -NativeBinary 'ABSOLUTE_CLEAN_LINUX_BINARY' `
  -MigratorBinary 'ABSOLUTE_CLEAN_LINUX_MIGRATOR' `
  -OutputDirectory 'ABSOLUTE_NEW_OUTPUT_DIRECTORY' -Build
```

The entry rejects wrong/symlinked inputs and existing output, copies exactly
three files with normalized mtimes and rechecks their hashes. `-Build` requires
an already running Linux engine and the exact base image locally present. It
uses `--pull=false --network=none`, records the actual image ID, and never starts
a daemon, pushes, runs containers, or changes production. Without `-Build`, it
only prepares the context and explicitly records `image_built: false`.

The locally built image is
`sha256:6d7062ca03efec8fd15cf78c1127f3442ff2c9b2a51558b6804489d082d50af7`.
Its engine config object is
`sha256:ea1ca6e8d4ab54a451da5ea305949f6dde6231181730b70834201860b204a766`.
The build uses a containerd manifest index; these identities are distinct. No
registry push has occurred. After transfer, inspect the target engine's exact
image reference, `.Id` and running container `.Image`; do not substitute a tag
or assume different engines expose the same representation.

`compose.native.example.json` is a partial merge template, not a deployment
command. Preserve the actual namespace, data, health check, session environment,
other services and DB-only mode. Both private files must exist before merge.
For the initial off/off deployment, set catalog `enabled=false` and admission
`MOMIAO_ADMISSION_ENABLED=false` independently.

See `acceptance.json` for current local results and `release-checklist.md` for
the bounded production sequence. Existing historical tests are not relabeled
as fresh production or container observations.

`deploy.py` is the bounded native-only deploy/rollback entry; see `DEPLOY.md`.
Its default is an offline plan. Preparation did not execute its production steps.

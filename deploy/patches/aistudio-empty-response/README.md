# AIStudio non-stream empty-response patch

Source: [iBUHub/AIStudioToAPI](https://github.com/iBUHub/AIStudioToAPI), v1.3.7,
revision `db624c24ab111ad0f50308f20f8a75a216dbf873`. The Dockerfile pins the exact
upstream image used for the deployed fix. The patch and upstream-derived context
retain upstream's [CC BY-NC 4.0 license](https://github.com/iBUHub/AIStudioToAPI/blob/db624c24ab111ad0f50308f20f8a75a216dbf873/LICENSE);
they are not relicensed under the portal's AGPL license. Upstream contributors
retain their authorship; the following behavior changes are project modifications.

## Change

All three non-stream protocol converters share the same candidate check:

- Missing candidates without an explicit cause: HTTP 502, not a successful empty
  assistant response with fabricated zero usage.
- `promptFeedback.blockReason`: HTTP 400 with the reported reason.
- An upstream error with a numeric HTTP error code: preserve its status/message.
- Normal text, tool calls and usage conversion remain unchanged.

The response writers reuse their existing error response/tracking helpers.
Streaming, account rotation, retry policy and request timeouts are unchanged.
The warning alone does not establish why Google omitted candidates; the three
historical responses did not retain their original error/feedback bodies.

## Rebuild and verify

Run in Bash from the repository root, with Docker and Git available. Extract only
the two source files from a **stopped** container into a temporary build directory;
do not copy auth files, environment files or data volumes. No runtime dependency
or application installation is added.

```sh
patch_dir="$PWD/deploy/patches/aistudio-empty-response"
build="$(mktemp -d)"
base="$(sed -n 's/^FROM //p' "$patch_dir/Dockerfile")"
mkdir -p "$build/src/core"
container="$(docker create --network none "$base")"
for file in FormatConverter.js RequestHandler.js; do
  docker cp "$container:/app/src/core/$file" "$build/src/core/$file" || exit 1
done
docker rm "$container"
git -C "$build" apply --check "$patch_dir/no-candidate.patch" &&
git -C "$build" apply "$patch_dir/no-candidate.patch" &&
docker build --network none --pull=false -f "$patch_dir/Dockerfile" \
  -t local/aistudio-to-api:empty-response-rebuild "$build" &&
docker run --rm --network none --entrypoint node \
  -v "$patch_dir/CHECK.cjs:/check.cjs:ro" \
  local/aistudio-to-api:empty-response-rebuild /check.cjs
```

`CHECK.cjs` is the existing isolated regression, reused without a new framework.
It drives the real non-stream response writer. The pinned unmodified image fails
the first check (`EMPTY HTTP=200`, exit 1); the patched image returns 502/400/429
for empty/blocked/error responses and preserves normal text/tool usage (exit 0).
The check makes no network or model requests.

Expected patched source SHA-256 values:

| File | SHA-256 |
| --- | --- |
| `FormatConverter.js` | `a223090065205dd7a40a68999dad4bbe74857a8e5c1fb067bd1f5d2d2028e7f7` |
| `RequestHandler.js` | `393f1a9308ad879de419017d7b2372f28105b587c5645e3c20c4002122d127a1` |

## Deployment and rollback

Building this image does not deploy it. The 2026-10-15 deployment changed only the
AIStudio service image; existing auth/data mounts, environment and network mode
were retained. A non-Claude Gemini request through the existing gateway returned
HTTP 200 with a non-empty response and nonzero usage.

To roll back a deployment, restore only that service's image to the pinned `FROM`
image above and recreate that service with `docker compose up -d --no-deps`.
Keep its private compose snapshot and original image; do not restore databases or
replace account/data volumes. To undo the source patch on a disposable copy,
run `git apply -R no-candidate.patch` from that copy's root and compare the original
source hashes. Private deployment evidence and credentials are not part of this
repository.

# Model timeout and upstream error feedback — 2026-10-16

The platform source changes are in `cmd/momiao`, `internal/relay` and
`web/src/api.ts`. `edge-timeouts.patch` carries the matching change for the
existing separately maintained `integration/v1/cmd/b01-runtime` edge source,
following the existing deployment-patch layout. It is incremental to the
already-deployed model/upload routing, not a complete legacy runtime bundle.

## Behavior

- Model business calls and channel tests: 330 seconds.
- Model/channel-test edge write/header budget and browser channel-test budget:
  360 seconds, leaving time to return the platform's timeout response.
- Only exact POST `/api/v1/ops/channels/{positive-numeric-id}/test` joins the
  model route's long edge budget. Ordinary API and upload budgets are unchanged.
- Preserve upstream JSON/SSE error metadata, including 429, with credential
  redaction; distinguish timeout 504, connection failure 502 and cancellation
  499. An already-started SSE response reports the error in an event rather
  than rewriting HTTP headers or pretending that the stream succeeded.
- Channel-test recording gets a separate bounded three-second write after
  business-context expiry. Billing, usage values, auth and retry policy stay
  unchanged.

The companion Vertex adapter's three Python fixes are maintained separately in
the owner's private `vertex2openai` repository, not embedded in this public tree.
Cloudflare settings are unchanged; these budgets cover the site-controlled
layers, not an external CDN's silent-response timeout.

## Apply the edge patch

Use the retained edge source tree containing the two paths in the patch. Verify
its original SHA-256 before applying; do not apply to an unknown runtime version.

| File | Original SHA-256 | Patched SHA-256 |
| --- | --- | --- |
| `main.go` | `3bc3419f8d4b4723f38d115645c718575cc4286af8e56bf98b01fe942c2a399f` | `2b5b47418fb25532f9122c0dff804fccdfe8422349c65a6dc6f63b7c230ba852` |
| `main_test.go` | `6a270c4ca126dbad67fa90270c4fb79d489bae1d25a768bef7feb6032529e399` | `d81334fec767537ae148fca66615ddd54aae2e6a427e7830181942fea78cc637` |

```sh
# Run at that retained source tree's root; PATCH is the absolute patch path.
git apply --check "$PATCH"
git apply "$PATCH"
```

Keep the original source, compose and images. Deploy platform and edge together
when edge shares the platform network namespace. The source publication itself
does not recreate any service or touch account/data volumes.

## Minimal verification and rollback

```sh
go test -count=1 -timeout=15s -v -run '^TestChannelErrorFeedback$' ./internal/relay
cd web && npm test -- src/api.test.ts -t 'reports HTTP'
```

`TestChannelErrorFeedback` is the single new table regression. The frontend
test is an extension of an existing test; no test framework was added. The
existing edge test `TestNewEdgeProxySanitizesForwardingHeaders` was reused on
the retained source with the patched files overlaid into this module. It covers
an 11-second channel-test first byte, a 31-second model stream, cancellation,
ordinary deadlines, upload and forwarding-header stripping.

For source rollback, reverse the edge patch with `git apply -R "$PATCH"` on a
copy and compare the original hashes above. Revert the platform change commit
for platform/browser source. For deployment rollback, restore the saved images
and compose, replacing platform and edge together; no database rollback is
needed. The deployment verification already rehearsed restoration on a copy.

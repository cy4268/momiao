# Family-cover upload ingress allowance

The existing private edge helper keeps a 10-second upstream-header timeout and
15/30-second server read/write deadlines. These can end the catalog's bounded
90-second upload before R2 finishes.

Apply `edge-family-cover-upload.patch` to the private deployment helper source,
not to the portal runtime. Its original `main.go` SHA-256 is
`a4c949cbc961656201b47f620af96e7e5de9b91c70bd5ad307cab1ab9b911346`.
The public application repository intentionally does not include private bundle
provisioning source or credentials.

Only canonical `POST /platform/v1/ops/models/family-covers/upload` without a query
uses a cloned transport with a 95-second response-header timeout and per-request
95-second read/write deadlines. The proxy bounds that request to 9 MiB; the
application still enforces its existing 8 MiB image and metadata limits.
Other requests, forwarded-header stripping, TLS, and the direct Poker WebSocket
route are unchanged. The listener remains the already deployed port 18444.

Reuse the helper's existing two tests; no new test declaration or framework:

```sh
go test -count=1 -run 'Test(NewEdgeProxySanitizesForwardingHeaders|EdgeBoundaryIsExactContainerPublishTarget)$' ./integration/v1/cmd/b01-runtime
```

The existing forwarding-header test now also verifies an 11-second upstream
upload through a TLS edge with two-second default server deadlines. The baseline
fails; the upload-specific allowance passes. The boundary test expectations are
aligned with the pre-existing production port, without changing that listener.

Build the helper from the reviewed private source copy, layer only its binary
onto the pinned existing edge image, verify the image and health, and recreate
only the edge service. Keep the old image and compose for an edge-only rollback;
do not revert the schema, restore a production database, or change Native Bridge.
Keep R2 credentials outside all source archives and images. The platform's upload
connection uses an existing systemd socket relay to its fixed R2 endpoint; image
reads remain on the configured CDN origin.

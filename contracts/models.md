# Models catalog and API Access — M3c

Status: implemented local review candidate; integration and release gates below remain open. Source baseline is M3a
`744000ee37d5ee2583345a1f05c194a3811e50b4`, with the approved M2b schema-only
`eea192e7bc7417a8325d4e59d4281106164d423f` imported as `3339506`.
0001–0006 are immutable; this batch owns 0007. Six original specifications are read-only.

## Implementation and verification order

1. Strict native reader, exact decimal DTO and failure tests; no new dependencies.
2. 0007 catalog metadata/publication/snapshots/availability/identity, atomic sync and
   recovery; independent real PostgreSQL and least-privilege role acceptance.
3. Models-scoped Ops edit/preview/confirm/publish/hide/retire and audit/epoch/concurrency tests.
4. Shared public catalog/detail/home selection, private personal quotes, public API
   Access and selection-only return intent; responsive, keyboard and error states.
5. Go/vet/web/build regression, real local browser + synthetic native evidence and a
   separate approved native integration read. Local commits and review report; no deploy.

## Native boundary

Pinned provider candidate `26604b681af246f0d98223f543e52d5b36c0f70c`, native revision
`f116414284162ad15d8925f7bca494c109b83e93`, patch SHA256
`289bb441d2249a745b38a2400f02ed9b7166dcd5819a9fefd13d437de992b108`.
Only the fixed Unix transport can read `GET /internal/momiao/catalog`, using an
independent 64-hex reader key from a private file. No URL input, root token, PAT,
client credential forwarding, redirect following, raw error logging or internal proxy route.

The complete response is bounded at 2 MiB / 1000 models / 5 seconds. Require exact
schema `momiao.native-catalog.v1`, `public_default_reference`, `native_settlement`,
unique valid opaque textual model IDs (1–255 UTF-8 bytes, trimmed, no controls),
sorted models/endpoints and known complete price shapes. Unknown or missing fields,
duplicates, incomplete bodies, invalid decimals, bad hashes and clock violations
reject the whole batch. SHA256 covers the exact compact `data` bytes; it is not a signature.
Source observation must be UTC and no more than 1 minute ahead / 5 minutes behind
the receive time. Last successful verification and source observation remain distinct.

Decimals remain strings, including explicit zero and tiny positives; missing is never
zero. Prices use API Credit per 1M tokens or per request independently. Conditional
and expression-based prices retain their source semantics. Reference tariffs do not
include all adjustments, tool fees or integer quota rounding and do not guarantee
personal settlement. Endpoints describe configured support, never call health.

Sync uses one database-serialized path for worker and Ops, 5-minute default interval,
10-minute stale warning, 30-minute usage CTA expiry, all explicit bounded config.
Verified complete batches replace current source facts atomically. Failure retains
last-good data and stores only a code-owned reason. Empty success marks prior models
not observed; it does not delete, hide or retire them. Same hash refreshes verification
without inventing metadata/identity versions. New identities begin pending metadata.

## Public, personal and editorial authority

All public discovery consumers use the same published-only projection. Pending,
hidden and retired IDs are indistinguishable from unknown IDs. Published models
retain detail and price history while unavailable. Publication, configuration
availability, freshness and price trust are separate facts.

Metadata is plain text with bounded controlled tags/use cases, nullable context,
explicit special-pricing explanation and managed asset IDs. Publication requires
reviewed identity, family, summary and a truthful price explanation. Native role does
not grant platform authority; Models scope, security epoch and operation version
are rechecked under locks. Writes use server previews, explicit confirmation,
global operation IDs, durable receipts and append-only audit.

Personal quotes use only native browser session credentials, current user group,
and `/api/momiao/catalog/prices?model_id=...`; no user_id or group input is exposed.
Responses are no-store, filtered again to the public identity, and discarded across
session epochs. Auto candidates remain ordered alternatives, never a selected group.

## UI and media

Anonymous and signed-in users share `/models` and encoded opaque `/models/:model`.
Cards: identity, summary, attributes/pricing, 2:3 persona slot, action rail; >1100px
three columns, 421–1100px two, <=420px one. Detail information/persona is about 7/5
on desktop; information leads on mobile. Search covers name and ID. Availability,
recommended, tag/use case, context and same-dimension price filtering/sorting are
required; unknown prices/context are not silently zero.

There are no approved production persona assets in this candidate. A code-owned
manifest accepts only PRODUCTION_READY managed local entries with source, rights,
fallback, focal point and safe-area metadata. Family geometry/glyph preserves the
slot and operation on missing/failed assets. Nine selected persona masters, media
upload/rights consent and production visual QA remain explicit later acceptance work.

`/api/access` is an exact SPA exception before native `/api/*`; other API routes
remain proxied. API Base URL must be explicitly configured. Known native endpoints
must also match the verified portal `/v1/` proxy paths. cURL uses JSON encoding and
POSIX single-quote escaping, always `<YOUR_API_KEY>`, and never makes a model call.
Use This Model produces `/api/access?model_id=<encoded opaque ID>&intent=use` for the
existing login gate. Signed-in users with no key enter Keys, otherwise Access. A
post-login Access continuation only reads key existence; it never creates a key or
makes an inference request. The unintegrated fallback keeps that destination in
React navigation state, while the current baseline login still finishes at dashboard.
Guest selection restoration is therefore an explicit integration gate, not a passed
end-to-end claim in this candidate.

## HTTP and configuration contract

All responses use the existing `{success,data}` or `{success:false,code,message}`
envelope, JSON, `Cache-Control: no-store`, and `X-Content-Type-Options: nosniff`.
Every read is bounded. Public reads do not require credentials. Personal and Ops
reads require the shared dashboard-session classifier plus a current native self
verification; token/PAT/native-role values cannot grant platform permissions.

| Method/path | Purpose |
| --- | --- |
| GET `/platform/v1/models` | Published-only list and count in one repeatable-read snapshot |
| GET `/platform/v1/models/detail?model_id=...` | Published detail, approved vocabulary and explicit API Base URL |
| GET `/platform/v1/models/access-config` | Explicit API Base URL only |
| GET `/platform/v1/models/personal-price?model_id=...` | Current session quote; public identity is checked before and after native read |
| GET `/platform/v1/ops/models` | Authorized catalog, sync state, principal and vocabulary |
| GET `/platform/v1/ops/models/detail?model_id=...` | Authorized unpublished or published metadata |
| POST `/platform/v1/ops/models/prepare` | Server preview for SAVE/PUBLISH/HIDE/RETIRE/SYNC |
| POST `/platform/v1/ops/models/execute` | Confirmed, epoch/version/actor-bound, idempotent transaction |

Public query keys: `q`, `availability`, `family`, `tag`, `use_case`, `recommended=true`,
`unknown_context=true`, `min_context`, `price_dimension`, `min_price`, `max_price`,
`sort`, `offset`, `limit`. Sort values: `recommended`, `name`, `context`, `price`.
Price sort/range requires one explicit supported dimension and its fixed unit.
Missing prices sort last and never satisfy numeric ranges. Unknown context is
nullable and sorts last. Default public page size is 24; maximum is 100. Ops accepts
only `q`, `state`, `offset`, `limit`, with default size 50. Duplicate, empty, malformed,
or unsupported query values fail closed. Detail accepts exactly one `model_id`.

An operation body is `{command}` for prepare and `{command,preview_id,confirmed}`
for execute. The command contains a UUID `operation_id`, integer `authz_epoch`,
`action`, opaque `model_id`, decimal-string `expected_version` and
`expected_catalog_version`, and a required plain-text `reason`. SAVE additionally
contains `metadata`, boolean `recommended`, integer `sort_order`. SYNC targets no
model. Preview lasts ten minutes; success receipts remain durable when the following
GET fails. An uncertain transport outcome keeps the same original command and
operation ID for explicit reconciliation. A source change between preview and
confirmation invalidates SYNC instead of silently approving new content.

| Configuration | Meaning |
| --- | --- |
| `MOMIAO_CATALOG_READER_KEY_FILE` | Optional absolute regular file containing an independent 64-hex secret; POSIX mode must exclude group/other access |
| `MOMIAO_CATALOG_SYNC_INTERVAL` | Default `5m`; bounded `1m`–`24h` |
| `MOMIAO_CATALOG_STALE_AFTER` | Default `10m`; at least the sync interval |
| `MOMIAO_CATALOG_DISABLE_AFTER` | Default `30m`; greater than stale threshold, at most `24h` |
| `MOMIAO_API_BASE_URL` | Optional explicit HTTPS URL whose path is exactly `/v1`, without credentials/query/fragment |

Reader configuration requires the existing platform DSN, WebDir and fixed native
Unix socket. Explicit timing configuration requires a reader key. Startup schedules
one immediate sync followed by the interval; shutdown cancels and joins the worker
before closing the store. The application does not run migrations or bootstrap roles.
With the reader disabled, retained published information remains readable and ages
into the configured CTA cutoff; manual SYNC has no source to read. With no Base URL,
the UI explains that examples are unavailable instead of guessing the portal origin.

Opaque route IDs use one percent-encoded suffix, including encoded slashes and
Unicode. The complete IDs `.` and `..` use reserved raw `~Lg` and `~Li4` suffixes to
avoid browser dot normalization. Literal tildes are percent-encoded, so those model
names remain distinct. API query parameters always carry the original textual ID.
`source_observed_at` records the latest availability observation; `last_seen_at`
records the last observation containing that model's retained price/endpoints. The
detail page labels retained price provenance with `last_seen_at`.

## Least-privilege runtime

0007 is frozen at `60add8b4b876ba030871051425ea830f8ea3cba5`, SQL SHA256
`728b47053eb5ae1745cb076991c0efb60ff9f16d4d530b0a161558d953126eb1`.
Apply through the migration runner with the separate migration owner. Runtime needs
schema USAGE on catalog/ops; SELECT principals/scopes; column-only UPDATE(updated_at)
on principals for row locks; SELECT/INSERT on ops operations/previews; SELECT/INSERT/
UPDATE on current metadata/publication/availability; SELECT/UPDATE on sync state;
SELECT/INSERT on snapshots/attempts/metadata revisions/identity; and only
UPDATE(effective_until) on historical identity. The exact executable grant matrix is
in `internal/platform/catalog_grants_test.go`.

The independent LOGIN-role tests prove sync, publication and detail reads, and deny
DDL, role changes, scope self-grants, audit/snapshot mutation, history deletion and
private profile reads. These are local grant proofs; production roles still require
their deployment owner's verification. Removing application code retains 0001–0007,
all publication/source/identity/audit history and existing grants for later recovery.

## Integration and release limits

M2b application/auth return-intent changes are not yet imported; integrate its exact
candidate and retain the same auth epoch rather than add another identity machine.
Global PostAuth Master/migration/critical-state orchestration still needs joint
acceptance. Synthetic fixtures do not close native session integration or production
grants/media/release gates. Rollback disables reader/worker and reverts application
code while retaining every schema, snapshot, identity and audit row; no down migration.

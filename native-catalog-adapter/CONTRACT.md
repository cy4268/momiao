# Native catalog adapter candidate (M3b)

Pinned upstream: `QuantumNous/new-api` revision `f116414284162ad15d8925f7bca494c109b83e93`. Application base: `d1c0145c182a2e7989962b5be0147333ed8f79e5`. This is a local, opt-in read-only provider candidate, not a deployed API or a complete M3 release. No native billing, quota, session issuance, platform schema or frontend implementation is replaced.

## Installation boundary

Apply `native-catalog.patch` to a **fresh fixed-revision checkout**, then build its normal backend/frontend. Two narrow integration sites (plus their imports):

1. `main.go`: after constructing `http.Server`, call `momiaocatalog.ConfigureServer(srv)` before serving. It preserves an earlier `ConnContext` callback.
2. `router/api-router.go`: after native API middleware registration, call `momiaocatalog.Register(router, apiRouter)`.

M2's authentication adapter is deliberately not included. Both candidates may touch imports/server construction in `main.go` and registration in `router/api-router.go`; resolve these two sites explicitly when composing, retain both callbacks/registrations, then run combined verification. Neither candidate's standalone patch-apply result proves combined acceptance. There are no changes under `native-adapter/`.

By default the APIs answer 404. Opt in by setting `MOMIAO_CATALOG_CONFIG_FILE` to an absolute path to an independent configuration file, read once before serving:

```ini
enabled=false
public_group=default
# To enable, set enabled=true and supply reader_secret as exactly 64 hex characters.
# reader_secret belongs only to the read-only catalog reader, not a native user/root/quota writer.
```

The file is regular, non-symlink (including resolved parents), at most 4096 bytes, and owner-only on Unix (0600/0400). Duplicate/unknown fields, malformed values and non-default public groups fail startup. Windows deployment must enforce its file ACL separately; the local Windows tests do not claim a Unix permission-runtime test. A real production reader secret is not shipped. Rotation/disable takes a native restart. Keep the file out of public web assets, repo history, logs and platform responses.

## Private baseline snapshot

`GET /internal/momiao/catalog`, **no query string**, exact single `Authorization: Bearer <reader_secret>`. Native verifies both actual connection endpoints are loopback/RFC1918/IPv6-private or Unix sockets. It never trusts `Forwarded`, `X-Forwarded-*`, `ClientIP` or request-assigned `RemoteAddr`. The verified current Unix-socket proxy to native loopback TCP is supported; a public peer or listener fails even with a valid secret. A private reverse proxy is not end-user authentication: the independent bearer remains mandatory. Do not add a public portal/API/SPA passthrough for `/internal/`.

Success envelope: `success:true`, `complete:true`, UTC RFC3339Nano `observed_at`, `content_hash:"sha256:..."`, and `data`:

- `schema: momiao.native-catalog.v1`; `basis: public_default_reference`; `billing_authority: native_settlement`.
- `models`: lexically sorted opaque `model_id`, `enabled_configuration`, `native_catalog_visible`, `endpoint_status`, static allowlisted `endpoints`, and `price`.
- Notices: configuration is not call health; missing source entries are not retirement; new entries still require platform metadata/review/publication; extra fees and native integer quota rounding are not included.

The hash is SHA-256 of the exact compact JSON bytes of `data`, using this version's fixed struct field order, sorted model/endpoint arrays and fixed dimension order. It excludes fetch time and is **not** RFC 8785 or a signature. Hash changes with projected content, not native static `pricing_version`. Only the complete default-group enabled-ability projection is covered: neither every native group nor every configured model is claimed. An empty valid source is an explicit complete empty array, not a retirement instruction.

Source reads selected columns from `abilities`, enabled-channel facts and model visibility rules in a bounded repeatable-read, read-only transaction. It uses native price/mode getters under the existing native option read lock. It does not wrap `/api/pricing`; its native `all` shortcut is intentionally not used for default eligibility. Dangling/disabled channels, ambiguous visibility rules, source errors, contention, cancellation and limits fail closed, never silently truncate. No channel key/address/name/weight, vendor data, arbitrary endpoint overrides, expressions or all-group maps are serialized.

Both routes return `Cache-Control: no-store`. Their local boundary rejects duplicate Authorization/New-Api-User headers before native UserAuth and recovers handler/auth panics to a fixed `CATALOG_INTERNAL_ERROR` 503 without logging panic contents, credentials, cookies, URI or expressions; it does not replace global native recovery. Provider limits: 1000 models/group, 20000 selected ability rows/read, 2000 metadata rows/read, 32 authenticated candidate groups, 255-byte valid model IDs, 1024-byte personal query, 4096-byte expression, 5-second source context and 2 MiB serialized response. Source and size failures are HTTP 503 with `success:false`, `complete:false`, stable non-sensitive codes. Native authentication remains the existing middleware and its own timing/cache contract; this provider's source deadline does not change global native auth behavior.

### Price trust matrix

`price.configured` means the effective branch has a real native price/ratio entry (or a nonempty expression), not a successful call, supported feature or guaranteed bill. `status` is `reference`, `conditional` or `unquotable`; a missing amount is unknown, not zero. Native effective float configuration is converted to decimal strings without UI rounding; a tiny positive rate stays positive. Non-finite, negative or >1e12 inputs/products are unquotable.

| Native facts | Projection / confidence |
|---|---|
| Explicit ratio `R`, group factor `G`, text billing surface | Input `2 R G` API_Credit/1M tokens; output `2 R G C`, with **native effective** completion getter (including built-in overrides) |
| Explicit zero ratio/price | Exact `"0"` for that base tariff only; no claim that surcharges are free |
| Missing ratio (native fallback 37.5, including accept-unset behavior) | `configured:false`, `price_not_configured`; never 75 or free |
| Cache-read factor `K` | `2 R G K`; conditional on native-reported billable cache-read tokens, not a support claim |
| Anthropic cache-write factor `W` | 5m `2 R G W`; 1h `2 R G W × 1.6`; exact decimal multiplication |
| Generic cache write | `2 R G W`, conditional on native-reported generic cache-write tokens |
| Omitted cache fields | Native getter defaults are marked `source:native_default`; **every dimension has `support:not_asserted`** |
| User-exclusive group factor | Replaces the base group factor, never multiplies it a second time |
| Fixed per-request price `P` on text surface | `P G` API_Credit/request, `conditional` on a plain text request with no extra multipliers/tools; not a universal image/audio quantity price |
| `tiered_expr` selected | Takes precedence even when a fixed price also exists. Missing/oversized/compile-invalid expression gets its reason. Compile-valid remains `expression_requires_usage`; expression text is never returned or evaluated/probed |
| Unknown billing mode | `mode:unknown`, `unsupported_billing_mode`; raw option value omitted |
| Image/audio/tools/request-dependent adjustments, unknown/mixed-unverified channel billing surfaces | Unquoted or `unsupported_billing_surface`; no inferred per-image/per-second/per-token tariff |
| Hidden native catalog metadata | Entry retained as configuration fact, `native_catalog_visible:false`, prices unquotable; platform must not auto-publish it |

The pinned native accounting constant is 500000 quota/API_Credit, hence the factor 2. A different effective `QuotaPerUnit` produces `unsupported_quota_unit` instead of a stale conversion. API_Credit denotes native USD-denominated accounting units, not a currency exchange quote. Settlement, token-specific restrictions, runtime channel selection, usage normalization, integer rounding/minimums, extra charges and funds remain native authority. No paid model calls are made.

Endpoint projection is a configured static subset, not a routing/health probe: only `/v1/chat/completions`, `/v1/responses`, `/v1/messages`, `/v1/images/generations`, all POST. Unknown provider/endpoint kinds are not advertised. The provider whitelist is pinned in code; adding support needs its own evidence, not a default fallback.

## Session-only personal quote

`GET /api/momiao/catalog/prices?model_id=<encoded opaque model ID>[&group=<native selection>]` uses native `UserAuth` **and** `GetSessionAuthIdentity`. A reader secret, relay API token, anonymous request or dashboard PAT is not a browser session. Client user IDs and other query keys are rejected; a supplied `New-Api-User` must match the authenticated native identity.

The source checks the authenticated user's native group, `GetUserUsableGroups` and `IsUserSelectableGroup` under one native option lock. Omitted group means that user's current native group; it does not claim to resolve a relay token's chosen group. Explicit group must be available to that user. `group=auto` also requires permission to select auto, then returns the complete ordered `GetUserAutoGroup` eligible candidate set (up to the hard bound). It does not consult/modify a relay token or invent an actual selected group.

The private `no-store` response contains the requested `model_id`, fetch time, native authority, safe `quotes` with response-local 1-based `candidate` ordinals, optional prices/configuration facts and an explicit `basis`. Auto basis is `eligible_auto_candidates_not_selected`; other bases explicitly say `reference_not_token_selection`. A candidate without that enabled model is marked with a reason, not assigned a zero price. No native group name/description, user ID, personal factor or all-group configuration is returned. The ordinal is not a stable selector. The optional native group request parameter is an integration input, not a label to publish. Consumers must not merge these prices into the public shared snapshot/cache. Listing safe public group labels/selection UX belongs to later platform integration.

## Verification and remaining work

PowerShell 7 entry: `./verify.ps1 -NativeSource <fresh source> -OutputDirectory <private evidence directory> -GoExecutable <go> -Apply -BackendFixture`. It checks the patch hash and pinned checkout/snapshot tree, applies once, runs module/middleware/router tests, native session/group-focused service tests, vet and a backend build. `-BackendFixture` only permits a clearly marked synthetic `web/dist/index.html` for Go embedding when no frontend build exists; that binary is **not** a deployable full UI image.

Set `MOMIAO_CATALOG_TEST_DSN` only in the process environment to run the PostgreSQL test. It requires a fresh loopback-only `m3_catalog_*` database; the test creates synthetic records there, then verifies both HTTP routes on a real PostgreSQL connection with server-enforced read-only transactions, including rejection of a write. It neither creates a cluster nor touches another database. Optional `MOMIAO_CATALOG_TEST_SNAPSHOT_FILE` writes the synthetic safe snapshot for private evidence. No test credential belongs in command arguments or committed artifacts.

Local evidence includes red/green tests, real direct Unix and Unix→TCP HTTP chains, SQLite and PostgreSQL read-only integration, and a Linux/amd64 cross-build (compilation only, not a Linux runtime acceptance). The broad upstream `go test ./service` is **not green** on the local Windows runtime: two `TestObserveChannelAffinityUsageCacheByRelayFormat_*` tests failed both with and without this adapter (timestamp-keyed cache-statistics fixtures; the root cause is not changed by this candidate). They are outside this module and are not modified; the relevant session/group subset passes. The verification entry does not conceal this broader-suite limitation or claim all upstream tests pass. MySQL live runtime, Linux owner-mode runtime, combined M2/M3 integration and the full production image are not yet validated.

Still outside this candidate: portal reader/configuration and public response handler, platform metadata/publication workflow, public/catalog UI and personal quote UX, refresh/last-good policy, endpoint proxy review, production secret provisioning, release/deploy/rollback rehearsal, and full M3 acceptance. Disable the opt-in file and restart to turn off this candidate; reverse the standalone patch only on a matching tree or reconcile the two integration sites after composition. No data migration or billing rollback is required.

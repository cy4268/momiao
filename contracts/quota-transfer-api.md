# Reserve → native Active quota

Current slice: explicit, one-way, fee-free transfer from Reserve into native API quota. No automatic refill, reverse transfer, ordinary credit-purchase shop or game payout. `/wallet/activate` uses the existing authenticated portal; opening it never moves assets.

## HTTP

All routes verify the current native identity through the fixed upstream. Body/query user IDs are rejected. Responses are `no-store`, with decimal-string IDs/units and exact decimal-string Credit amounts.

| Route | Result |
|---|---|
| `GET /platform/v1/native-quota` | Own `user_id`, `raw_quota`, `amount`, bridge `enabled` |
| `GET /platform/v1/quota-transfers` | Latest 20 own receipts; not a complete-history export |
| `GET /platform/v1/quota-transfers/by-key?key=UUID` | Own original receipt or `null` |
| `POST /platform/v1/quota-transfers` | `202` durable intent; same-key replay `200` original receipt |

POST body: exactly `{"idempotency_key":"lowercase UUID","amount":"100"}`. Requires exact configured HTTPS Origin, JSON content type, max 2048 UTF-8 bytes, no duplicate/unknown fields. Positive exact decimal with at most six fractional digits; no rounding. 500,000 atomic units = 1 API Credit; native raw units map 1:1. Technical request/target ceiling: 9,007,199,254,740,991 raw units (current native browser precision), not a business entitlement or load limit.

Receipt fields: `id`, `user_id`, `amount_units`, `amount`, `status`, `reason`, `native_before`, `native_after`, `created_at`, `updated_at`. Before/after are nullable signed integer strings. For CONFIRMED, after = before + requested units; they describe the historical target transaction, not the current balance after subsequent API usage.

States:
- `PENDING`: source debit and durable intent committed; a worker continues independently of the browser.
- `CONFIRMED`: target-local journal confirms the exact addition.
- `REFUNDED`: target durably rejected the operation and the exact Reserve debit was refunded atomically with local completion.
- `NEEDS_REVIEW`: target rejected, but local refund hit a storage bound. Original operation remains locked for operator reconciliation; never imply refunded.

`409`: insufficient Reserve, wallet absent, another unresolved transfer, or same key/different amount. `400`: invalid fields/amount; `401/403`: native identity/origin rejection; `405/415`: method/content type; `503`: unavailable dependency/configuration. Errors contain only stable codes, not database details.

The browser persists the non-secret original key and amount before POST. Uncertain responses lock new submissions; explicit GET lookup precedes any same-key retry. Only GET polling is automatic. A confirmed same-key receipt remains replayable when new transfers are disabled. One unresolved transfer per account prevents accumulating ambiguous transfers.

## Source-backed target adapter

Pinned native revision: `f116414284162ad15d8925f7bca494c109b83e93`; image digest `sha256:54a0b10924aa75fa5b5947208b820ced66b6ef4b445b35f122b31d80676aba2b`.

Native `model/user.go` quota mutators have no operation-ID journal and may write Redis asynchronously. `model/quota_reserve.go` uses a conditional database UPDATE when Redis is disabled. `common/redis.go` supports an empty `REDIS_CONN_STRING`; `main.go` enables batch accounting only for `BATCH_UPDATE_ENABLED=true`.

This adapter therefore requires **Redis disabled and batch updates disabled** on the pinned native app. No native fork is built. `internal/platform/native_quota.sql` installs a default-disabled, separate `momiao_quota` schema **inside native PostgreSQL**, explicitly, never as portal startup or a platform migration. Its restricted functions expose user-ID reads and positive credits. UUID journal and native `users.quota` update share one transaction and row lock; repeated UUIDs return the original result, conflicting fingerprints fail. Definitive account/source/overflow failures are journaled too. Network errors are not interpreted as failure receipts.

Platform schema 4 commits Reserve debit + transfer intent in one transaction. The worker first queries the original native operation; absent receipt may be executed with the same UUID. Native commit followed by local failure is recovered from that same receipt, never by another debit or a new target ID. Target-outcome uncertainty stays pending.

## Verification and remaining boundary

Real PostgreSQL tests cover debit/replay/conflict, one pending per user, target commit plus local completion failure, concurrent workers, direct consumption racing with credits, durable rejection/refund, disabled target recovery, overflow and immutable history. HTTP/UI tests cover strict amounts, own identity, uncertain response preservation and no automatic POST.

Deployment acceptance uses two isolated PostgreSQL databases, a clone of the pinned native image in DB-only mode and restricted runtime roles. It verifies daily 500 → transfer 100 → Reserve 400 → native self sees 50,000,000 units; replay adds nothing. One isolated real model request succeeds and consumes quota. Formal balances stay unchanged. This is functional evidence, not throughput or whole-host recovery evidence.

Native version, quota scale or accounting-mode changes require source/runtime reverification before enabling this adapter. Reverse transfer, automatic refill, game settlement and a general manual-review tool remain separate work.

# Wallet actions / daily reward v1

Uses the existing fixed native authentication and `/platform/v1` namespace. GET is read-only. POST requires the exact configured Origin and JSON content type; bounded 2 KiB JSON rejects unknown/duplicate/null/non-string fields. No caller-supplied user, day, reward amount, policy or eligibility.

| Method | Path | Body/query |
|---|---|---|
| GET | /platform/v1/rewards/daily | no query |
| POST | /platform/v1/rewards/daily/claim | `{idempotency_key: UUID}` |
| POST | /platform/v1/wallet/exchange | `{idempotency_key: UUID, from_asset: RESERVE_API_CREDIT or AVAILABLE_CHIPS, amount: decimal string}` |
| GET | /platform/v1/transactions | optional `after_id` UUID; 20 newest-first records, `has_more` / `next_after_id` |
| GET | /platform/v1/transactions/by-key | `kind=DAILY or EXCHANGE`, `key=UUID`; own confirmed receipt or null |

All UUID request keys use lowercase hex and are scoped to the verified user and operation family. Keep the same key and payload after unknown outcomes. Lookup before an explicit retry; null is not permission to replace the key while the original request may still commit. The page stores only the pending request (not credentials) in this tab's sessionStorage and restores its lock after reload.

Daily: 500 API Credit = 250000000 units into Reserve, once per database-clock Asia/Shanghai natural day. Same-key retries after midnight return the original claim; a new explicit claim uses the current server date. Fixed policy v1, no randomness. Wallet initialization is required but never silently performed. This slice consolidates daily claim, policy snapshot and issuance receipt in one immutable `rewards.daily_checkins` row, linked to the same-transaction ledger/asset transaction rather than building unused policy/worker tables.

Exchange: Reserve ↔ Available Chips only, 1:1, zero fee, exact positive multiples of 0.000002. Both wallet row locks are taken in asset order; the debit, credit, two ledger legs, transaction and key receipt commit or roll back together. No NewAPI Active quota change or cross-database transfer. Duplicate key with changed semantics returns 409. A receipt is CONFIRMED only after commit; failed local transactions leave no economic effect.

Receipt fields: `id`, `user_id` (decimal string), `biz_id`, `kind` (DAILY_REWARD/LOCAL_EXCHANGE), `status` (CONFIRMED), `from_asset` (empty for daily), `to_asset`, `amount_units` (decimal string), `amount` (exact decimal string), `created_at`, `confirmed_at` (UTC). Daily status also provides business_date, timezone, next_reset_at, claimed and transaction_id.

Errors: 400 INVALID_REQUEST / AMOUNT_INVALID / AMOUNT_NOT_REPRESENTABLE / AMOUNT_OUT_OF_RANGE; 401 AUTH_UNAUTHORIZED; 403 AUTH_FORBIDDEN / ORIGIN_REJECTED; 405 METHOD_NOT_ALLOWED; 415 INVALID_CONTENT_TYPE; 409 INSUFFICIENT_BALANCE / IDEMPOTENCY_CONFLICT / WALLET_NOT_INITIALIZED / BALANCE_OVERFLOW; 502 AUTH_UNAVAILABLE; 503 ECONOMY_UNAVAILABLE. Responses use no-store and generic server messages. All lookup/history results are owner-filtered.

Registration/migration initial grants remain tied to their actual eligibility flows. Hourly/relief unresolved fields stay inactive. No top-up shop, ordinary credit purchases, user transfers or game wagering are introduced.

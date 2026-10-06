# Private Discord Blackjack adapter

This adapter adds Blackjack to the existing authenticated Unix-only Bot Games
listener. It uses the website's existing `games.Service` for commitments, deals,
HIT/STAND/DOUBLE/SPLIT, wallet mutations, typed action history, recovery and the
24-hour inactivity worker. It introduces no engine, RNG, wallet, schema,
dependency, migration, public route or production configuration change.

## Private API

All operations are `POST /internal/v1/bot-games/blackjack/<operation>`. The
existing bearer token, one `X-Discord-User` subject, exact string-only JSON,
4096-byte body cap, forbidden credential headers, inflight limit and 8-second
handler deadline remain unchanged.

| Operation | Exact body fields | Result |
| --- | --- | --- |
| `prepare` | `request_id`, `initial_wager` | `BlackjackPrepared` |
| `play` | `quote` | `BlackjackRound` |
| `lookup` | `quote` | `BlackjackRound` |
| `state` | `quote` | `BlackjackRound` |
| `action` | `quote`, `request_id`, `action_type` | `BlackjackActionResult` |
| `action-lookup` | `quote`, `request_id`, `action_type` | `BlackjackActionResult` |

`request_id` is the original Discord interaction's canonical uint64 decimal
snowflake. Action types are exactly HIT, STAND, DOUBLE and SPLIT. Only `play`
and `action` can create a game transition. An uncertain response to either is
reconciled through the corresponding original-operation lookup, not an automatic
write retry. State/lookup can use expired tokens; absent expired writes fail.

### Preview and resume

`prepare` resolves the account and bootstraps Blackjack. An existing active
round returns exactly `{action: "RESUME", round: ...}` after a fresh read of
that same round. The supplied wager is ignored on this branch, including during
maintenance. No commitment or price for a second wager is exposed.

Otherwise, PLAY and the current frozen config, wager policy and commitment must
agree. Both Blackjack V1 and V2 are supported, including V2 fair return. The
preview returns DEAL, an opaque signed quote, initial wager, stake, current
available units, minimum/maximum units, ruleset/text, seed hash, commitment ID
and expiry. All amounts are decimal strings. The initial wager is whole chips
and at least 10. The maximum, floored to whole chips, is:

```
min(balance, MaxInt64 / 17, (MaxInt64 - balance) / 16,
    matching economic policy's single-player cap)
```

`play` binds the original key `discord-blackjack-v1:<request_id>` and canonical
`CreateInput{Type: "BLACKJACK", InitialWager: ...}`. It checks the durable original
before expiry, then calls the existing transaction once if absent and unexpired.
Another active round returns BLACKJACK_ACTIVE_ROUND rather than silently changing
the target. `lookup` never creates a missing round.

### Signed snapshots and actions

The HMAC domains are independent of the four earlier game adapters:

- quote: `bot-games.blackjack.quote.v1\0`
- deterministic action ID: `bot-games.blackjack.action-id.v1\0`

DEAL and ROUND tokens have disjoint exact canonical JSON fields. Common fields
include game, kind, subject/account binding, initial wager and fixed 120-second
issued/expiry times. DEAL binds request and commitment; ROUND binds round,
version, active hand, total stake and active-hand stake. Empty terminal hand IDs
and zero active stakes are explicit. Money, version and Unix times are canonical
decimal strings. Unknown, duplicate, null, case-aliased and noncanonical payloads
are rejected after authentication. Mutable record IDs are canonical UUIDv7.

Action UUIDv7 is derived from subject plus the originating interaction only,
never type/hand/version/round. Its timestamp prefix comes from the snowflake.
Reusing one interaction with different semantics therefore conflicts. The signed
snapshot supplies the hand/version; clients cannot submit those authority fields.

`FindBlackjackActionMatching` is the only addition to the website games service.
It shares the existing user/game lock, checks the original round and request hash,
and returns the durable original response or nil when absent. It never executes
an absent action. Existing `FindBlackjackAction` semantics remain unchanged.

The adapter first performs matching lookup before checking token expiry or
attempting an action. An accepted operation yields a durable receipt and then a
fresh read of the **same round**. The receipt requires original version = expected
+ 1 and exact added stake: zero for HIT/STAND, signed active-hand stake for
DOUBLE/SPLIT. A later website action is never treated as this action's receipt.
A failed read after commit returns an unavailable response for lookup recovery,
not a second write.

### Projection and financial meaning

Only PLAYER_TURN and SETTLED with NORMAL recovery state are projected. Cards
are public codes 0..51, never deck-copy IDs. Hidden dealer cards/total, shoe,
future cards, seeds, encrypted authority and internal transaction IDs are absent.
Hand indexes are sparse and stable (e.g. 0, 1, 2, 4), with at most four hands.
All specified arrays, empty results, null totals and settlement fields are
explicit, including immediate-natural timestamps and terminal empty controls.

Fresh legal actions come from the website read using the **current wallet**.
DOUBLE/SPLIT are additionally filtered by the frozen canonical economic cap.
Historical `BalanceAfterUnits` is not exposed as current available balance.

Settlement separates raw per-hand results and raw whole-round WIN/LOSS/BREAK_EVEN
from gross payout, V2 fair return, withheld profit, credited payout and actual
net. Gross equals hand payouts plus fair return. Actual net equals credited
payout minus all stakes. No single-step wallet identity is imposed on a multi-step
round that can span unrelated wallet transactions. A raw WIN can be actual net
zero at the economic cap.

The Bot owns private owner confirmation and an unextended 120-second active-view
window. Expiring or cancelling UI controls never stands/refunds/forfeits. Re-run
`/blackjack` or use the website to resume. The website's successful-action +24h
inactivity handling, maintenance recovery and history/share path are unchanged.

## Errors and tests

New 409 codes are BLACKJACK_ACTIVE_ROUND, BLACKJACK_STALE_STATE,
BLACKJACK_ACTION_NOT_ALLOWED and BLACKJACK_NEEDS_REVIEW. DOUBLE/SPLIT balance
refusals use existing INSUFFICIENT_CHIPS. The validated action path maps the
engine's cumulative stake-cap refusal to BLACKJACK_ACTION_NOT_ALLOWED. All four
legacy adapters keep their original routes and behavior.

Run `go test ./... -count=1 -json` with the repository toolchain. New unit/HTTP
tests live in `internal/botgames/blackjack_*_test.go` and
`cmd/momiao/bot_blackjack_http_test.go`. Matching lookup's real PostgreSQL test is
`TestBlackjackMatchingLookupOriginalConflictAbsent` in `internal/games`.

`TestBotBlackjackPostgres` requires a **new empty**, explicitly declared loopback
PostgreSQL database on port 55432 named `momiao_test_g1_original_blackjack_*` via
`MOMIAO_GAMES_TEST_CONNECTION_FILE`. It runs the real migrations, current V2 config,
existing runtime grants and actual engine/ledger. Protected test-only precommit
construction selects reproducible natural, split and ordinary-hit cases before
preview; there is no production seed override. Each run preserves its database
and evidence; use another fresh database for reruns. The integration covers
multi-client continuity, same-ID concurrency, stale versions, restart/expiry,
read loss after commit, absent lookup, hidden-card projection, economic clipping,
current-wallet controls and all four earlier adapters.

# Private Discord scratch bridge

This additive contract uses the existing authenticated Unix listener, account
resolver, wallet, ledger, historical rounds, scratch configuration and engine.
It adds no settings, public routes, production tables, migrations or website UI.
The implementation entry points are `ScratchPrepareInput`, `ScratchPrepared`,
`ScratchResult`, `PrepareScratch`, `PlayScratch` and `LookupScratch`.

## Requests and previews

Three private POST paths share the existing strict 4096-byte JSON boundary,
authority headers, eight-second deadline and 16-request admission limit:

- `/internal/v1/bot-games/scratch/prepare`: exactly `request_id,wager`, both
  canonical positive decimal strings. Whole Chip wager is at least 10; 11 is valid.
- `/internal/v1/bot-games/scratch/play`: exactly `quote`.
- `/internal/v1/bot-games/scratch/lookup`: exactly `quote`.

Every request re-resolves the current Discord binding and existing account gates.
Prepare calls `Bootstrap(user,"scratch")`; it never creates or reveals a round.

### PURCHASE

Exact response keys:

```text
action quote wager stake_units additional_stake_units available_units
minimum_wager_units maximum_wager_units prizes ruleset rules_text
server_seed_hash commitment_id expires_at
```

The two stake fields equal wager × 500000. All monetary values are nonnegative
int64 decimal strings. Purchase requires PLAY, no pending ScratchBlocker and a
valid next commitment. Configuration schema/rules/algorithm must be
`scratch-config-v1` / `scratch-rules-v1` / `scratch-map-v1`. Reconstructing
`games.NewScratchConfig` validates the actual prize-table resource identity and
hash without consuming any RNG. All eight tiers are projected, including zero
weights; current valid order and weights are retained, not replaced by defaults.

Maximum stake is the whole-Chip floor of the minimum of current balance,
MaxInt64/100, (MaxInt64−balance)/99 and the active economic single-player limit.
This bounds both gross payout and post-settlement balance before RNG. The real
Create transaction rechecks funds, maintenance, commitment, limits and blockers.

### RESUME

Exact response keys:

```text
action quote wager stake_units additional_stake_units round_id created_at
ruleset rules_text expires_at
```

The submitted wager is validated, but the response describes the OLD owned,
settled, still-unrevealed ticket and its historical wager. Additional stake is
`"0"`. No current balance, odds, wager range or commitment is exposed. The notice
states: 已有未揭晓券，本次只揭晓原券，不再次扣款；本次输入的筹码不会用于购买新券.
Historical typed cells, tier, reward and accounting are validated without
including them in the preview. Bootstrap's RESUME remains available during a
game maintenance/disabled state, without allowing a new purchase.

## Signed authority and two transactions

The HMAC domain is `bot-games.scratch.quote.v1\0`. Exact payload fields are
`v,game,action,request_id,subject,binding,wager,commitment_id,round_id,iat,exp`.
Version is 1, game is `scratch`, and expiry is issue+120 seconds. PURCHASE has
only a nonempty commitment ID; RESUME has only a nonempty round ID. All four
games' signature domains are mutually isolated. Duplicate/unknown/null/aliased
payload fields and changed account bindings are rejected.

PURCHASE first resolves `discord-scratch-v1:<original interaction ID>`. An
existing exact-input round wins before expiry checks; changed wager conflicts.
Only an absent, nonexpired play may call `Create`, with exactly
`{Type:"SCRATCH", Wager:<canonical>}`. RESUME reads only its signed round ID for
the resolved user and never calls Create. A stale PURCHASE preview never switches
to another website ticket; it returns `SCRATCH_PREVIOUS_REVEAL_INCOMPLETE` (409).

**Settlement and presentation completion are separate transactions.** A purchase
may durably settle once while its subsequent `RevealComplete` fails. Such a
response is uncertain, not proof of no debit. Explicit original-quote recovery
finds the saved ticket and completes only presentation, including after restart.

**Scratch lookup is not entirely read-only:** it may finish presentation on the
original ticket. It never calls Create, buys, charges, re-prepares or follows a
newer ticket. The Bot offers 恢复原券 only after the owner's confirmation attempt.

| Original quote/ticket | At or after expiry |
| --- | --- |
| PURCHASE, absent ticket | Play: QUOTE_EXPIRED; lookup: NOT_FOUND; no purchase |
| PURCHASE, saved incomplete ticket | Original presentation may complete |
| RESUME, still incomplete ticket | Play and lookup: QUOTE_EXPIRED; no reveal |
| Either action, completed original ticket | Read-only receipt recovery |

Completion uses a deterministic per-round HMAC under the independent
`bot-games.scratch.reveal.v1\0` domain. It retains the round's six UUID timestamp
bytes and derives the rest, with UUIDv7 and RFC variant bits. Concurrent requests
and restarts under the same key use the same action ID. Already completed tickets
skip completion. `games.Service.RevealComplete` also rechecks completion under the
existing user/scratch lock after checking cross-round action-ID conflicts. This
prevents the website and Bot from appending separate reveal actions for one
ticket when both observed it incomplete. No wallet or ledger mutation occurs in
this presentation transaction.

## Completed receipt

Exact result keys:

```text
action round_id created_at settled_at presentation_completed_at status wager
cells prize_tier multiplier result stake_units gross_payout_units withheld_units
credited_payout_units actual_net_units ruleset
```

Only confirmed complete SETTLED tickets succeed. UTC RFC3339Nano timestamps obey
created ≤ settled ≤ presentation_completed. Cells contain exactly nine ordered
row-major objects with integer `index` 1..9, `symbol`, and boolean `matching`.
Allowed symbols are P1/P2/P3/P5/P10/P25/P100. LOSS includes every symbol once,
plus two DIFFERENT duplicated symbols and no matches. Winning tickets have
exactly three winning symbols and one each of all six other symbols; only the
three winning cells match. Noncanonical but superficially coherent grids fail.

Tier multipliers are LOSS:0, BREAK_EVEN:1, T2:2, T3:3, T5:5, T10:10, T25:25,
TOP:100. Gross is stake × multiplier. Raw LOSS/BREAK_EVEN/WIN compares gross to
stake; credited is gross minus withheld, and actual net is credited minus stake.
The existing CLIP_PROFIT settlement semantics protect principal and validate the
historical policy and balance delta. A raw WIN can legitimately have net zero.
Receipts include no quote, seed secret, Native ID or available-balance snapshot.

## Verification and rollback

Unit and HTTP tests cover exact action projections, all eight canonical tiers,
malformed grids/configuration/accounting, signature isolation, expiry, binding,
original-only recovery, private headers and absence from the public portal.

`TestBotScratchPostgres` needs its own **new empty** loopback PostgreSQL database
whose name starts `momiao_test_g1_original_scratch_`, supplied with the existing
`MOMIAO_GAMES_TEST_CONNECTION_FILE` fixture contract. Do not run another Native
fixture suite against the same database. Default-suite PG skips are not database
evidence. The scratch suite exercises the real engine/HTTP/ledger and covers:
10/11 Chip, 20 concurrent confirmations, one reveal action, cross-channel reveal
race, cross-round action conflicts, website old-ticket RESUME with different
requested wager, zero extra monetary effects, all expiry branches, a real DB
failure between settlement and completion, restart recovery, subsequent purchase,
original-only lookup, nondefault odds, caps and pre-RNG overflow.

The corruption test uses only its disposable fixture's owner to inject malformed
rows, restoring the immutable guard inside the same transaction before runtime
reads. No production grants or triggers change. Deployment remains website then
Bot; rollback replaces program files only and retains settled rounds, wallet,
ledger, commitments and reveal history. It must not reverse financial history.

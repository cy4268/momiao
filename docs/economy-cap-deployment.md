# Local economic cap and one-time campaign

The migration does not start a new campaign or replay the completed one.
Keep production containers and databases unchanged until the complete restricted
dev acceptance described in the NewAPI removal plan passes.

## Local assets and Poker

Platform and Poker read the same six local asset buckets: Reserve, available
chips, Poker stacks/pots, solo-game funds in flight and roulette escrow.
The Native quota observer and `MOMIAO_ECONOMY_READ_SOCKET` are gone. Existing
per-account locks, cap receipts and wallet writers remain in use.

Apply the existing migrations in order, preserving their original checksums.
Keep the existing 0041/0043/0044 runtime grants. Migration 0065 changes only
the campaign's accepted actor foreign key to `identity.users`; it adds no grant
and does not alter any existing campaign, badge, snapshot or wallet value.
History reader/writer roles remain function-only.

The policy version is persisted before RNG. Old unbound rounds remain legacy.
Poker retains original cards, events, pot awards and neutral returns; cap
receipts record withheld profit without rewriting the original game result.
Migration 0047 keeps legacy receipts readable while new receipts separately
record `neutral_return_units`. Public receipts omit whole-account asset totals.

## One-time campaign operations

The fixed campaign is `01995000-2026-7000-8000-000000000927`.
The panel is at `/ops/gambler-campaign`, linked from Economy. It uses local
Redis/cookie authentication and roles >= 10. POST writes require CSRF, a live
role check and one transactional audit record. No preview, fresh-auth flow,
client version, epoch or generic operation record is required.

Before starting or resuming financial work:

1. Back up the platform database.
2. Open maintenance for CHALDEA_USER_WRITES, WALLET_EXCHANGE, REWARDS,
   DIRECT_PLAY_NEW_ROUNDS, POKER_NEW_TABLES_NEW_HANDS and RANKINGS_PUBLISHING.
   The server selects the current window; no UUID needs to be pasted.
3. Wait for already accepted model requests to finish settling. Maintenance
   rejects new relay requests, but it does not cancel the accepted ones.
   The action confirmation records `requests_drained: true`; this is an
   operator attestation, **not an automated active-request measurement**.
4. The server also checks the existing local drain function for unsettled
   games, table funds, registration grants and historical transfer work.
   Resolve outstanding work rather than skipping or replacing its receipts.

Actions at the same GET/POST campaign endpoint:

- `snapshot`: freeze the qualification cutoff and account set. The worker
  seals the snapshot after its final bounded verification pass. Eligibility
  is strictly greater than 1 billion credits.
- `grant`: award permanent badges from the frozen qualification snapshot.
  This does not reset balances. Repeating a successful grant does not duplicate
  badges; the successful save is audited.
- `prepare-reset`: freeze a separate local reset checkpoint.
- `reset`: separately confirm that target Reserve becomes 10,000 credits and
  available chips become zero. The original maintenance window must still be
  active. Each account uses the original deterministic ledger business keys;
  already completed accounts are never replayed, and zero deltas create no leg.
- `resume`: accept the current local administrator and maintenance window,
  clear the verification cursor and recheck the original frozen checkpoint.
  Changed balances block continuation; this does not recapture or overwrite
  historical values. Sealed qualification remains historical even if current
  assets later differ. A completed, already active campaign has no UI actions.
- `activate`: after every target finishes, verify the reset and activate the
  economic policy. A repeated activation preserves the original timestamp.
  End maintenance after checking completion. The existing rankings worker
  recognizes an already newer public pointer or publishes one fresh snapshot
  once rankings maintenance ends, then ordinary publication continues.

The worker rechecks the accepted actor's local role/status on every batch.
Revocation or a missing maintenance window stops progression. Durable cursors,
membership hashes and immutable account records preserve restart behavior.
New commands leave the historical operation/epoch fields null. Historical
Native totals remain in old qualification JSON; local verification compares
the six local buckets rather than silently rewriting those old totals. The
old `native_unchanged` column is retained as history, not a live measurement.

## Verification and rollback

The current minimal dev regression uses the imported completed campaign inside
one rolled-back transaction. It checks local role handling, direct auditing,
maintenance/drain confirmation, worker revocation and changed checkpoints.
HTTP checks reject writes; UI actions use mocked responses. They do not run a
real grant/reset or constitute full in-flight production acceptance.

Source rollback is exercised on another copy. Do not restore an old database
over accepted ledger entries. Retain compatible history and handle committed
financial changes through audited corrections. The additive 0065 schema may
remain when rolling back this source batch.

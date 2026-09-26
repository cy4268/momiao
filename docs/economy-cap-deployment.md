# Economic cap candidate: inactive by default

Development of this feature does not authorize deployment, qualification capture,
badge distribution, policy activation or asset reset. Keep the policy singleton
inactive until the separately reviewed campaign is complete.

## Independent Poker process

Both platform and Poker must configure the same private
`MOMIAO_ECONOMY_READ_SOCKET` (for example `/run/momiao/economy-read.sock`).
It must differ from public, Native, refill and Poker request sockets. Platform
creates the listener, Poker connects to it. Preserve the existing private Unix
directory ownership and socket permissions; never route this listener through
the public web proxy.

The listener accepts only signed POST `read` and `query` beneath
`/internal/v1/economy/quota/`. It reuses Service Assertion V1 with fixed
**poker → platform** direction, the existing independent process signers, and
peer public-key files. It never accepts browser credentials or executes Native
writes. Calls have a two-second budget and 16 KiB response limit. Platform does
not acquire account SQL locks here: the Poker caller already owns that lock.
The embedded Poker mode instead uses the same in-process read-only observer.

Apply additive migrations 0041–0045 with the normal non-login schema owner.
Apply `runtime-grants-0041-economy-cap.psql` to platform (`platform_writer=true`)
and Poker (`platform_writer=false`) runtime roles. Apply the SELECT-only
`runtime-grants-0043-cap-history.psql` to each dedicated history reader that
projects direct-game or Poker cap receipts. Preserve previous migration checksums,
key-family separation and all original Native per-operation bounds.

The service persists the policy version before RNG. Old unbound rounds remain
legacy even after activation. Poker retains original cards, events, pot awards
and returns, records the profit withheld in immutable cap receipts, and carries
only actual credited stacks into the next hand or cashout.

## One-time campaign (separate production approvals)

The fixed campaign is `01995000-2026-7000-8000-000000000927`. Apply
`runtime-grants-0044-gambler-campaign.psql` after 0045 to the platform role only.
Its readiness function exposes aggregate counts, not raw Poker or Native rows.
The campaign panel is under Operations → Economy; only SUPER_ADMIN with
`economy.adjust` can execute its typed, fresh-authenticated operations. Merely
opening the page, deploying migrations, or starting the worker has no effect.

1. Back up the current platform database and record its recovery reference.
   Pause **new Native model consumption at the actual API ingress**, drain all
   already accepted requests and continue serving the private read-only quota
   endpoint. Platform maintenance alone does not stop Native consumption.
2. Establish one ACTIVE maintenance window covering CHALDEA_USER_WRITES,
   WALLET_EXCHANGE, REWARDS, DIRECT_PLAY_NEW_ROUNDS, POKER_NEW_TABLES_NEW_HANDS
   and RANKINGS_PUBLISHING. Accepted recovery/refunds/cashouts remain enabled.
   Wait for pending transfers, accepted registration grants, unresolved games
   and all table funds to drain. Existing unknown receipts must be reconciled,
   never silently omitted. The initial preview reports any remaining blockers.
3. Separately approve qualification preparation and then medal grants. The
   worker freezes a cutoff and account-set digest, observes every target, and
   seals only after a final observation check. The >1 billion comparison is
   strict. Permanent grants retain this snapshot even after balances change.
4. Separately approve reset preparation, review its private per-account before
   values, and approve RESET_START. The frozen set includes zero-balance and
   administrator accounts. Each durable batch uses the original authorization,
   epoch and operation ID; process restarts continue the same campaign. Reset
   writes only Reserve/chips difference entries with stable business keys;
   already-correct balances receive a no-change receipt and no zero ledger leg.
5. Any changed/unavailable Native evidence stops progression. A detected Native
   change remains an explicit blocking fact; investigate and reconcile before
   any manual recovery. Do not falsely mark it unchanged or start a new campaign.
6. After every account is completed, separately approve ECONOMY_POLICY_ACTIVATE.
   Native and local before/after values are rechecked. End maintenance and
   restore ingress only after verifying completion. The existing rankings gate
   stays authoritative: once publishing reopens, the worker publishes one fresh
   asset snapshot (or recognizes an already newer pointer after a lost response).
   Normal hourly publication then continues. Reset ledger entries are not bets
   or game profits. No production step in this document is executed by tests.

Source rollback must not restore an old database over accepted ledger entries.
Keep the compatible backend for new receipts; use audited compensation for any
already committed financial effects. The offline rollback package is tested
only on separate source/database copies, not the live service.

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

Apply additive migrations 0041–0043 with the normal non-login schema owner.
Apply `runtime-grants-0041-economy-cap.psql` to platform (`platform_writer=true`)
and Poker (`platform_writer=false`) runtime roles. Apply the SELECT-only
`runtime-grants-0043-cap-history.psql` to each dedicated history reader that
projects direct-game or Poker cap receipts. Preserve previous migration checksums,
key-family separation and all original Native per-operation bounds.

The service persists the policy version before RNG. Old unbound rounds remain
legacy even after activation. Poker retains original cards, events, pot awards
and returns, records the profit withheld in immutable cap receipts, and carries
only actual credited stacks into the next hand or cashout.

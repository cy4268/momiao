# Blackjack v1 pure engine integration

Authority: the project implementation specification, IS-06 §§248–249, 265, 277–287; Technical Design §§341–365; baseline §10 Direct Play wager policy. No database, wallet mutation, production environment or UI is part of this module.

## Entry points

```go
Shuffle(sample func(n uint32) (uint32, error)) ([312]uint16, error)
ShoeHash(shoe [312]uint16) (string, error)
Evaluate(cards []Card) (Value, error)
New(shoe [312]uint16, wagerUnits int64, initialHandID string, now time.Time) (State, error)
Apply(state State, shoe [312]uint16, action Action, availableUnits int64, now time.Time) (Transition, error)
AutoResolve(state State, shoe [312]uint16, now time.Time,
    systemActionID func(handID string) (string, error)) (Transition, error)
VerifyRecovery(state State, rebuiltShoe [312]uint16) error
LegalActions(state State, availableUnits int64) []ActionType
PublicView(state State, availableUnits int64) Projection
```

- Bind **one** shared fairness stream to `blackjack:shuffle`. `Shuffle` uses canonical `[0..311]`, Fisher–Yates from 311 down to 1, and requests bounds 312 … 2. Adapt shared `UniformInt(reader,uint64(n))`; no local randomness or additional seed/stream protocol is implemented.
- Shoe hash validates exactly one of each card instance, then SHA-256 hashes 624 bytes of U16BE instances. Cards carry consumed zero-based `ShoeIndex` and `InstanceID`; `InstanceID % 52` is the public canonical card code.
- Creation version is 1. Initial deal is player/upcard/player/hole. New receives the DB creation/first-player-turn instant, not the browser clock. The layer must allocate and persist `initialHandID` as a UUID before/with creation. Nonterminal state has an initial 24-hour anchor.
- `Action` contains `ID`, `ExpectedVersion`, `HandID`, `Type`, `NewHandID`. Supply transaction-owned UUIDs. `NewHandID` is required/fresh for SPLIT and otherwise unused. UUID syntax belongs to the request/transaction layer, while the engine checks nonempty IDs, active hand, uniqueness and version. Actions are HIT/STAND/DOUBLE/SPLIT only; player input cannot invoke SYSTEM_AUTO_STAND.
- Split retains the existing left hand ID/index; the right child has the supplied new ID and `ParentID` of the split hand. `Hands` is left-to-right. `Hand.Index` is a stable **sparse** ordering key in `[0,8)`, initially 0; midpoint insertion preserves all preexisting indexes through a maximum of three splits. Do not reassign indexes on re-split or require contiguous indexes; derive display ordinals from ordered `Hands` separately. The source requires stable ordered indexes, not a contiguous-index wire encoding.
- `availableUnits` means the current locked wallet balance after previous debits. The returned `AdditionalStakeUnits` is the one debit to apply for this transition; New's initial wager is separate. The settled state's `TotalPayoutUnits` is the one total credit, including returned principal. All amounts are integer atomic units; 500,000 units=1 Chip. Initial wager is whole Chip >=10. The numeric safety bound `MaxInt64/16` reserves all legal future four-hand doubled payouts, not a product policy ceiling.
- Apply/AutoResolve verify the loaded authority first and operate on a deep copy. On error discard the zero transition; original state is unchanged. Illegal/stale/duplicate actions never consume source-state cards. `ErrDuplicateAction` is an engine guard, **not** the original stored response: the transaction layer must first look up `action_id` and return its durable original result without another debit/transition.
- On expiry, AutoResolve calls the supplied ID factory once per unfinished hand, in left-to-right order, recording a SYSTEM_AUTO_STAND per hand. These calls allocate IDs only, not independent commits. A factory failure/duplicate ID discards all intermediate changes. A pre-expiry or terminal no-op never calls the factory, which may be nil then. Persist the complete returned state/actions/settlement atomically under durable job dedupe key `blackjack:auto-resolve:{round_id}`. Recovery replays recorded IDs and never calls the factory. System records do not extend the manual anchor.

## Typed persistence / recovery (not an in-memory-only round)

All `State`, `Hand`, `Card` and `ActionRecord` fields are exported for explicit typed column mapping. Persist/load the following within the shared transaction layer:

| Durable source | Engine fields |
| --- | --- |
| Common round/create parameters | `InitialHandID`, `CreatedAt`, `InitialWagerUnits`, `Version`; external round UUID, seed/nonce/config metadata remain in the common envelope |
| `blackjack_round_state` | `ShoeHash`, `ShoeIndex`, `Phase`, `DealerRevealed`, `ActiveHandID`, `LastPlayerActionAt`, `AutoResolveAt`, `TotalStakeUnits`, `TotalPayoutUnits`, `NetChangeUnits`, `Class` (derive redundant fields deterministically if the schema stores them elsewhere) |
| `blackjack_hands`, ordered by stable index | `ID`, `Index`, `ParentID`, `StakeUnits`, `FromSplit`, `SplitAces`, `Natural`, `Status`, `Value` hard/best/soft, `Result`, `PayoutUnits`, `NetChangeUnits` |
| `blackjack_dealt_cards`, ordered by shoe index | `Dealt`; group current PLAYER_HAND recipients into each `Hands[i].Cards`, DEALER recipients into `Dealer`, preserving recipient sequence. A split moves the second original card to the right child without changing its shoe index/instance; persist its ownership along with the two newly drawn cards. |
| Durable round actions, ordered by sequence | `Actions[].Action` (all five fields), `Sequence`, `At`, `AdditionalStakeUnits`. Creation is not an action record. |

Load these fields, independently rebuild the shoe from the protected seed and frozen fairness input, then call **`VerifyRecovery(loadedState, rebuiltShoe)` before exposing actions**. It verifies the shoe hash, recreates New, replays every action and compares the complete resulting state: consumed cards/recipients/index, active/order/hand flags, all amounts/results, versions, action metadata and inactivity timestamps. UTC-equivalent locations and empty-vs-nil slice representation are normalized, but changed values/order are rejected. `Apply` and `AutoResolve` also invoke this gate. Any mismatch returns `ErrNeedsReview`; the transaction layer must mark the round NEEDS_REVIEW and stop dealing (not refund/reshuffle). Timestamps must retain the DB clock's actual stored precision; pass that same precision at creation/actions.

This uses typed state restoration, not an opaque JSONB-only result authority. `State.MarshalJSON` deliberately rejects accidental transport; ordinary formatted State/Transition logs redact authority. Do not log individual private fields either. `PublicView` is a detached projection with only visible card codes, no future shoe, hole value, shoe hash/index, private history or seed. The dealer total is absent before reveal. Monetary fields and round version serialize as decimal JSON strings. Common envelope/fairness projection and explicit post-settlement seed reveal are shared-layer responsibilities.

## Versions/config/availability boundary

```text
implementation_key = direct.blackjack.v1
ruleset_version = blackjack-rules-v1
algorithm_version = blackjack-map-v1
config_schema_version = blackjack-config-v1
shuffle_algorithm_version = blackjack-fy-v1
```

Only the frozen six-deck American-peek S17, 3:2 natural, double any two/DAS, value-based split/re-split max four, one-card split-aces/no re-split-aces rules are implemented. No insurance/even-money/surrender/side bets/persistent shoe or automated player strategy is added. A doubled bust remains DOUBLED_COMPLETE with result BUST. Split 21 is ordinary, never NATURAL.

The shared layer locks/validates these versions and owns §249 canonical JSON/config hashing (`CHALDEA_GAME_CONFIG_CANONICAL_JSON_V1`, SHA-256 of the `CHALDEA-GAME-CONFIG-V1\x00` domain + three LP16 identity fields + canonical payload). This module does not select/change config, hash a different payload, or manufacture a validation artifact. Production `BLACKJACK_RTP`/house-edge computation and VERIFIED activation artifact, durable HTTP/ledger/jobs/restore integration and real-runtime acceptance remain outstanding outside this change. No claim that Blackjack is formally playable follows from these pure tests.

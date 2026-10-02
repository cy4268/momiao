# Private Discord dice listener

This optional platform-only listener connects the signed `internal/botgames` service to the existing website dice engine. With all four settings absent it opens no listener, Native connection, or bot-game worker. No route is mounted on the public portal; existing `/internal/` denial remains in place.

## Configuration and startup

Configure all four absolute paths, or none:

| Setting | Purpose |
| --- | --- |
| `MOMIAO_BOT_GAMES_SOCKET` | Dedicated Unix socket, created with mode `0600` |
| `MOMIAO_BOT_GAMES_TOKEN_FILE` | Shared service bearer token; 64 lowercase hex characters |
| `MOMIAO_BOT_GAMES_QUOTE_KEY_FILE` | Website-only persistent 32-byte quote signing key; 64 lowercase hex characters |
| `MOMIAO_BOT_GAMES_NATIVE_DSN_FILE` | Dedicated private pgx-compatible Native read-only connection file |

The platform role, existing wallet DSN, game fairness keyring, and access-gate deployment declaration are prerequisites. Empty/partial settings, relative paths, or collisions with configured sockets/authority files are rejected. Token/quote files must be private regular files (no symlinks); an optional single LF/CRLF is accepted. All-zero keys and reused token/quote/fairness key material are rejected. Keep the quote key persistent across restarts; replacing it invalidates previously issued quotes.

Use a dedicated socket directory. The Bot needs only that directory and the service token; it must not receive the website runtime directory, Native/wallet DSNs, quote key, or fairness keyring. Keep token, quote key, and fairness keys independently generated. Startup never replaces an existing filesystem entry. Normal shutdown stops HTTP before closing the Native pool and removes only the listener's own socket.

The Native pool has at most four connections, default read-only mode, a two-second connect timeout, and explicit read-only resolution transactions. It needs only `SELECT(id,discord_id,status,deleted_at)` on `public.users`. The query uses `discord_id=$1 LIMIT 2`, including disabled/deleted matches when checking uniqueness. No Native write, account initialization, profile initialization, or migration acknowledgement is performed by the resolver. It then checks `AccountRefExists`, profile `COMPLETE`, notice `NOT_REQUIRED`/`ACKNOWLEDGED`, and declared `EXPERIENCE=AVAILABLE`, on **every** prepare/play/lookup request. Unknown dependencies fail closed.

A libpq DSN containing `hostaddr` must not be copied unchanged into pgx: use a separately verified B connection file with compatible options and certificate paths. Keep the existing read-only bridge, DSN, ACLs and other services unchanged.

## HTTP contract

Exactly three POST paths:

- `/internal/v1/bot-games/dice/prepare`
- `/internal/v1/bot-games/dice/play`
- `/internal/v1/bot-games/dice/lookup`

Send exactly one `Authorization: Bearer TOKEN` and one `X-Discord-User: SUBJECT`, plus exactly `Content-Type: application/json` (no parameters). `SUBJECT` and `request_id` are canonical decimal strings in `1..2^64-1`.

```json
{"request_id":"970000000000000001","wager":"10","choice":"BIG"}
```

Play and lookup accept only the returned quote:

```json
{"quote":"SIGNED_QUOTE"}
```

Bodies are limited to 4096 bytes, including whitespace, also for streamed requests. Duplicate/unknown fields, missing fields, nonstring fields, trailing JSON, query strings, encoded/confused paths and additional body credentials are rejected. The existing non-bot decoder retains its 2048-byte limit. Cookie, Proxy-Authorization, New-Api-User, X-Auth-Session, X-Native-User, X-User-ID, X-Forwarded-User, X-Authenticated-User, Idempotency-Key and X-Fairness-Commitment headers are rejected even if empty. External idempotency keys, Native IDs and client-supplied commitments are never accepted.

The handler has an eight-second context deadline, at most 16 in-flight requests, and an immediate 503 on overload. Header/read/write/idle timeouts are 1/2/10/10 seconds; MaxHeaderBytes is 4096. Responses are JSON with `Cache-Control: no-store`. Logs/errors expose stable types only, never request bodies, quotes, identities, tokens, SQL, DSNs or upstream error text.

Prepare returns `quote,wager,choice,available_units,minimum_wager_units,maximum_wager_units,ruleset,rules_text,server_seed_hash,commitment_id,expires_at`. All units and expiry Unix seconds are strings. Only the private Prepared projection includes balance.

Play/lookup return `round_id,created_at,settled_at,status,wager,choice,dice,total,result,stake_units,gross_payout_units,withheld_units,credited_payout_units,actual_net_units,ruleset`. Times are UTC RFC3339Nano strings. Raw WIN/LOSS/BREAK_EVEN and actual net are separate: a capped raw win can have zero actual net. Neither result includes Native identity, wallet snapshots, transaction identifiers or unrevealed seeds.

Quotes bind the request, subject/current account fingerprint, wager, choice and commitment for 120 seconds. Use `discord-dice-v1:<original interaction ID>` internally. Every new round still requires the member's explicit confirmation. Cancellation/expiry makes no new round. The engine owns randomness, immutable result authority, wallet/ledger writes and durable idempotency. An existing same-parameter result is returned before applying expiry; changing the same request's wager/choice conflicts. A different current binding rejects the old quote. A commitment consumed in the browser cannot create a second round.

| HTTP | `{"error":"CODE"}` |
| --- | --- |
| 400 | INVALID_REQUEST (including invalid method/body/path encoding) |
| 401 | UNAUTHORIZED |
| 403 | BINDING_CHANGED, ACCOUNT_RESTRICTED |
| 404 | NOT_LINKED, NOT_FOUND |
| 409 | ACCOUNT_NOT_READY, QUOTE_EXPIRED, COMMITMENT_INVALID, IDEMPOTENCY_CONFLICT, INSUFFICIENT_CHIPS, MAINTENANCE |
| 503 | UPSTREAM_UNAVAILABLE (including overload and unknown/internal errors) |

An unavailable response after play is an uncertain outcome, not proof that no round settled. Recover with lookup and the original quote; never manufacture a new interaction ID automatically.

## Verification, deployment order and rollback

1. Run the default suite and separate Linux socket/real-PG tests. The real-PG suite uses a newly prepared isolated G1 fixture, not a production connection and not the old fixed-migration history fixture.
2. Confirm existing platform runtime grants for the engine, notably the maintenance guard functions and active-config row locks, without weakening production privileges. This feature adds no production tables or automatic migrations.
3. Provision independent private files and dedicated socket mounts, then deploy the platform with the optional configuration. Verify process readiness, mode `0600`, public-route rejection, Native read-only access and failure behavior before enabling the Bot command.
4. Deploy the Bot client only after the private listener contract is verified. Recreate namespace-sharing sidecars alongside a replaced platform where applicable. Do not run unattended real-member wagers; any final real-member round requires that person's confirmation.
5. Roll back program/configuration and disable the four optional settings (and the Bot feature). Preserve existing settled records, ledger, wallet state and durable keys. Never restore old balances or delete settled rounds as rollback.

### Disposable test harness reuse

`cmd/momiao/bot_games_postgres_test.go` supplies `newBotGamesPGFixture`, `player`, `prepare`, `effects`, `balance` and `botGamesPGCheck`. A later cross-language harness can use `newBotGamesHandler(f.service, botGamesTestToken)` or `openBotGamesApplication` in a disposable test source copy; no production fixture endpoint is needed. The fixture starts with `gameBrowserStores`, applies all current migrations, and uses the least-privilege runtime plus synthetic Native rows. It adds the existing engine's narrow `EXECUTE` on `ops.lock_write_scopes(text[])`/`ops.is_maintenance_scope_active(text)` and `UPDATE(status)` on `games.game_config_versions`, because the reused browser fixture predates those requirements. Owner-only setup installs synthetic Native quota support and toggles test maintenance/cap policy. Tests assert the Native role's default read-only behavior **and** UPDATE denial in an explicitly read-write transaction.

Opt in through `MOMIAO_GAMES_TEST_CONNECTION_FILE`, whose Native URL must refer to loopback port 55432 and a `momiao_test_g1_original_` database. The private fixture JSON includes OwnerURL/RuntimeURL/RuntimeRole/NativeURL/NativeRole. Never print it. Prepare a fresh fixture before each run; do not run these write tests concurrently against one fixture. Without opt-in, the PostgreSQL test skips explicitly; that skip is not PostgreSQL evidence. Real socket permission acceptance runs on Linux.

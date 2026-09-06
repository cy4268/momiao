# M2b platform admission implementation plan

> Execution: inline in this authorized isolated task using executing-plans and TDD. The controller explicitly prohibited additional agents/tasks and button-level approval stops.

**Goal:** Complete the local portal admission/account UI and durable provisional Master/registration reward recovery candidate.

**Architecture:** Native M2a owns identity, passwords and sessions. The platform consumes committed registration receipts through its fixed private transport, persists a cursor/inbox/claim/job transaction, and applies the one-time Reserve grant through the existing economy transaction. React extends the existing ApiClient epochs and routes.

**Tech stack:** Existing Go 1.27.1, pgx/PostgreSQL, React 19, React Router, TypeScript, Vite/Vitest and existing CSS tokens; standard library first.

**Spec:** `docs/m2b-admission-contract.md` and imported `native-adapter/CONTRACT.md`.

## Global constraints

- Work only in the new M2b clone; M2a and original/shared sources remain frozen/read-only.
- Grant: 500000000 units, RESERVE_API_CREDIT, initial_grant:registration:{native_user_id}, INITIAL_GRANT_REGISTRATION per-user uniqueness.
- No 0006 until accepted 0005 is supplied; never modify 0001–0004 or the contiguous/checksum checks.
- No real credentials in source/logs, no production/account action, no push/deploy and no new agents.

## Task 1: Freeze adapters and browser authority

Files: new `cmd/momiao/admission_source.go`, `admission_source_test.go`, `admission_config.go`, `admission_config_test.go`; extend `web/src/api.ts`; new `web/src/admission-api.ts`, `admission-session.test.ts`.

- [x] Add failing fixed-endpoint reader tests: private bearer only; no browser headers/cookies; bounded responses; malformed/duplicate/unsorted pages rejected; 429/503/transport failures retain safe classification.
- [x] Implement `readRegistrationPage(ctx, transport, readerKey, after, limit)` consuming the exact M2a receipt envelope. It returns validated receipt data and native next cursor, and never mutates a database.
- [x] Add failing ApiClient tests for callback/logout/account-switch races, single-flight 2FA, proof-only sensitive results, native password bundle rotation and no storage writes.
- [x] Implement fixed-path Discord start/callback/2FA/password helpers in the existing client; reject untrusted authorization URLs before browser navigation.
- [x] Run targeted Go/Vitest checks and retain red/green logs.

## Task 2: Public and account UI

Files: `web/src/App.tsx`, new `Authentication.tsx`, `Account.tsx`, `PostAuthGate.tsx`, `admission.css` and corresponding tests; modify `main.tsx`, `PersonalHub.tsx` and native browser-route allowlist as needed.

- [x] Build failing page tests around existing-user login versus new registration, denial/membership/role/provider/unknown states, password show/hide and clearing, three password flows, and native 2FA.
- [x] Implement public authentication with shared design tokens and a separate account/security page; add no new authentication storage.
- [x] Implement immediate callback query cleanup and in-memory challenge/proof handling; cleanup is tested before network completion.
- [x] Implement fixed non-sensitive route intents and Master-first post-auth gate, preserving existing wallet/quota receipt storage and recovery handlers.
- [x] Run affected frontend tests, typecheck/build and inspect real DOM before broader integration.

## Task 3: Accepted 0005 dependency and provisional identity

Files after accepted stack/import: new `internal/platform/migrations/0006_admission.sql`, `admission.go`, `admission_test.go`; modify `profile_store.go`, `profile_test.go`; new `cmd/momiao/admission.go`, `admission_test.go`.

- [x] Wait for the controller's exact accepted 0005 commit, inspect its diff and import without altering migration sequencing.
- [x] Add real PostgreSQL failures for durable v0, same-user concurrent ensure, v0-to-v1 same-row upgrade/history, nickname competition and unchanged existing COMPLETE profiles.
- [x] Add only 0006 schema changes and implement idempotent account/provisional ensure plus original-row initialization.
- [x] Add authenticated explicit ensure and read-only admission status handlers, empty strict request body and original/native identity verification.
- [x] Confirm no GET writes and no initialization grant before source receipt ingestion.

## Task 4: Receipt consumption and one-time grant recovery

Files: `internal/platform/admission.go`, `admission_test.go`, `0006_admission.sql`; `cmd/momiao/admission_source.go`, `admission_worker.go`, worker tests; limited startup/config/router hooks and transaction-kind display.

- [x] Add real PostgreSQL red tests for atomic page cursor/inbox/source/claim/job ingestion; repeat/unsorted/conflicting receipt handling; source outage and failed local transaction.
- [x] Implement ingestion under a locked cursor; classify only trusted immutable new-registration receipts and preserve complete profiles.
- [x] Add red tests for crash after ingestion, unknown grant commit replay, concurrent workers and injected transaction failure with zero partial ledger/supply/claim effects.
- [x] Implement durable recovery using `applyInTx` and immutable registration supply issuance evidence, then mark claim CONFIRMED in the same local transaction.
- [x] Include the registration transaction in actual private history/status UI without altering daily, exchange or quota semantics.

## Task 5: End-to-end local acceptance and clean handoff

- [x] Provision only M2b local databases and start a real isolated portal plus stateful synthetic native service; no production configuration values.
- [x] Exercise registration → callback → 2FA → Master → actual grant/ledger, all password flows and logout/late responses at 1440 and 390. Capture safe screenshots and interaction evidence.
- [x] Run affected Go tests/vet, frontend tests/typecheck/build, migration upgrade checks and replay/restart recovery; record any conditional skips honestly.
- [x] Review exact diff, protected source boundaries and migration 0006 delta. Commit with noreply identity, verify clean worktree, and deliver `evidence/M2b-platform-review.md` plus contracts and source references to the controller.
- [x] Freeze after the local candidate handoff; do not merge/push/deploy or claim full initial release.

Final boundary: actual browser validation used a stateful synthetic native/provider service. Real Discord, the combined post-auth Gate and dedicated Saber/Mash art remain explicit integration gaps in docs/M2b-platform-review.md. The controller requested focused final verification without repeating unchanged suites.

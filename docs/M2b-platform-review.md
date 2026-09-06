# M2b local candidate

Implemented: native Discord callback/2FA/password pages, durable Master v0 → v1, private receipt ingestion, and exactly-once 1000 Reserve with atomic ledger/issuance/claim confirmation. No push, deployment or formal account operation.

## Integration

- Base: `d1c0145c182a2e7989962b5be0147333ed8f79e5`.
- Frozen M2a reference imported as `a7fa6fd`; approved 0005 imported as `8e928ea` from `e988524ef0894f84c0c951682c7d9f35c9b4b660`.
- Approved 0006 commit: `eea192e7bc7417a8325d4e59d4281106164d423f`.
- 0006 SHA256: `6466e21dfc6332dffd785016f704269d95d687a13677468076f0ef82f8e8a4e9`.
- The final application commit follows 0006. If these exact dependencies are already integrated, apply only that application commit. Native-adapter, 0001–0005 and the migration runner/checksum rules are unchanged.

Scope: admission store/worker/config/router, original profile initialization and transaction history, React authentication/account/post-auth pages and tests. Existing ledger negative controls now remove registration FK dependents only inside their always-rolled-back test transaction; production guards are unchanged.

## Evidence

Raw root: `E:\Programs\vps\.codex-tmp\m2b-platform-20260906\evidence`.

| Check | Observed result | Raw file |
|---|---|---|
| Full Go + fresh PostgreSQL | 268 pass, 0 fail, 4 conditional skips | `final-go-tests-fresh.jsonl` |
| Go vet | passed | `final-go-vet-fresh.log` |
| Full frontend | 169 passed / 22 files | `final-frontend-tests.log` |
| Final affected Account/App regression | 4 passed, including new proof handoff test | `sensitive-callback-handoff-green-v2.log` |
| Final TypeScript + Vite build | exit 0 | `sensitive-callback-build-v2.log` |

Full Go includes concurrent workers, lost actual COMMIT responses, killed test connection, zero partial issuance, and V5 profile/history/cooldown preservation. Skips: opt-in browser harness, Windows symlink setup and two Unix-only checks. The harness ran separately twice: both raw `browser-harness*.log` files end in PASS. Earlier failed logs are retained: old negative controls conflicted with new FKs; a repeated run also reused fixed-ID fixtures. The passing full suite used fresh `momiao_test_m2b_20260906_02`.

Actual portal/React/PostgreSQL with synthetic native HTTP: registration → Master v0 → saved nickname → pending grant → wallet 1000 / one transaction / one +1000 ledger; first password set, current-password change, mobile sensitive 2FA/reset, logout and reset-password login with 2FA returning to `/account`. Browser-discovered proof handoff failure was reproduced in a red App test, fixed and verified in the actual browser. Credentials/proofs remain memory-only and leave-page cleanup is tested.

Screenshots: `screenshots/register-desktop.png` (1440×1000), `welcome-provisional-desktop.png` (v0/pending), `wallet-confirmed-desktop.png`, `sensitive-2fa-mobile.png` and `password-reset-mobile.png` (390×844). Captures use the viewport because full-page capture showed compositor artifacts. Browser overrides were reset; test tabs and harnesses closed. `_test.go` fixtures are excluded from production builds.

## Remaining gates

1. Real Discord OAuth/callback/membership/role acceptance is unverified. Automatic approval review rejected real Discord navigation; no alternate external-login mechanism was attempted. Synthetic provider tests do not prove real OAuth. The user performs that gate after deployment readiness.
2. Controller composes the native M2+M3 image, Linux Unix transport/restricted reader file and combined migration/announcement/critical-action Gate. This candidate does not claim those integration gates closed.
3. Royal Observatory/Beacon is the verified existing visual asset; dedicated Saber/Mash character art remains an explicit asset gap.

Revert UI/code independently if required; preserve 0006, claims and immutable issuance/ledger facts after issuance. Pausing the opt-in worker leaves its durable jobs recoverable. Full contract: `docs/m2b-admission-contract.md`.

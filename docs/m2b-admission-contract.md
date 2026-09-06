# M2b portal admission contract

This implements the controller-approved M2b batch on published base `d1c0145c182a2e7989962b5be0147333ed8f79e5`, with reviewed M2a imported as contract reference. The native patch remains frozen. Source authority is Requirements §3, IA §219–228 and §265–272, Art Direction §8 and AD-FRZ-520, Technical Design §34–38, Implementation Spec §145–158 and §217–221 / IS-FRZ-142, 143, 204. The Model Persona prompt archive does not define authentication UI or servant art.

## Native authentication and pages

The portal keeps native `/api/user/login`, refresh/logout and keys/logs/model paths. M2a's fixed `/api/momiao` routes remain the only Discord/password authority. `/login` places existing-user Discord login above password login; `/register` is a separate eligibility/Discord-start page. `/oauth/discord` captures code/state in memory and immediately removes query parameters before the fixed callback XHR. Login, callback, 2FA and password completion use the existing ApiClient epoch and logout flight boundaries; no provider credential, proof, password or browser token enters storage or a URL beyond the required provider redirect/callback protocol.

`/account` shows the actual read-only native login identifier, private short account ID, connection/password/2FA states and current session actions. First set uses a same-binding fresh proof, change requires current password, reset uses same-binding Discord recovery. A returned sensitive proof stays in memory on the callback/account view and is lost on reload; users reauthorize rather than restore it from storage. Native 2FA challenges are completed before a login bundle or proof is accepted. No second credential store, refresh mechanism, username editor or self-rebind exists.

`GET /platform/v1/admission/config` is a non-secret feature/eligibility display contract. `POST /platform/v1/admission/ensure` explicitly ensures only the authenticated account/provisional profile; it accepts an empty object and never a source or grant flag. `GET /platform/v1/admission` reads actual source/grant state. Both authenticated endpoints use the existing fixed private native verification transport. GET remains free of writes. Private `/internal` paths are rejected by public routing even when a reader bearer is supplied.

## Durable identity and grant

Approved migration 0005 was imported unchanged, and migration **0006 only** adds a persisted `INCOMPLETE` profile at version 0. A provisional row reserves no nickname; initialization updates that same user row to version 1 and appends exactly one first name-history entry. Existing complete profiles, versions, names and seven-day rename semantics are preserved. The reviewed native DTO has no provider display name, so suggestions use `Master-<short account ID>`; a native username is never silently saved as the public profile.

The source adapter reads only the reviewed native `/internal/momiao/registrations` endpoint using its own restricted reader credential and existing private transport. It never accepts client registration/source claims. A local transaction locks the ordinal cursor, validates/deduplicates the immutable receipt, ensures the account anchor, classifies the trusted source, inserts a PENDING grant claim and durable recovery job, then advances the cursor. Same or earlier receipt replays are accepted only if immutable fields agree. Invalid ordering/conflicting facts fail closed; no higher cursor hides a failed page.

The recovery worker uses existing `applyInTx` locks and issues exactly **500000000 units = 1000 API Credit** into `RESERVE_API_CREDIT`, with `initial_grant:registration:{native_user_id}` and per-user `INITIAL_GRANT_REGISTRATION` uniqueness. Claim confirmation, transaction, wallet leg, ledger and the narrow 0006 immutable issuance record commit together. The accepted daily-checkin issuance receipt and other economic rules are unchanged. Unknown commit outcomes recover the original claim/job and never delete or re-register an account.

Absence of a consumed source receipt is reported as unverified/pending-source rather than proof of a new registration or an eligible reward. Historical accounts do not receive an initial grant. Source outages do not prevent authentication/provisional Master initialization. PENDING/RECOVERING claims remain visible and recover through the same business identity after restart.

## Post-auth navigation and visual scope

Native authentication/account status precede Master initialization. A complete Master resumes the original permitted destination using a 30-minute, single-use stable whitelisted path under `chaldea.post-auth.route.v2`; the obsolete index format is discarded. It contains no credential, request body, amount or arbitrary URL. Existing per-user wallet/quota receipts recover through their current pages; navigation never replays a mutation. Migration/announcement/critical-action Gate composition remains the controller's combined integration scope.

UI reuses the existing Marcellus display face, Segoe UI/Microsoft YaHei body, ivory `#f4f0e8`, azure `#3568b7`, moon `#95acd0`, ink `#17202d`, muted `#596573` and white paper. Public authentication uses the same Royal Observatory/Beacon family; a calm scene and clear action panel carry login/registration, while account/provisional forms keep their existing compact layout. Real DOM holds every action/status. Character assets are used only when suitable existing provenance is verified; no production-ready character or avatar availability is invented. Desktop 1440 and mobile 390, keyboard focus, labels, meaningful failure/unknown states and reduced motion are mandatory.

## Isolation and acceptance

No main merge/push/deploy, formal account/asset operation, original design/shared-state edit, native patch change or new agent/task. Local databases use dedicated M2b names and never reuse M2/M3 databases. Migration sequence and checksums stay enforced; no 0006 file is introduced until accepted 0005 is supplied.

Acceptance includes source-offline/duplicate/out-of-order/crash/rollback tests, real PostgreSQL profile and exactly-once amount/ledger/supply tests, ApiClient epoch and 2FA/password UI tests, affected Go/vet and frontend test/typecheck/build, and an actual local portal with a synthetic native HTTP service at 1440/390. Real Discord, real users and production activation remain later integration gates. The final report records commands/exits, candidate SHA, migration delta and non-destructive rollback boundaries.

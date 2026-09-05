# Portal native API contract

Current reference: NewAPI `v1.0.0-rc.25`, source revision `f116414284162ad15d8925f7bca494c109b83e93`. This is a compatibility scope, not proof of every upstream feature. Recheck before upgrading the native service.

Browser calls stay same-origin. Protected requests use `Authorization: Bearer …` and `New-Api-User`; the active session is attached where supported. The Go transport does not mint tokens or rewrite native cookies. HTTP 200 alone is not success: JSON `success` must be true.

| Method / path | Used behavior |
|---|---|
| POST `/api/user/login` | Username/password login; success data contains `access_token`, `user`, `session`; required 2FA is a separate flow |
| POST `/api/user/login/2fa` | Complete native `flow_token` + code flow |
| POST `/api/user/auth/refresh` | Restore access from native refresh cookie |
| POST `/api/user/auth/logout` | Revoke native refresh session |
| GET `/api/user/self` | Current user's real account and native quota/usage |
| GET `/api/token/?p=1&page_size=20` | User-owned masked token list, `data.items/total/page/page_size` |
| POST `/api/token/` | Create a named, explicitly bounded or explicitly unlimited key; returns success, not its ID/key |
| POST `/api/token/{id}/key` | Reveal exactly one owned key on explicit action; returned key lacks client-side `sk-` prefix |
| PUT `/api/token/?status_only=true` | `{id,status}` changes only state; 1 enabled, 2 disabled |
| DELETE `/api/token/{id}` | Delete exactly one owned key after confirmation |
| GET `/api/log/self` | Paginated personal logs and supported filters, not admin-wide logs |

Native refresh cookie: Secure, HttpOnly, host-only, SameSite Strict, exact Path `/api/user/auth`. The page does not write access/refresh tokens to localStorage, sessionStorage, URLs or logs. A full key is fetched only for explicit reveal/copy and cleared with its view.

Create/delete are not automatically replayed after an ambiguous response. Refresh is single-flight; logout invalidates in-flight client state so late responses do not restore it. The source of truth remains the backend after any operation.

Quota numbers here are native accounting units, not the planned platform Reserve/API Credit. Neither key creation nor `/api/status` proves a model channel exists or can complete a request.

Controller acceptance uses a short-lived synthetic key with one native quota unit: create → masked list → explicit reveal → disable/enable → delete, preserving the preexisting key set. Authentication/self/logs and refresh/logout are also checked over real HTTPS; private values and operational evidence do not enter this repository. Frontend tests use isolated synthetic responses only in test files.

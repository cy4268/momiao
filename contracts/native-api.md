# Portal native API contract

Current reference: NewAPI `v1.0.0-rc.25`, source revision `f116414284162ad15d8925f7bca494c109b83e93`. This is a compatibility scope, not proof of every upstream feature. Recheck before upgrading the native service.

Browser calls stay same-origin. Protected requests use `Authorization: Bearer …`, `New-Api-User` and `X-Auth-Session`. The Go transport does not mint tokens or rewrite native cookies. For management APIs HTTP 200 alone is not success: JSON `success` must be true. The playground uses a separate OpenAI-style response parser.

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
| GET `/api/user/self/groups` | Actual usable groups: `data[group] = {ratio,desc}`; no hardcoded available group |
| GET `/api/user/models?group=…` | Enabled model IDs for the selected usable group; omitting group returns a union, not necessarily the account group |
| POST `/pg/chat/completions` | Native login session, text-only OpenAI payload with explicit model/group, messages, max_tokens and stream; no API key creation |
| GET `/api/channel/?p=1&page_size=10` | Admin channel metadata page; upstream keys omitted |
| POST `/api/channel/` | Root basic create: `{mode:"single",multi_key_mode:"",batch_add_set_key_prefix_2_name:false,channel:{type:1,…}}`; no ID in response, reload list |
| PUT `/api/channel/` | Narrow intended field patch including id; never send status or a blank replacement key |
| POST `/api/channel/{id}/status` | `{status:1}` enable or `{status:2}` disable; distinct operate permission |

## Model workspace details

Source references: `router/relay-router.go`, `controller/{playground,user,group,channel}.go`, `middleware/distributor.go`, `service/authz/resources_channel.go`, `relay/helper/common.go` at the revision above.

Use the account's actual `group` from self as the initial selection. Both catalog and playground use the same explicitly selected group. An empty catalog is a real state; model names do not establish upstream capability or availability.

The playground accepts the current login bundle, not a legacy dashboard personal access token. It bypasses permanent API-token creation, **not** user quota accounting. Success is OpenAI JSON or SSE `data:` events with `[DONE]`; errors may use an OpenAI `error` object or a native authentication envelope. Handle partial frames, UTF-8 boundaries, content/reasoning/usage events, explicit errors and premature EOF. A missing normal terminator is not successful completion. Never replay a model POST after an ambiguous response or authentication failure.

Ordinary HTTP 403 permission/quota errors must not clear a valid login. Explicit authentication/session errors still invalidate local state. Cancellation on navigation/logout must discard late output; authentication renewal happens before a call, not by replaying it.

Channel type 1 appends the relay path to `base_url`; do not enter the full chat-completions URL as the base. `model_mapping`, `setting` and `header_override` are native JSON **strings**. The basic editor only changes its owned fields, not the advanced settings. List/detail/update omit or blank the upstream key; this portal does not call the sensitive channel-key reveal endpoint. New channels and sensitive edits require root or an explicitly granted native `ChannelSensitiveWrite` permission. This UI exposes basic editing to root and leaves custom delegated permission management to native administration.

Native refresh cookie: Secure, HttpOnly, host-only, SameSite Strict, exact Path `/api/user/auth`. The page does not write access/refresh tokens to localStorage, sessionStorage, URLs or logs. A full key is fetched only for explicit reveal/copy and cleared with its view.

Create/delete are not automatically replayed after an ambiguous response. Refresh is single-flight; logout invalidates in-flight client state so late responses do not restore it. The source of truth remains the backend after any operation.

Quota numbers here are native accounting units, not the planned platform Reserve/API Credit. Neither key creation nor `/api/status` proves a model channel exists or can complete a request.

Controller acceptance uses a short-lived synthetic key with one native quota unit: create → masked list → explicit reveal → disable/enable → delete, preserving the preexisting key set. Authentication/self/logs and refresh/logout are also checked over real HTTPS; private values and operational evidence do not enter this repository. Frontend tests use isolated synthetic responses only in test files.

# M3a announcement contract

Scope: an isolated local candidate based on d1c0145. No production actions, automatic
administrator creation, native role mapping, financial changes, or general Ops framework.
REQ §19 → IA §191–210 → Art Direction §21 → IS §386–397, §417–422, §425–427.
Design migrations 000025/000027 map to local **0005**, preserving 0001–0004 byte for byte.

## Content and delivery

Announcement UUID, immutable integer content version, notification revision, and optimistic
row version are separate. Types: SYSTEM, NEW_MODELS, GAME_EVENTS, MAINTENANCE, IMPORTANT,
ACKNOWLEDGEMENTS. Visibility: PUBLIC or AUTHENTICATED (transport spelling for logged-in viewers).
Canonical ACKNOWLEDGEMENTS is unique, with default PUBLIC, PINNED_LIST and ENTRY_POPUP;
no invented contributors. Versioned structured entries contain only consenting public display
names/notes/groups/HTTPS links, order and anonymity. No media upload is exposed until the
managed-asset validation/consent pipeline exists; media IDs are rejected in this candidate.

Controlled Markdown → goldmark v1.8.5 → bluemonday v1.0.26 custom allowlist,
policy announcement-sanitize-v1. Reject raw HTML and Markdown images. Allow p/br/hr,
h2–h4, strong/em, blockquote, lists, pre/code and HTTPS links without credentials or
scheme-relative URLs; apply noopener noreferrer nofollow. Store source and canonical HTML hashes.
Public responses contain canonical HTML and safe structured entries only.

Every query evaluates database time, visibility and withdrawal. Active delivery requires
PUBLISHED, publish_at <= now, visible_from <= now < visible_until (NULL = unbounded).
Archive is explicit: formerly published, non-withdrawn expired/archived content may be
read as history (IA §194–196); never eligible for active placements. A missed scheduled
window is EXPIRED with MISSED_PUBLISH_WINDOW and was never public, so absent from archive.
List filters: type/search/date_from/date_to/archive, bounded offset pagination. Shanghai
calendar dates become UTC bounds; sort pinned manual order then publish_at DESC and UUID DESC.

PINNED_LIST, ENTRY_POPUP, POST_LOGIN_POPUP, PUBLIC_HOME_BANNER and DASHBOARD_SUMMARY are
independent. ENTRY_POPUP and PUBLIC_HOME_BANNER must be public; the latter uses the
PRIMARY_HOME_BANNER guard. Fixed guard rows are locked in deterministic order, including
future interval checks for scheduled announcements. Half-open windows permit adjacent slots.
Scheduling and durable publish/expiry jobs commit together. A bounded in-process worker
locks jobs/targets, resumes after restart, never pushes a popup or records reads, and skips
obsolete jobs. Missed windows expire without delivery. Runtime reads enforce expiry even
when the worker is stopped.

## HTTP and authority

Existing `/api/*` remains the native proxy. IS's illustrative `/api/v1` adapts to
`/platform/v1/announcements` and `/platform/v1/ops/announcements`.
Public GET list/detail, current-entry-popup, current-home-banner and dashboard-summary use
one projection. Authenticated current-post-login-popup returns unread `candidates[]`, sorted
visible_from ASC/UUID ASC, with no invented product priority. Detail GET is read-only;
POST `/{id}/reads` records precisely the submitted notification_revision, idempotently.
An old delayed read cannot mark a newer revision. Authentication, when supplied, must verify
through existing fixed Unix native transport; invalid credentials never downgrade to guest.
Authenticated announcement entry points require browser-session credentials. A bounded
unverified credential-kind gate checks the fixed native HS256 claim grammar, issuer
`new-api`, audience `new-api-dashboard`, token_use `access`, and matching sid/sub headers.
It grants no identity: the unchanged complete token must still pass fixed Unix
`/api/user/self` signature/expiry/live-session validation. Source: native service/auth_token.go
ParseDashboardAccessToken and middleware/auth.go classifyDashboardCredential; matching
internal credentials cannot fall back to PAT. Opaque/dotted PATs are rejected here.

Every Ops request verifies native identity then the current ACTIVE platform principal,
base role, assigned ANNOUNCEMENTS scope and authz_epoch. Code-owned permissions:
announcements.read/write/publish. SUPER_ADMIN all; AUDITOR read only; OPERATOR requires
ANNOUNCEMENTS. Empty principal set denies everyone. No online role management.
This candidate provides only explicit synthetic test seeds, restricted to local `m3_announcements_*`
databases and excluded from the production binary. A production initial SUPER_ADMIN bootstrap
is a separate verified operations handoff: IS §177/§550 requires a set lock, zero existing
principals, a source-verified target, role history and SYSTEM_BOOTSTRAP audit, permanently
closed after the first principal. No general-purpose bootstrap CLI ships in M3a.

Ops GET list/detail expose the principal epoch and current row version. POST render-preview
validates/sanitizes without publishing. POST execute accepts a stable UUID operation_id,
authz_epoch, target ID/version and action. SAVE creates/edits drafts; UPDATE_CONTENT_ONLY
edits live content while preserving notification revision and placements. PUBLISH, SCHEDULE,
WITHDRAW, ARCHIVE, RE_NOTIFY and live updates require POST prepare first. UPDATE_PLACEMENTS
adjusts active published channels independently, rechecks the same fixed guards and increments
only the row version. Server-generated
impact (target/version/visibility/window/placements/read audience), command hash, principal
epoch and ten-minute expiry are stored; execute requires the exact preview and confirmed=true.
No Level3 fresh-auth dependency is invented for Level2 announcements.
All mutations lock principal/epoch and target version, perform business validation, and insert
an append-only audit/result in one transaction. Duplicate operation with equal semantics
returns the same committed result; different semantics conflict. A rollback leaves neither
business changes nor success audit. Exact-origin JSON writes; bounded strict request bodies.
Public errors contain stable codes and no SQL, raw Markdown or operational fields.

## Web adaptation and acceptance

Pages: `/announcements`, `/announcements/:id`, `/ops/announcements`.
Existing ivory/azure typography, list rows, quiet article body; compact functional Ops workspace.
Home banner is independent and below the M1 hero. No invented online counts or activity.
Anonymous entry popup only on `/` and `/login`, fails open, immediately dismissible/Escape,
with a persistent close control and detail link. Local key
`chaldea.announcement.entry-dismissed.v1` stores id/revision/dismissed_at;
session key `chaldea.announcement.popup-seen.v1` stores presentation identity only.
Storage exceptions/corruption are caught; in-memory session dedupe still prevents route loops.
Successful authenticated detail render explicitly POSTs that response's revision.

The production runtime role must receive only announcement-domain grants at integration:
schema USAGE; SELECT on the required content/Ops tables; INSERT on content/notification
revisions, acknowledgements, reads, jobs, previews and operations; INSERT/UPDATE on the
announcement identity; INSERT/DELETE on placements; UPDATE on jobs; UPDATE privilege on
the fixed guard rows and `admin_principals.updated_at` solely to acquire FOR UPDATE locks.
No principal/scopes INSERT/DELETE, role-column UPDATE, history UPDATE/DELETE, schema DDL,
or blanket ALL grant is part of the runtime contract. This local candidate has not applied
or verified production-role grants. Startup never runs migrations; the existing explicit
migration command owns schema changes. The browser fixture is test-only and loopback-only.

Post-login automatic presentation is deliberately unconnected: current App exposes Master
completion but lacks a combined migration-notice-complete, return-intent-resolved and
global critical-business-state contract (including wallet unknown result and active rounds).
This batch supplies authenticated candidates and session dedupe, not an unsafe automatic gate.

Implementation sequence: contract → renderer red/green → schema/transaction/PG acceptance →
HTTP authorization → Web and tests → local 1440/390 browser verification → reviewed local commit.
Tests use a separate `m3_announcements_*` PostgreSQL database and synthetic identities only.
Evidence records renderer/XSS, timeline/archive, reads/re-notify, epoch/permissions, concurrent
version/window conflicts, restart recovery, atomic audit/jobs/idempotency, HTTP and browser
limits. Rollback candidate code independently; production migration/deployment requires a
separate integration decision and reviewed grants. Never drop content/audit history for rollback.

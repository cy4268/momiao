# M4b: access Gate and independent migration notice

Frozen source: Implementation Spec IS-FRZ-134 and §168 / IS-FRZ-154.

- Order: native account status → durable Master initialization → migration notice → route role/scope → deployment resource/maintenance declaration → consume navigation-only return intent.
- `GET /platform/v1/access-gate?route=<whitelisted path>` is a server-derived navigation decision, not an authorization credential. Existing native/platform handlers retain their own authoritative identity, permissions and resource checks; passing this Gate does not grant API rights or implement the frozen game/asset chain.
- `GET /platform/v1/migration-notice` reads the newest persisted requirement for the native session user. It is separate from ordinary Announcements, popup campaigns and announcement read state.
- `POST /platform/v1/migration-notice/acknowledge` accepts only `{"version":"<canonical positive int64>"}` with the same-origin session. Owner identity is derived from native self, never supplied by the browser.
- Schema 0009 adds immutable completed notice versions, per-user requirements and per-user/version ACKs. ACK uses `INSERT ... ON CONFLICT DO NOTHING`; retries return the original version/time even after a newer notice is published. The next Gate read still checks the newest requirement. ACK has no quota/reset/grant/profile/key migration side effects.
- Lost ACK response disables a blind repeat and offers an authoritative Gate reread. Existing ApiClient memory session generation remains the stale-response boundary. Feature children do not mount before READY.
- `normalizeRouteIntent(path)` in `web/src/post-auth-intent.ts` is the extensible client whitelist; `gateRouteDomain` is its server policy counterpart. New implemented M3c model/access routes must extend both together. The existing `chaldea.post-auth.route.v2` contains only a whitelisted route and expiry; arbitrary queries, credentials and write bodies are rejected. Consume it only after the same destination is READY.

## Controlled deployment declaration

`MOMIAO_ACCESS_GATE_DECLARATION_FILE` optionally points to an absolute deployment-managed JSON file. It is read at startup, must match `MOMIAO_PUBLIC_ORIGIN`, rejects duplicate/unknown keys and unsupported values, and is limited to 16 KiB. Protect ownership/write ACLs; Unix group/world-writable files are rejected. Changing the file requires a service restart.

Fields: `version: 1`, `environment: DEVELOPMENT|STAGING|PRODUCTION`, exact `origin`, nonblank bounded `evidence_ref`, `migration_applicability`, and `resources`.

- Applicability: `UNVERIFIED`, `PERSISTED_COMPLETED_NOTICE`, or explicitly reviewed `NO_MIGRATION_APPLICABLE`. Missing file/requirement is UNVERIFIED, not proof of a new installation or completed cutover. Only explicit no-applicability permits NOT_REQUIRED, and existing persisted requirements always take precedence.
- Resource keys: `ACCOUNT`, `API`, `COMMUNITY`, `OPERATIONS`, `ASSETS`, `EXPERIENCE`; values: `AVAILABLE`, `MAINTENANCE`, `UNAVAILABLE`, `UNVERIFIED`. Missing keys are UNVERIFIED, never AVAILABLE.
- This declaration records independently verified deployment facts. It neither performs a cutover nor publishes notice versions or requirements. There is deliberately no public browser publishing/cutover endpoint.

Production applicability/opening remains a product decision: independent new site versus opening only after verified legacy migration. This candidate creates no production declaration, requirements, ACKs, rewards or cutover facts, and does not alter the old site or its retained data.

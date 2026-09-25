# Public model workshop

The public directory groups by the reviewed original `metadata.family`, not a reseller prefix. One card contains the approved family portrait and family name only. Da Vinci is the background hostess, not a model persona.

## API and navigation

- `GET /platform/v1/models?group_by=family`: apply all existing filters, rank matching models within each family using the requested stable sort, select one representative, then paginate. `total` counts matching families; the optional `group_by` response field is `family`.
- Omitting `group_by` retains model-level counts, pagination and name/ID search semantics. Unknown, empty and duplicate grouping values are rejected.
- `/models?view=family&family=deepseek` lists **all** published models in that family using the existing model-level API. Directory-only search/recommendation/price filters do not hide siblings. Family pagination is independent.
- A selected `model_id` remains an exact, opaque channel/model identity. Details, prices, login intent, access checks and cURL examples never use a family name as a billing identity. Old `/models/<encoded-id>` links remain valid.
- Selecting another model invalidates pending access navigation. Personal quotes are scoped to the current session generation and disappear on logout.

## Art delivery

`internal/platform/catalog-personas.json` is the reviewed nine-persona registry. The original selected images are preserved; new WebP delivery files are lossless format encodings with identical decoded RGBA pixels, not redesigned characters. Portraits use contain sizing with a five-percent display inset, not a claimed normalized pixel margin. The historical prompt archive is unchanged.

The `web/src/catalog-workshop-art.json` background and hashed `/assets/models/` persona keys are served by the configured `VITE_ASSET_BASE_URL`. The existing CDN bucket contains the ten files. A narrowly scoped successful-response rule (200/206/304, exact immutable keys on the asset host) adds `public, max-age=31536000, immutable`; failed responses are not given this policy. No account identifiers or credentials are stored in source.

Desktop pages keep a single viewport with internal scrolling for long lists/details. Mobile pages use natural vertical scrolling. The existing operator catalog theme is untouched. Load, empty, stale/expired, denied access and image-error states remain explicit.

## Focused verification

Reuse `npm test -- src/Catalog.test.tsx`, `TestCatalogHTTPPublicAndStrictQueries`, and `TestCatalogPublicFilterSortAndPriceUnits` (with the existing isolated PostgreSQL test setup). One additional frontend regression covers exact channel selection and a late access response; existing assertions cover grouping before pagination, normal ungrouped behavior, anonymous access, quote privacy and placeholder-only cURL copying. Existing Home/Workspace navigation assertions also follow the new directory heading and grouped public URL; their private-read and prompt-clearing checks remain intact.

This stage does not deploy the website or merge the feature branch. Deploy the paired backend/frontend revision together at the eventual release; the directory requires the optional grouping support.

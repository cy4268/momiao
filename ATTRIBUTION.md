# Attribution and rights

## Repository license

Original momiao code and project-authored documentation are offered under **GNU Affero General Public License version 3 only (`AGPL-3.0-only`)**. See [LICENSE](LICENSE) for the unmodified standard license text. No later-version option is granted by this repository's license designation.

## Official NewAPI reuse

Local authentication and related account/key behavior are adapted from the clean official [QuantumNous/new-api](https://github.com/QuantumNous/new-api) revision `f116414284162ad15d8925f7bca494c109b83e93`. Adapted files identify their origin and modifications in source comments. Model protocol adapters use the pinned official `github.com/QuantumNous/new-api/relaykit` module recorded in `go.mod`; dependency notices stay with their distributions.

Production builds emit the original LICENSE, NOTICE and THIRD-PARTY-LICENSES.md under `/notices/new-api/` as text assets. The image also includes them under `/app/licenses/new-api/`; the homepage, login and account shell show the upstream attribution and project link.

The unmodified upstream [LICENSE](docs/upstream/new-api/LICENSE) and [NOTICE](docs/upstream/new-api/NOTICE) are retained separately from the deleted VPS-customized Native source patches and deployment tooling. The legacy Native login bridge is not the source of the local authentication implementation. New API contributors retain their authorship; no affiliation or endorsement is implied.

## Design archive and third-party assets

The six Markdown documents under `docs/design/archive/` are historical project design references, not source code or artwork from a third-party product. Their original project name, product/character references and textual visual intent are retained for traceability; private file locations and session-image identifiers are replaced by descriptive example paths.

No third-party character images, screenshots, reference art or logos are included. The portal crest is an original inline vector design. References to FGO/Fate, characters, model brands and other third-party names confer no copyright, trademark or publicity rights. Their rights remain with their respective holders. Project text or prompts describing intended artwork do not establish permission to distribute any resulting or referenced asset; asset-specific rights must be checked before inclusion or publication.

## Marcellus font

The portal locally bundles Marcellus through `@fontsource/marcellus` 5.3.0. Its original copyright notice and SIL Open Font License 1.1 are retained without modification in [web/public/fonts-license.txt](web/public/fonts-license.txt), which is copied to `/fonts-license.txt` in the built site. The font is governed by that license, not relicensed under this repository's AGPL designation. Other package dependencies keep their respective license notices in their distributions and are locked in `web/package-lock.json`.

## Guangxi homepage mascot and Cubism

The homepage uses the project-delivered Guangxi v1 model and the approved v2 preview, copied without re-encoding under `web/public/assets/guangxi/v1/`. It is a separate project character, not an official FGO/Fate character or endorsement. Asset hashes and integration details are recorded in [docs/guangxi-mascot.md](docs/guangxi-mascot.md).

Cubism Core, Framework and WebGL shaders are from the user-supplied **Cubism SDK for Web 5-r.5** archive (SHA-256 `67064a7fb1812cf502f5c4a03bfe12cc638c75a621bb4acf06bb28763df06ba0`). The compiled Framework is in `web/vendor/cubism-r5`; the matching Core and shaders are in `web/public/vendor/cubism-r5`. Original SDK, Core and Framework licenses and NOTICE remain in that public directory. These components retain Live2D's copyrights and their own license terms; the repository's AGPL designation does not relicense them. No SDK sample characters are included. The vendor manifest records the release metadata, source comparison and file hashes.

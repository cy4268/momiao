# Native-only executable promotion / rollback

`deploy.py` reuses the existing exact-input promotion pattern: expected-old
identity/hash checks, private backups, and `compose up --no-deps ... newapi`.
It is not an initial provisioner. Application source and existing DB data stay
outside its file mutations. Both feature switches default to false.

1. Fill a private copy of `deployment-request.example.json`. Resolve every
   REQUIRED field from the concentrated release review, including archive and
   coordinated current native/platform backup checksums. Backup files are only
   checksum-verified here; this entry neither creates nor restores DB backups.
2. Offline plan: `python3 -I /ABS/native-release/deploy.py /PRIVATE/request.json`.
   This does not read secret files, query the host, invoke Docker or run a migration.
3. Only the explicitly approved release command adds `--apply`. It checks the
   old native/other container identities, Compose/env/guard hashes, exact image
   archive/config/payload hashes and restricted private inputs. It then creates
   a new `/opt/momiao-native/releases/RELEASE_ID/rollback` private backup/receipt.
4. The dedicated native migration runs before replacement. The DSN must use the
   existing native database owner and exact private native target; no new roles,
   platform migration, seed administrator, grants, reward or quota operation is
   added. Credentials reach Docker by environment name, never argument value.
5. Only newapi is recreated; HTTP namespace READY precedes quota/Discord READY.
   HTTP guard bytes and native.env remain unchanged. Quota's single image literal
   changes exactly once. Native capabilities, private namespace and DB-only mode
   stay fixed. Secure cookies are explicit approval: preserve SESSION_SECRET and
   existing trusted origins, adding only the admission file's HTTPS origin.

The accepted image's containerd identity is `sha256:6d7062ca03efec8fd15cf78c1127f3442ff2c9b2a51558b6804489d082d50af7`;
its config identity is `sha256:ea1ca6e8d4ab54a451da5ea305949f6dde6231181730b70834201860b204a766`.
Select the actual target engine representation as `loaded_image_id` after the
offline image transfer review. No tag-only trust or replacement binary is allowed.

On failure, the entry stops without declaring success and returns the private
receipt when created. Inspect it before the separate rollback command:
`python3 -I /ABS/native-release/deploy.py /PRIVATE/rollback/state.json --rollback --apply`.
Without `--apply`, this is also only a plan. Rollback checks recorded identities
and file hashes, stops/removes only this batch's Discord units, restores Compose
and quota guard bytes, and recreates only the old native image if necessary.
The HTTP guard/env must still match their backups. Current databases, unique
binding protection, schema increments, sessions, receipts and both quota journals
are retained. No automatic retry, refund, SQL down migration or DB restore exists.

Local verification is only `test_deploy.py` plus Python syntax compilation.
No deployment, migration, image/container suite or real OAuth was executed by
this entry's preparation batch. Portal wiring and real OAuth remain separate.

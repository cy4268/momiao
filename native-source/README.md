# Native source integration

This is the complete current source delta against the pinned upstream revision, replacing the historical incremental replay chain for a fresh checkout. It includes backend integration and the isolated Native authentication UI; it contains no runtime configuration, credentials, databases or build output.

```sh
git clone https://github.com/QuantumNous/new-api ../native
git -C ../native checkout f116414284162ad15d8925f7bca494c109b83e93
git -C ../native apply --check ../momiao/native-source/current.patch
git -C ../native apply --index ../momiao/native-source/current.patch
```

Use the patch path relative to your actual checkout if its directory is not named `momiao`. Install the pinned Native web dependencies from `bun.lock`, then run `node scripts/build-native-auth-web.mjs ../native/web` from the platform root. Native source and modifications retain the upstream AGPL-3.0 license and notices. The original upstream LICENSE, NOTICE and THIRD-PARTY-LICENSES.md must remain with the reconstructed source and distributions.

For the Platform build, `web/.env.production` selects the opaque BFF Cookie session. Override it explicitly only when intentionally using the independent Native-client mode. Supply private runtime configuration separately; this source checkout does not activate any deployment or migrate data.

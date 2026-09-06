# Fixed Discord L4 egress

Reuses the reviewed socat/systemd fixed-upstream pattern. The only outbound
destination is the literal `TCP4:discord.com:443`. There is no target parameter,
CONNECT parser, host TCP listener, new route, container network change, TLS
termination, certificate override or native ELF change.

`native HTTPS discord.com:443 -> namespace 127.0.0.1:443 -> 0600 Unix socket ->
host fixed TCP discord.com:443`. Host DNS resolves the fixed destination; the
unchanged native Go client retains HTTPS URL, SNI and certificate verification.
This is L4 confinement, not HTTP path filtering; native code fixes OAuth paths.

## Minimal target configuration (not executed by this batch)

1. Confirm existing `momiao-egress` identity, socat, original namespace guard,
   namespace path/state and loopback443 availability. Keep the old NIM bridge.
2. Install these four units and `sync.py` at `/opt/momiao-discord/sync.py`,
   root-owned, not writable by the service user. Validate units before reload.
   Enable `momiao-discord-upstream.service` and `momiao-discord-sync.timer`,
   then start the sync service; never enable the bridge directly without guard.
3. The controller's current target inspection on 2026-09-06 found
   `net.ipv4.ip_unprivileged_port_start=0`, matching local Docker. Keep both
   capability sets empty: this deployment needs no added low-port capability.
   Do not grant NET_ADMIN/SYS_ADMIN or change the namespace sysctl. If the
   measured threshold changes, stop this deployment for a bounded review.
4. Merge `compose.discord.example.json` into only the native service: exact
   hosts mapping only. Current target HTTP(S)/ALL proxy variables are empty;
   retain that property so an inherited proxy does not divert Discord traffic.
   No proxy variables or host-side Discord resolver override are added.
5. Verify socket 0600 and its parent0700, correct service UID, loopback-only
   listener in the approved namespace, no host TCP443 listener added, active
   guarded bridge and end-to-end TLS. Missing/changed namespace closes at sync;
   missing upstream closes connections. Real OAuth remains manual acceptance.

Guard reconciliation follows the existing 15-second cadence, not instantaneous
container lifecycle notification. Units are not installed or started locally;
the actual socat commands are exercised in two private synthetic namespaces.
See the adjacent acceptance record. Stop this timer/bridge/upstream and remove
only this exact hosts addition to revert; native data remains intact.

Local check: run `test_egress.py` with Python3/OpenSSL/socat inside
`unshare --user --map-root-user --mount --net --fork`. It creates only synthetic
TLS material, binds a private hosts fixture, exercises verified/wrong-host/
untrusted TLS, rejects CONNECT semantics and closes after upstream removal.
It also checks the reconciler with synthetic OS boundaries. For an extracted
socat, set `SOCAT_TEST_BINARY` and its library path; no package install is needed.

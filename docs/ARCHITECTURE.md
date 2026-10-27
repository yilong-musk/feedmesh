# Architecture and Operations

## Design constraints

- **Standard library only**: `go.mod` must not contain a single `require`. Before adding a dependency, decide whether it is worth giving up "one binary, zero dependencies".
- **Single static binary**: `CGO_ENABLED=0`, about 7.7 MB, roughly 15 MB resident for RSS.
- **Output discipline**: stdout carries data only (RSS XML or one line of JSON), stderr carries JSON line logs only; never interactive.

## Data flow

```
CLI / HTTP
   ↓
feed.Service (cache, single-flight, error classification)
   ↓
provider packages (weibo / sspai / v2ex / xnitter, HTTP and parsing only)
   ↓
feed.Item (renderer-agnostic unified model)
   ↓
RSS 2.0 + Media RSS / JSON
```

Each provider knows only its own upstream; `feed` knows nothing about URLs, request headers, or field names.

## Cache

- Location: `--cache-dir` / `FEEDMESH_CACHE`, defaulting to `~/.cache/feedmesh` (`~/Library/Caches/feedmesh` on macOS). When `os.UserCacheDir()` is unavailable, a random private temporary directory is used instead of falling back to the shared `/tmp/feedmesh`; that fallback directory is deleted when the process exits.
- Layout: one JSON file per key, filename derived by sanitizing the key (`profile:2803301701` → `profile_2803301701.json`).
- Permissions: directory 0700, files 0600; a cache directory that is a symlink is refused.
- Atomic writes: temporary file → `fsync` → `rename`; concurrent writes are mutex-protected, so readers never see a half-written file.
- Expired entries are cleaned up by `GC()` at startup and hourly while the service runs; on read they are also treated as a miss and deleted. Corrupt entries (not JSON), or files whose ownership cannot be verified through `Entry.Key`, are treated as a miss on read and are not deleted; `GC()` likewise will not delete unverifiable files on the strength of a filename prefix alone, and it cleans up `.json.tmp*` temporary files older than 1 hour. Before deleting, `GC()` re-reads the file and compares its contents, so it does not remove a new entry that another process has just written.
- Purge scope: `cache purge --all` deletes only feedmesh entries whose `Entry.Key` matches the filename, so other `.json` files in the same directory are left untouched; `--uid` deletes `longtext_<uid>_*`, additionally reads the legacy timeline to collect longtext ids and scans domain entries (the discovery phase ignores expiry, so expired domain/timeline entries get cleaned too); `--handle` deletes the matching X timeline.
- **limit awareness**: list caches record `{items, want}`; when a request asks for more items than the cached width, it goes upstream and widens the cache, so a small `limit` cannot truncate a cache built with a large `limit`; `refresh=1` also preserves the existing width while writing the new data. Reads / widening / write-backs are serialized per key, so concurrent requests with different limits cannot narrow the cache again.

| key | TTL |
|---|---|
| `timeline:<uid>` | 5 minutes |
| `profile:<uid>` | 1 hour |
| `longtext:<uid>:<id>` | 24 hours |
| `domain:<name>` | 24 hours |
| `hot:top` | 5 minutes |
| `sspai:list` | 10 minutes |
| `sspai:detail:<id>` | 24 hours |
| `v2ex:latest` / `v2ex:hot` | 10 minutes |
| `x:<handle>` | 40 minutes |
| `youtube:feed:<channel_id>` | 10 minutes old counts as fresh; the entry is stored for 24 hours so the last good list survives an outage |
| `youtube:channel:<ref>` | 24 hours; an expired entry is reused when the channel page cannot be read |

## Upstream calling policy (Weibo)

- **Global concurrency 1**: a semaphore guarantees at most one in-flight request at any moment, to avoid triggering risk controls.
- **single-flight**: concurrent requests for the same key collapse into one; this only works within a single process, and HTTP and CLI share results through the disk cache.
- **Retries**: network errors / 5xx are retried 3 times with exponential backoff of 300ms → 600ms → 1200ms.
- **Breaker**: on 418/403, refresh the visitor cookie and retry first; if still blocked, open the breaker for 10 minutes, during which upstream calls immediately return `BREAKER_OPEN`; once the cool-off ends only one probe request is let through — success restores service, and another block opens the breaker again.
- The sspai client has a global 3-concurrency semaphore (effective across requests); V2EX completes in a single request. An explicit `--timeout` overrides all providers; the defaults are 9 seconds for Weibo, 15 seconds for sspai/V2EX and 20 seconds for X (a single X attempt is capped at 25 seconds).
- HTTP layer: every request has a 3-minute handler deadline; `http.Server` sets `ReadTimeout`/`WriteTimeout`; `ErrorLog` is wired to `logx`; panics are turned into a 500 JSON response by the recovery middleware.
- Security: optional Bearer token (`--token`/`FEEDMESH_TOKEN`); per-IP rate limiting over a rolling one-minute window (`--rate-limit`/`FEEDMESH_RATE_LIMIT`); when listening on a non-loopback address with no limit configured, the default is 120 requests/minute/IP. With a token enabled, RSS responses use `Cache-Control: private` and carry `Vary: Authorization` so shared caches cannot bypass authentication. Upstream HTML is allowlist-sanitized at the provider boundary, and XML-illegal control characters are stripped before CDATA output.
- X/Nitter: the instance pool accepts only addresses that are https, carry no userinfo, and are not IP literals or private-network domains, and the resolved IP is verified to be a public address at dial time; cross-host redirects are forbidden; PoW difficulty is capped at 12, challenge data at 2 KB, the nonce at 2 million, and at most 20 items per page; the pool tries at most 3 nodes (20 seconds per attempt by default, 25 seconds maximum), and the built-in fallback is still attempted after all of them fail. Third-party directories can still be poisoned, so pairing this with an egress firewall is recommended.

## Directory layout

```
cmd/feedmesh/       CLI: flag parsing, six subcommands, exit code mapping
internal/weibo/     Weibo HTTP and parsing (including hot search)
internal/sspai/     sspai JSON API
internal/v2ex/      V2EX Atom + hottest page
internal/smzdm/     SMZDM official deal RSS
internal/xnitter/   X timelines: Nitter instance pool + Anubis PoW + RSS parsing
internal/feed/      unified model, RSS rendering, cache orchestration
internal/cache/     key-per-file cache, TTL, atomic writes
internal/httpapi/   HTTP routing and error envelope
internal/guard/     breaker + single-flight
internal/logx/      stderr JSON line logging
deploy/             systemd user unit
tools/              repository gates: Go-only language check and RSS validator
docs/               API, platform list, this file
```

## Deployment (systemd user unit)

`deploy/feedmesh.service` can be installed straight into `~/.config/systemd/user/`: `%h` / `%C` expand to the user's home directory and `~/.cache` respectively, `CacheDirectory=feedmesh` creates the directory (`0700`), `ReadWritePaths=` opens up only the cache directory, and `ProtectSystem=strict` makes the rest of the filesystem read-only.

```bash
install -m0644 deploy/feedmesh.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now feedmesh
```

After installing, verify each item one by one (skip one and it is not delivered):

```bash
systemctl --user is-active feedmesh            # active
systemctl --user show feedmesh -p Result -p NRestarts --value
curl -s http://127.0.0.1:3000/healthz          # status/version/uptime/cache/breaker
```

**Do not add `ProtectKernelModules=` to this user unit.** Besides mounting `/usr/lib/modules`, that option also removes `CAP_SYS_MODULE` from the capability bounding set (the difference is visible in `systemctl show -p CapabilityBoundingSet`); modifying the bounding set requires `CAP_SETPCAP`, and the user manager runs as an ordinary user (`CapEff`/`CapPrm` are both 0), so `prctl(PR_CAPBSET_DROP)` returns EPERM and systemd declares failure outright: `status=218/CAPABILITIES`, `Failed to drop capabilities: Operation not permitted`, and the binary in `ExecStart` never runs at all. `PrivateUsers=yes` does not rescue it either — it has to create a user namespace first, and on hosts with `kernel.apparmor_restrict_unprivileged_userns=1` enabled (the Ubuntu 24.04 default) even the uid mapping is refused. If you genuinely need module-loading protection, convert the unit to system level (the system manager has `CAP_SETPCAP`).

The remaining hardening options (`ProtectSystem` / `ProtectKernelTunables` / `ProtectControlGroups` / `RestrictAddressFamilies` / `RestrictSUIDSGID` / `LockPersonality` / `MemoryDenyWriteExecute` / `SystemCallArchitectures` / `UMask` / `NoNewPrivileges`) do not modify the bounding set, and all of them have been verified to work under a user unit.

## Security and privacy

- **Binding and authentication**: by default it listens on `127.0.0.1` only; to expose it to a LAN or the public internet, add `--token` (`FEEDMESH_TOKEN`) and put it behind a TLS reverse proxy. Starting on a non-loopback address prints a warning, and without an explicit limit the default 120 requests/minute/IP rate limit applies (implementation details under "Upstream calling policy").
- **Upstream content**: allowlist sanitization happens at the provider boundary — `script`/`iframe`, `on*` event attributes and `javascript:` URLs are dropped; XML-illegal control characters are stripped before CDATA output. Neither the RSS nor the JSON should be rendered as trusted HTML.
- **Outbound requests**: the Nitter instance pool comes from third-party directories, so only https, userinfo-free, non-private addresses are accepted, and the resolved IP is verified to be a public address at dial time; cross-host redirects are forbidden. Third-party directories can still be poisoned, so an egress firewall at the host or container layer is recommended, blocking loopback, RFC1918 and link-local addresses.
- **Trigger cost**: `refresh=1` and `full=1` trigger real upstream requests, so public deployments need to restrict access frequency.
- **Third-party visibility**: the Weibo UID is sent to Weibo, the X handle is sent to the Nitter instance pool, and sspai/V2EX see the server IP; images in the RSS are loaded by the reader directly from third-party image hosts. Self-hosting is not anonymity.
- **Local data**: the cache directory defaults to `~/.cache/feedmesh` (`~/Library/Caches/feedmesh` on macOS) and holds account/domain/handle identifiers and article bodies, with directory permissions 0700 and file permissions 0600; expired entries are cleaned up at startup and hourly while running; when `os.UserCacheDir()` is unavailable a random private temporary directory is used and deleted when the process exits. `cache purge --all`, `--uid` and `--handle` allow targeted cleanup.
- **Compliance**: scraping the Weibo visitor cookie, solving Anubis, and fetching full text from V2EX/sspai may fall under platform terms and content copyright; confirm your own use case.

## Troubleshooting

**When Weibo changes its API, only `internal/weibo` needs to change.** Every other package consumes already-parsed structs.

```bash
feedmesh doctor --json                        # 1. which step is broken
feedmesh fetch 2803301701 --refresh --json    # 2. one account, end to end
journalctl --user -u feedmesh -n 50           # 3. service-side logs
```

| Symptom | Likely cause | What to change |
|---|---|---|
| `doctor` fails on the first step | the visitor endpoint or the cookie names (`SUB`/`SUBP`) changed | `fetchVisitor` in `internal/weibo/client.go` |
| Exit code 4 / `upstream blocked` | missing cookie, UA being fingerprinted, or genuinely rate limited | `DefaultUserAgent`, request headers; breaker cool-off `breakerCooloff` |
| Exit code 5 even though the account exists | the field used to detect "user does not exist" changed (the status is in the top-level `ok`) | `Profile` in `internal/weibo/api.go` |
| Timeline empty but the profile is fine | how `containerid` is derived changed (`tabs[tab_type=="weibo"]`) | the tab matching in `Profile` |
| Long text comes back truncated | the `statuses/extend` field was renamed | `LongText` / `StatusText` |
| `created_at` becomes the fetch time | the time format changed | `createdLayouts` in `internal/feed/service.go` |
| Lots of `3` timeouts | the overseas host reaches upstream slowly | raise `--timeout` (defaults: Weibo 9s; sspai/V2EX 15s; X 20s) |
| Exit code 6 | breaker open, recovers automatically in 10 minutes | no action needed |
| `/rss/x/*` all return 504 | the Nitter instance pool is dead as a whole (`rss=true` nodes in status.d420.de rotate) | first hit the X routes of `feedmesh` with `refresh=1`; if it keeps failing, update `defaultInstances` in `internal/xnitter`, or switch `StatusURL` |
| X timeline returns 0 items but HTTP 200 | the instance changed its RSS structure or switched to a JS shell | `parseRSS` in `internal/xnitter/client.go`; compare against the real `/{handle}/rss` output |
| X scraping occasionally stalls on a verification page | the instance changed its Anubis challenge format | `parseChallenge` (`anubis_challenge` / `preact_info`) |

## Development

```bash
go build ./... && go vet ./... && go test ./...

make check                       # vet + test + gofmt + language gate
bash scripts/acceptance.sh       # full acceptance run (requires the service on :3000)
go run ./tools/rsscheck f.xml    # validate that the RSS is well-formed
```

CI (GitHub Actions) runs the same `gofmt` / language / `vet` / `test -race` /
static build checks.

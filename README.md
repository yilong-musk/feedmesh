# feedmesh

[![CI](https://github.com/yilong-musk/feedmesh/actions/workflows/ci.yml/badge.svg)](https://github.com/yilong-musk/feedmesh/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

English | [Chinese (Simplified)](README.zh-CN.md)

Bridges content from multiple platforms into RSS — self-hosted, one static binary, no third-party dependencies (there is not a single `require` in `go.mod`), listening only on `127.0.0.1` by default. Supported platforms are listed below and detailed in [docs/PLATFORMS.md](docs/PLATFORMS.md); new platforms arrive as providers.

## Supported platforms

| Platform | Content | RSS | JSON |
|---|---|---|---|
| <img src="docs/assets/weibo.png" width="18" height="18" alt="Weibo"> Weibo | Account timeline | `/rss/user/{uid}`, `/rss/domain/{name}` | `/api/v1/feed?uid=` |
| <img src="docs/assets/weibo.png" width="18" height="18" alt="Weibo"> Weibo | Realtime hot search | `/rss/hot` | `/api/v1/hot` |
| <img src="docs/assets/sspai.png" width="18" height="18" alt="sspai"> sspai | Latest articles | `/rss/sspai` | `/api/v1/sspai` |
| <img src="docs/assets/v2ex.png" width="18" height="18" alt="V2EX"> V2EX | Latest topics | `/rss/v2ex` | `/api/v1/v2ex?section=latest` |
| <img src="docs/assets/v2ex.png" width="18" height="18" alt="V2EX"> V2EX | Hot topics | `/rss/v2ex/hot` | `/api/v1/v2ex?section=hot` |
| <img src="docs/assets/smzdm.png" width="18" height="18" alt="SMZDM"> SMZDM | Selected deals | `/rss/smzdm` | `/api/v1/smzdm` |
| <img src="docs/assets/smzdm.png" width="18" height="18" alt="SMZDM"> SMZDM | Community articles | `/rss/smzdm/post` | `/api/v1/smzdm?channel=post` |
| <img src="docs/assets/smzdm.png" width="18" height="18" alt="SMZDM"> SMZDM | Overseas deals | `/rss/smzdm/haitao` | `/api/v1/smzdm?channel=haitao` |
| <img src="docs/assets/x.png" width="18" height="18" alt="X"> X | Account timeline | `/rss/x/{handle}` | `/api/v1/x?handle=` |
| <img src="docs/assets/youtube.png" width="18" height="18" alt="YouTube"> YouTube | Channel uploads | `/rss/youtube/{handle\|id}` | `/api/v1/youtube?ref=` |

Parameters, data sources and limits live in **[docs/PLATFORMS.md](docs/PLATFORMS.md)**.

## Features

- **Complete RSS**: body HTML, images (real dimensions plus lazy loading), repost quotes, link cards, long-form full text; `content:encoded` and Media RSS in the same document, with `enclosure` for Weibo videos.
- **Gentle on upstreams**: on-disk cache with limit-aware widening (a small request never truncates a larger cached one) and single-flight; Weibo concurrency stays at 1, cookies are refreshed on 418/403, and a persistent failure opens a 10-minute breaker.
- **Two views**: RSS for readers, JSON for scripts and agents (one line per response).
- **Single binary**: statically linked (CGO off), about 8.2 MB, roughly 11 MB of RSS at rest.

## Quick start

Requires Go 1.27+.

```bash
git clone https://github.com/yilong-musk/feedmesh.git
cd feedmesh

make check        # go vet + go test + gofmt
make install      # installs to ~/.local/bin/feedmesh
feedmesh serve --listen 127.0.0.1:3000
```

Try it:

```bash
curl -s 'http://127.0.0.1:3000/rss/hot?limit=5'
curl -s 'http://127.0.0.1:3000/rss/v2ex/hot?limit=5'
curl -s 'http://127.0.0.1:3000/healthz' | jq
```

### systemd (user unit)

```bash
mkdir -p ~/.config/systemd/user
cp deploy/feedmesh.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now feedmesh
loginctl enable-linger "$USER"     # keep it running without an active login
```

The unit uses systemd's `%h` / `%C` expansion for the home and cache directories, so no username substitution is needed; adjust `ExecStart` if the binary lives elsewhere.

## CLI

| Command | What it does |
|---|---|
| `serve --listen <addr>` | Start the HTTP server; optional `--token` / `--rate-limit` |
| `fetch <uid\|domain>` | Print one Weibo account timeline (RSS; `--json` prints a single JSON line) |
| `doctor [--uid <uid>]` | Probe visitor, getIndex and statuses in order |
| `cache stat` | Cache entry count and size |
| `cache purge --all` | Remove every feedmesh cache entry |
| `cache purge --uid <uid>` | Remove one account's profile/timeline/domain/feed/long-text entries |
| `cache purge --handle <handle>` | Remove one X handle's timeline entries |
| `--dry-run` | Report only, delete nothing |

Global flags: `--json`, `--quiet`, `--timeout`, `--cache-dir`, `--refresh`. `--timeout` defaults to 9s for Weibo, 15s for sspai/V2EX/SMZDM and 20s for X/YouTube; an explicit value overrides every provider, but a single X attempt never exceeds 25s. stdout carries data only; stderr carries JSON log lines.

## Documentation

- **[API](docs/API.md)**: endpoints, parameters, JSON shapes, error codes, RSS output details, cache TTLs
- **[Platforms](docs/PLATFORMS.md)**: rules and limits per source, plus the steps for adding a provider
- **[Architecture and operations](docs/ARCHITECTURE.md)**: cache implementation, upstream policy, security and privacy, layout, troubleshooting, development commands
- **[Chinese (Simplified)](README.zh-CN.md)**: this README in Simplified Chinese; the English pages stay the source of truth

## License

[MIT](LICENSE)

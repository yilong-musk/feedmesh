# feedmesh

[![CI](https://github.com/yilong-musk/feedmesh/actions/workflows/ci.yml/badge.svg)](https://github.com/yilong-musk/feedmesh/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

[English](README.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | [Español](README.es.md) | [Français](README.fr.md) | Deutsch | [Português](README.pt.md) | [Русский](README.ru.md)

Führt Inhalte mehrerer Plattformen zu RSS zusammen — selbst gehostet, ein statisches Binary, keine Drittabhängigkeiten (in `go.mod` steht kein einziges `require`), standardmäßig nur auf `127.0.0.1`. Die unterstützten Plattformen stehen unten und im Detail in [docs/PLATFORMS.md](docs/PLATFORMS.md); neue Plattformen kommen als Provider dazu.

## Unterstützte Plattformen

| Plattform&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp; | Inhalt | RSS | JSON |
|---|---|---|---|
| <img src="docs/assets/weibo.png" width="18" height="18" align="absmiddle" alt="Weibo">&nbsp;Weibo | Timeline eines Kontos | `/rss/user/{uid}`, `/rss/domain/{name}` | `/api/v1/feed?uid=` |
| <img src="docs/assets/weibo.png" width="18" height="18" align="absmiddle" alt="Weibo">&nbsp;Weibo | Echtzeit-Trendsuche | `/rss/hot` | `/api/v1/hot` |
| <img src="docs/assets/sspai.png" width="18" height="18" align="absmiddle" alt="sspai">&nbsp;sspai | Neueste Artikel | `/rss/sspai` | `/api/v1/sspai` |
| <img src="docs/assets/v2ex.png" width="18" height="18" align="absmiddle" alt="V2EX">&nbsp;V2EX | Neueste Themen | `/rss/v2ex` | `/api/v1/v2ex?section=latest` |
| <img src="docs/assets/v2ex.png" width="18" height="18" align="absmiddle" alt="V2EX">&nbsp;V2EX | Beliebte Themen | `/rss/v2ex/hot` | `/api/v1/v2ex?section=hot` |
| <img src="docs/assets/smzdm.png" width="18" height="18" align="absmiddle" alt="SMZDM">&nbsp;SMZDM | Ausgewählte Angebote | `/rss/smzdm` | `/api/v1/smzdm` |
| <img src="docs/assets/smzdm.png" width="18" height="18" align="absmiddle" alt="SMZDM">&nbsp;SMZDM | Community-Artikel | `/rss/smzdm/post` | `/api/v1/smzdm?channel=post` |
| <img src="docs/assets/smzdm.png" width="18" height="18" align="absmiddle" alt="SMZDM">&nbsp;SMZDM | Angebote aus dem Ausland | `/rss/smzdm/haitao` | `/api/v1/smzdm?channel=haitao` |
| <img src="docs/assets/smzdm.png" width="18" height="18" align="absmiddle" alt="SMZDM">&nbsp;SMZDM | Gute Preise nach Kategorie | `/rss/smzdm/fenlei/{slug}` | `/api/v1/smzdm?channel=fenlei&slug=` |
| <img src="docs/assets/x.png" width="18" height="18" align="absmiddle" alt="X">&nbsp;X | Timeline eines Kontos | `/rss/x/{handle}` | `/api/v1/x?handle=` |
| <img src="docs/assets/youtube.png" width="18" height="18" align="absmiddle" alt="YouTube">&nbsp;YouTube | Kanal-Uploads | `/rss/youtube/{handle\|id}` | `/api/v1/youtube?ref=` |
| <img src="docs/assets/douyin.png" width="18" height="18" align="absmiddle" alt="Douyin">&nbsp;Douyin | Videos eines Creators | `/rss/douyin/{sec_uid}` | `/api/v1/douyin?user=` |
| <img src="docs/assets/tiktok.png" width="18" height="18" align="absmiddle" alt="TikTok">&nbsp;TikTok | Videos eines Creators | `/rss/tiktok/{handle}` | `/api/v1/tiktok?user=` |
| <img src="docs/assets/bilibili.png" width="18" height="18" align="absmiddle" alt="Bilibili">&nbsp;Bilibili | Videos eines Creators | `/rss/bilibili/{mid}` | `/api/v1/bilibili?user=` |
| <img src="docs/assets/telegram.png" width="18" height="18" align="absmiddle" alt="Telegram">&nbsp;Telegram | Beiträge eines öffentlichen Kanals | `/rss/telegram/{channel}` | `/api/v1/telegram?channel=` |
| <img src="docs/assets/pinterest.png" width="18" height="18" align="absmiddle" alt="Pinterest">&nbsp;Pinterest | Pins eines Profils oder Boards | `/rss/pinterest/{user}`, `/rss/pinterest/{user}/{board}` | `/api/v1/pinterest?user=&board=` |
| <img src="docs/assets/polymarket.png" width="18" height="18" align="absmiddle" alt="Polymarket">&nbsp;Polymarket | Neue oder trendende Ereignisse | `/rss/polymarket/new`, `/rss/polymarket/hot` | `/api/v1/polymarket?feed=new\|hot` |
| <img src="docs/assets/polymarket.png" width="18" height="18" align="absmiddle" alt="Polymarket">&nbsp;Polymarket | Quoten eines Ereignisses | `/rss/polymarket/event/{slug}` | `/api/v1/polymarket?feed=event&slug=` |
| <img src="docs/assets/polymarket.png" width="18" height="18" align="absmiddle" alt="Polymarket">&nbsp;Polymarket | Wallet-Aktivität | `/rss/polymarket/user/{address}` | `/api/v1/polymarket?feed=user&address=` |
| <img src="docs/assets/linuxdo.png" width="18" height="18" align="absmiddle" alt="LINUX DO">&nbsp;LINUX DO | Forenthemen (neu / top / heiß / Kategorie / Tag) | `/rss/linuxdo`, `/rss/linuxdo/top`, `/rss/linuxdo/hot`, `/rss/linuxdo/c/{slug}/{id}`, `/rss/linuxdo/tag/{tag}` | `/api/v1/linuxdo?feed=` |
| <img src="docs/assets/reddit.png" width="18" height="18" align="absmiddle" alt="Reddit">&nbsp;Reddit | Subreddit-Liste | `/rss/reddit/{sub}` | `/api/v1/reddit?sub=` |

Parameter, Datenquellen und Grenzen stehen in **[docs/PLATFORMS.md](docs/PLATFORMS.md)**.

## Funktionen

- **Vollständiges RSS**: HTML-Text, Bilder (echte Maße plus Lazy Loading), Repost-Zitate, Linkkarten, Langtext in voller Länge; `content:encoded` und Media RSS im selben Dokument, mit `enclosure` für Weibo-Videos.
- **Schonend zu den Upstreams**: Platten-Cache, der sich an `limit` anpasst (eine kleine Anfrage schneidet einen breiteren Cache nicht ab) und single-flight; Weibo bleibt bei Parallelität 1, Cookies werden bei 418/403 erneuert, anhaltende Fehler öffnen einen 10-Minuten-Breaker.
- **Zwei Sichten**: RSS für Reader, JSON für Skripte und Agents (eine Zeile pro Antwort).
- **Ein Binary**: statisch gelinkt (CGO aus), etwa 8,3 MB, rund 11 MB residenter Speicher.

## Installation

Statische Binaries für Linux, macOS und Windows (amd64/arm64) sowie Debian/RPM-Pakete
hängen an jedem [Release](https://github.com/yilong-musk/feedmesh/releases); Archive gegen
`checksums.txt` prüfen. Mit einer Go-Toolchain (1.27+) liegt das Hauptpaket in
`cmd/feedmesh`, der Installationspfad enthält es also:

```bash
go install github.com/yilong-musk/feedmesh/cmd/feedmesh@latest
```

### Homebrew

```bash
brew install --cask yilong-musk/tap/feedmesh
brew upgrade --cask feedmesh
```

Das Cask veröffentlicht die Release-Pipeline nach [yilong-musk/homebrew-tap](https://github.com/yilong-musk/homebrew-tap). Es installiert das vorgebaute macOS- oder Linux-Binary und entfernt das Quarantine-Attribut (die Archive sind nicht notarisiert).

### Docker

```bash
docker run -d --name feedmesh -p 127.0.0.1:3000:3000 \
  -v feedmesh-data:/data ghcr.io/yilong-musk/feedmesh:latest
```

Zu jedem Release gibt es Images für `linux/amd64` und `linux/arm64`. Sie laufen als Nicht-Root und halten den Cache im Volume `/data`. Jeder Provider funktioniert außer Douyin, dessen Reader Chromium braucht (`FEEDMESH_CHROME`).

## Schnellstart

Go 1.27 oder neuer ist nötig.

```bash
git clone https://github.com/yilong-musk/feedmesh.git
cd feedmesh

make check        # go vet + go test + gofmt
make install      # installiert nach ~/.local/bin/feedmesh
feedmesh serve --listen 127.0.0.1:3000
```

Ausprobieren:

```bash
curl -s 'http://127.0.0.1:3000/rss/hot?limit=5'
curl -s 'http://127.0.0.1:3000/rss/v2ex/hot?limit=5'
curl -s 'http://127.0.0.1:3000/rss/tiktok/nasa?limit=5'
curl -s 'http://127.0.0.1:3000/rss/bilibili/946974?limit=5'
curl -s 'http://127.0.0.1:3000/rss/telegram/sspai?limit=5'
curl -s 'http://127.0.0.1:3000/rss/linuxdo?limit=5'
curl -s 'http://127.0.0.1:3000/healthz' | jq
```

### systemd (User-Unit)

```bash
mkdir -p ~/.config/systemd/user
cp deploy/feedmesh.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now feedmesh
loginctl enable-linger "$USER"     # läuft weiter ohne aktive Anmeldung
```

Die Unit nutzt systemds `%h`- / `%C`-Ersetzung für Home- und Cache-Verzeichnis, ein Benutzername muss also nicht eingesetzt werden. `ExecStart` anpassen, wenn das Binary woanders liegt.

## CLI

| Befehl | Wirkung |
|---|---|
| `serve --listen <addr>` | Startet den HTTP-Server; optional `--token` / `--rate-limit` |
| `fetch <uid\|domain>` | Gibt die Timeline eines Weibo-Kontos aus (RSS; `--json` druckt eine JSON-Zeile) |
| `doctor [--uid <uid>]` | Prüft visitor, getIndex und statuses der Reihe nach |
| `doctor --x-handle <handle>` | Prüft den X-Instanzpool und jeden Spiegel für einen Handle |
| `cache stat` | Anzahl und Größe der Cache-Einträge |
| `cache purge --all` | Entfernt jeden feedmesh-Cache-Eintrag |
| `cache purge --uid <uid>` | Entfernt profile/timeline/domain/feed/Langtext eines Kontos |
| `cache purge --handle <handle>` | Entfernt die Timeline-Einträge eines X-Handles |
| `--dry-run` | Nur melden, nichts löschen |

Globale Flags: `--json`, `--quiet`, `--timeout`, `--cache-dir`, `--refresh`. `--timeout` ist standardmäßig 9 s für Weibo, 15 s für sspai/V2EX/SMZDM/Reddit/Pinterest/Polymarket, 20 s für X/YouTube, 30 s für TikTok, 40 s für Bilibili, 45 s für Douyin (ein kalter Browser braucht etwa eine halbe Minute), 15 s für Telegram und 20 s für LINUX DO; ein gesetzter Wert überschreibt jeden Provider, ein einzelner X-Versuch dauert aber nie länger als 25 s. stdout trägt nur Daten, stderr nur JSON-Logzeilen.

## Dokumentation

Englisch ist die maßgebliche Fassung; diese Seite ist die deutsche Übersetzung.

- **[API](docs/API.md)**: Endpunkte, Parameter, JSON-Formen, Fehlercodes, RSS-Ausgabedetails, Cache-TTLs
- **[Plattformen](docs/PLATFORMS.md)**: Regeln und Grenzen je Quelle, plus die Schritte zum Hinzufügen eines Providers
- **[Architektur und Betrieb](docs/ARCHITECTURE.md)**: Cache-Implementierung, Upstream-Politik, Sicherheit und Datenschutz, Aufbau, Fehlersuche, Entwicklungsbefehle
- **[English](README.md)**: englische Fassung, einzige maßgebliche Quelle (diese Seite ist die deutsche Übersetzung)

## License

[MIT](LICENSE)

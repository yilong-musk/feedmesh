# API

All endpoints are `GET`, and by default the service listens only on `127.0.0.1:3000`. The `Content-Type` is `application/rss+xml; charset=utf-8` for RSS responses and `application/json` for JSON responses.

With `--token <token>` or `FEEDMESH_TOKEN` set, every request must carry `Authorization: Bearer <token>`, otherwise it returns 401. With `--rate-limit <n>` or `FEEDMESH_RATE_LIMIT` set, each remote IP may make at most n requests within any rolling minute, and anything above that returns 429. If the listener is bound to a non-loopback address and no limit is configured, the service applies 120 requests/minute/IP by default. Behind a reverse proxy `RemoteAddr` is usually the proxy IP, so rate limiting at the proxy layer as well is recommended.

The mapping between platforms and routes is in [PLATFORMS.md](PLATFORMS.md).

---

## Common parameters

| Parameter | Description |
|---|---|
| `limit` | Number of items returned; per-platform defaults and maximums are in [PLATFORMS.md](PLATFORMS.md) |
| `refresh=1` | Skip the stale cache, force an upstream fetch, and overwrite the cache (the existing cache width is kept) |
| `full=1` | sspai only: fetch the article body (off by default) |
| `section=latest\|hot` | `/api/v1/v2ex` only: select latest or hot |
| `handle` | `/api/v1/x` only: X username (`@` allowed, 1–15 letters, digits or underscores) |

The `.xml` path suffix is equivalent to no suffix only on RSS routes (such as `/rss/hot.xml`); `/api/v1/*` does not accept a `.xml` suffix.

---

## Health check

### `GET /healthz`

```json
{"status":"ok","version":"16ae5de","uptime_s":8,
 "cache":{"entries":5,"bytes":29433},
 "breaker":{"open":false,"until":null}}
```

`?probe=1` additionally runs a real upstream liveness probe (the result is reused for 30 seconds so the upstream is not hammered):

```bash
curl -s 'http://127.0.0.1:3000/healthz?probe=1' | jq -c '.probe'
```

> `status` reflects process health only; whether dependencies are reachable shows up in `probe` and `breaker`. Keeping the two apart is what prevents reporting "all good" while the upstream is down.

---

## RSS endpoints

### `GET /rss/user/{uid}`

Weibo account timeline. `/rss/user/{uid}.xml` is equivalent.

```bash
curl -s http://127.0.0.1:3000/rss/user/2803301701 | head -8
```

### `GET /rss/domain/{name}`

A custom domain name (such as `rmrb`) is resolved to a uid first, then returned in the same format.

### `GET /rss/hot`

Realtime Weibo hot search. `limit` 1–50 (default 20); only ranks 1–50 of the main list are included.

```bash
curl -s 'http://127.0.0.1:3000/rss/hot?limit=10'
curl -s 'http://127.0.0.1:3000/rss/hot?refresh=1'   # bypass the 5-minute cache
```

### `GET /rss/sspai`

Latest sspai articles. Summary plus cover image by default; `full=1` fetches the article body.

```bash
curl -s 'http://127.0.0.1:3000/rss/sspai?limit=10'
curl -s 'http://127.0.0.1:3000/rss/sspai?limit=5&full=1'
```

### `GET /rss/v2ex`

Latest topics from the official V2EX Atom feed (full post text).

```bash
curl -s 'http://127.0.0.1:3000/rss/v2ex?limit=20'
```

### `GET /rss/v2ex/hot`

V2EX hot topics: the "promotions" node is filtered out and results are sorted by reply count descending; falls back to the official API when the page looks wrong.

```bash
curl -s 'http://127.0.0.1:3000/rss/v2ex/hot?limit=20'
```

### `GET /rss/smzdm`

Selected deals from the official SMZDM RSS channel (`http://feed.smzdm.com`).

```bash
curl -s 'http://127.0.0.1:3000/rss/smzdm?limit=20'
curl -s 'http://127.0.0.1:3000/rss/smzdm?limit=10&refresh=1'   # bypass the 5-minute cache
```

- 20 items by default, maximum 30 (one upstream slice); cached for 5 minutes, `Cache-Control: max-age=300`.
- `guid`/`link` is the deal permalink (`https://www.smzdm.com/p/<id>/`); the cover (`focus_pic`) goes into the body and Media RSS.
- `pubDate` is Beijing time when the feed omits the zone, and is converted to UTC in the JSON view.
- The upstream slice rotates, so items that leave it disappear from the feed; readers that keep history collect a wider set.

### `GET /rss/x/{handle}`

X (Twitter) account timeline, fetched through public Nitter instances; `{handle}` may include `@`.

```bash
curl -s 'http://127.0.0.1:3000/rss/x/elonmusk?limit=20'
```

- 20 items by default, maximum 20 (one page); cached for 40 minutes, `Cache-Control: max-age=2400`.
- Item `link`/`guid` is always `https://x.com/{handle}/status/{id}`; instance links inside the body are rewritten too.
- Repost titles are prefixed with `🔁`; images go through Media RSS.
- When every instance fails, returns `UPSTREAM_UNAVAILABLE` (504).

---

## JSON endpoints

Timestamps are always RFC 3339 (UTC); `images` is always an array (`[]` when there are no images, never `null`).

### `GET /api/v1/feed?uid=<uid>&limit=N`

```json
{"uid":"2803301701","screen_name":"People's Daily","description":"…",
 "fetched_at":"2026-10-23T06:55:12Z","cached":true,
 "items":[{"id":"…","bid":"…","created_at":"2026-10-23T06:45:32Z",
           "text":"…","html":"…","url":"https://weibo.com/2803301701/RnZRCA7Kf",
           "images":[],"video_url":"…","page_url":"…"}]}
```

`video_url` / `page_url` appear only when they have a value (`omitempty`).

### `GET /api/v1/hot?limit=N`

```json
{"title":"Weibo Hot Search","description":"Realtime Weibo hot search ranking","fetched_at":"2026-10-23T08:00:00Z",
 "cached":true,
 "items":[{"rank":1,"heat":1488601,"title":"1. Example topic","text":"Example topic",
           "url":"https://s.weibo.com/weibo?q=…","html":"<p>Heat 1.5M</p>"}]}
```

`heat` is the raw upstream counter as an integer, so clients never have to parse the display text in `html`.

### `GET /api/v1/sspai?limit=N&full=1`

```json
{"title":"sspai","description":"Latest sspai articles","fetched_at":"…","cached":true,
 "items":[{"id":"117169","title":"…","text":"Summary","url":"https://sspai.com/post/117169",
           "html":"…","images":["https://rssfile.sspai.com/…png"],"created_at":"…"}]}
```

### `GET /api/v1/v2ex?section=latest|hot&limit=N`

```json
{"title":"V2EX Hot","description":"Top V2EX topics, sorted by replies","fetched_at":"…","cached":true,
 "items":[{"id":"1251126","title":"…","text":"…","url":"https://www.v2ex.com/t/1251126",
           "html":"…","images":[],"created_at":"…"}]}
```

### `GET /api/v1/smzdm?limit=N`

```json
{"title":"SMZDM Deals","description":"Selected deals from SMZDM","fetched_at":"2026-10-25T14:12:08Z","cached":false,
 "items":[{"id":"184734707","title":"Brand model 620ml 28.84 CNY (buy 2, coupon)","text":"",
           "url":"https://www.smzdm.com/p/184734707/","html":"…",
           "images":["https://y.zdmimg.com/…png_a200.jpg"],"created_at":"2026-10-25T13:39:37Z"}]}
```

- `limit` defaults to 20 with a maximum of 30; `refresh=1` behaves as on the RSS route.
- `text` is empty for the common case (nickname-only entries); the price and conditions live in `title`.

### `GET /api/v1/x?handle=<handle>&limit=N`

```json
{"handle":"elonmusk","title":"elonmusk / X","description":"Latest posts from @elonmusk (via a Nitter instance)",
 "fetched_at":"…","cached":true,
 "items":[{"id":"2113508310267674684","title":"Try Grok @Bot","text":"…",
           "url":"https://x.com/elonmusk/status/2113508310267674684","html":"…",
           "images":["https://pbs.twimg.com/media/…jpg"],"created_at":"2026-10-23T05:50:22Z"}]}
```

- `handle` is required (`@` allowed); `limit` defaults to 20 with a maximum of 20.
- `refresh=1` is supported and behaves the same as on the RSS routes.

---

## Errors

A single envelope, the matching HTTP status code, and the `X-Feedmesh-Code` header (the legacy name `X-Weibo-RSS-Code` is also sent for compatibility):

```json
{"error":{"code":"USER_NOT_FOUND","message":"weibo user 999 not found: no content here yet"}}
```

| code | HTTP |
|---|---|
| `BAD_REQUEST` | 400 (405 for a wrong method, with an `Allow` header) |
| `UNAUTHORIZED` | 401 (missing or wrong Bearer token once `--token` is enabled) |
| `USER_NOT_FOUND` / `LOGIN_REQUIRED` | 404 |
| `UPSTREAM_BLOCKED` | 502 |
| `BREAKER_OPEN` | 503 |
| `NETWORK` / `TIMEOUT` / `UPSTREAM_UNAVAILABLE` | 504 |
| `RATE_LIMITED` | 429 (with `Retry-After`) |

---

## RSS output details

- `description`: HTML wrapped in CDATA. It always carries the body text, images (`<img>` with `width`/`height` and `loading="lazy"`), repost quote blocks `<blockquote>`, and link cards (cover + title + summary + link).
- `content:encoded`: the same HTML as `description`, for readers that prefer the full-text namespace (Inoreader / Feedly / NetNewsWire).
- Media RSS: the first image as `<media:thumbnail>`, every image as `<media:content medium="image">`; Weibo videos additionally emit `<media:content medium="video">` plus `<enclosure type="video/mp4" length="0">`.
- `link` / `guid`: `https://weibo.com/<uid>/<bid>` for Weibo (falling back to `m.weibo.cn/detail/<id>` when bid is missing); the search URL for hot search; the original-site permalink for sspai/V2EX.
- `pubDate`: RFC822.
- Escaping covers `& < > " '`; a `]]>` inside CDATA is split safely and never closes the section early.
- The channel `<title>` / `<link>` / `<description>` are always present (required by RSS 2.0).

### Known limitations

- Weibo video direct links are **signed URLs that expire in about an hour** (`Expires=`); use `page_url` for long-term archiving. The direct link requires `Referer: https://weibo.com/`, and players that omit that header get a 403.
- sspai `full=1` depends on the upstream detail endpoint and takes 3–20 seconds per article; member-only articles return the summary only ("Member-only article; summary shown.").
- V2EX hot comes from page parsing, so it relies on the official API as a fallback whenever the structure changes.

---

## Cache behavior (API view)

| Platform | Cache key | TTL |
|---|---|---|
| Weibo timeline | `timeline:<uid>` | 5 minutes |
| Weibo profile | `profile:<uid>` | 1 hour |
| Weibo long post | `longtext:<uid>:<id>` | 24 hours |
| Weibo domain | `domain:<name>` | 24 hours |
| Weibo hot search | `hot:top` | 5 minutes |
| sspai list | `sspai:list` | 10 minutes |
| sspai article body | `sspai:detail:<id>` | 24 hours |
| V2EX latest/hot | `v2ex:latest` / `v2ex:hot` | 10 minutes |
| SMZDM deals | `smzdm:deals` | 5 minutes |
| X timeline | `x:<handle>` | 40 minutes |

- `refresh=1` skips reading the stale value and overwrites the cache on write-back; to keep the cache width it reads the width of the old entry. Both the RSS and the JSON endpoints support it.
- Cache entries record their width from the `limit` of the request, and neither a normal request nor `refresh=1` truncates a wider cache with a smaller `limit` (the entry is widened by fetching upstream as needed).
- Cache directory: `--cache-dir` / `FEEDMESH_CACHE`, default `~/.cache/feedmesh`; one JSON file per key, written atomically. The directory mode is 0700 and the file mode 0600; expired entries are cleaned at startup. `cache purge --all` deletes only entries it can verify were written by feedmesh (it checks the `key` inside the entry against the file name) and leaves other `.json` files in the same directory alone.

Implementation details (concurrency, circuit breaking, retries) are in [ARCHITECTURE.md](ARCHITECTURE.md).

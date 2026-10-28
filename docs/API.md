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
| `ref` | `/api/v1/youtube` only: channel id, `@handle` or channel URL — `/@name`, `/channel/UC…`, `/c/…`, `/user/…` (the RSS route takes only a single path segment, so URL forms belong here) |

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

### `GET /rss/smzdm`, `/rss/smzdm/post`, `/rss/smzdm/haitao`

The three official SMZDM RSS channels that answer anonymously: selected deals
(default), community articles and overseas deals.

```bash
curl -s 'http://127.0.0.1:3000/rss/smzdm?limit=20'
curl -s 'http://127.0.0.1:3000/rss/smzdm/post?limit=10'
curl -s 'http://127.0.0.1:3000/rss/smzdm/haitao?limit=10&refresh=1'   # bypass the cache
```

- 20 items by default, maximum 30 (one upstream slice). Cache: 5 minutes for the deal channels, 10 minutes for articles; `Cache-Control: max-age` follows the same TTL.
- `guid`/`link` is the SMZDM permalink: `/p/<id>/` for deals, `/p/<slug>/` for articles.
- Deal channels: the cover (`focus_pic`) goes into the body and Media RSS, and the blurb is published when `content:encoded` carries one.
- Article channel: the writer comes from `<author>` and the blurb from `<description>`; the feed has no cover.
- `pubDate` is Beijing time when the feed omits the zone, and is converted to UTC in the JSON view.
- The deal slices rotate, so items that leave them disappear from the feed; readers that keep history collect a wider set.
- `faxian` and `news` are WAF-protected and not exposed.

### `GET /rss/smzdm/fenlei/{slug}`

The good-price cards of one SMZDM category page (`https://www.smzdm.com/fenlei/{slug}/`),
which is the only public source of deals outside the curated RSS slices. `{slug}` is
the category path segment, for example `tixuhuli` (shaving) or `tixuchumao`.

```bash
curl -s 'http://127.0.0.1:3000/rss/smzdm/fenlei/tixuhuli?limit=20'
curl -s 'http://127.0.0.1:3000/rss/smzdm/fenlei/tixuhuli.xml'
```

- 20 items by default, maximum 30; one page holds 16–20 cards, so a larger `limit` returns what the page has.
- **One upstream read per slug per hour.** The cache lifetime is exactly that budget (60 minutes) and `Cache-Control: max-age=3600` follows it. `refresh=1` inside the window is served from the cache instead of reading the page again: at most one page read per slug per hour is a hard constraint, not a target.
- Reads are serialised across slugs: the service keeps one category page request in flight at a time.
- Only good-price cards are kept. The page also carries article, review and repost cards; those (and any card without a readable price) are dropped.
- The page publishes no timestamp for a good-price card, so `pubDate` falls back to the read time. `fetched_at` is that same instant: reads served from the cache keep the time the page was read instead of advancing to the request time, so a polling reader sees a stable timestamp for the batch.
- A failed read (HTTP error, transport error, or a large page with no cards) costs one upstream request and spends the rest of the hour: further calls in the window return `UPSTREAM_UNAVAILABLE` (504) with the retry time in the message instead of reading the page again. The marker is one budget window, not a cooldown.
- A challenge answer (HTTP 202, `TCaptcha`, or an empty shell under 20 KB with no cards) is reported as `UPSTREAM_BLOCKED` (502), and the slug is skipped for six hours (`BREAKER_OPEN`, 503). Nothing is retried, and no WAF challenge is ever solved.
- Category slugs must be 2–32 lowercase letters or digits, and the three paths that `robots.txt` disallows (`3cjiadian`, `jujiashenghuo`, `shishangyundong`) are refused with `BAD_REQUEST`.
- A cold read takes tens of seconds (the page is 600–720 KB): expect 8–50 s for the first request after the cache expires, then milliseconds.

### `GET /rss/x/{handle}`

X (Twitter) account timeline, fetched through public Nitter instances; `{handle}` may include `@`.

```bash
curl -s 'http://127.0.0.1:3000/rss/x/elonmusk?limit=20'
```

- 20 items by default, maximum 20 (one page); cached for 40 minutes, `Cache-Control: max-age=2400`.
- Item `link`/`guid` is always `https://x.com/{handle}/status/{id}`; instance links inside the body are rewritten too.
- Repost titles are prefixed with `🔁`; images go through Media RSS.
- When every instance fails, returns `UPSTREAM_UNAVAILABLE` (504).

### `GET /rss/youtube/{ref}`

A channel's upload feed, read from YouTube's official Atom endpoint; no API key and no account. `{ref}` is a channel id (`UC…`) or a handle (`@name`, `@` optional) — a channel URL has slashes and cannot travel in a path segment, so pass that form as `ref=` to the JSON endpoint instead.

```bash
curl -s 'http://127.0.0.1:3000/rss/youtube/@GoogleDevelopers?limit=15'
curl -s 'http://127.0.0.1:3000/rss/youtube/UC_x5XG1OV2P6uZZ5FSM9Ttw.xml'
```

- 15 items by default, maximum 15: the upstream feed carries 15 entries and cannot be paginated, so a channel that uploads more between two polls tops out.
- Cached for 10 minutes, `Cache-Control: max-age=600`; a degraded document is sent with `max-age=60` so readers retry soon after the upstream recovers.
- `X-Feedmesh-Source` says which upstream answered: `atom` (official feed), `browse` (the web-client fallback) or `stale` (the last good cached list). It is how an outage window shows up in a reader or a proxy log.
- `link`/`guid` is always `https://www.youtube.com/watch?v=<videoId>`, including for entries upstream publishes as Shorts.
- The cover is the upstream thumbnail (`i4.ytimg.com` on the Atom path, `i.ytimg.com` on the fallback); the body carries the description, a compact view count and a link back.
- The channel page is read once to turn a handle into a channel id and the mapping is cached for 24 hours; a handle and every spelling of it (`@name`, `name`, a pasted `/@name` URL) share one mapping, while `/c/…` and `/user/…` URLs cost one lookup of their own (the same name can point at a different channel there). An expired mapping is reused when the page cannot be read, so an outage does not blank a channel whose list is cached.
- A feed read is retried three times (2 s, then 4 s apart), and the 30-second budget covers the whole lookup — channel page and feed, retries and the fallback included — so a hanging endpoint cannot hold a request for minutes. If the Atom endpoint keeps failing, the service reads the same list from YouTube's web client endpoint instead: up to 30 entries, relative timestamps ("5 days ago") and no description, so those items carry approximate dates and repeat the title as their text. A cold cache during an outage therefore still answers; the first read takes a few seconds longer because the retries run first.
- If both sources fail, the last good list (up to 24 hours old) is served with `X-Feedmesh-Degraded: 1` and a normal 200. With no cached list at all the request returns `UPSTREAM_UNAVAILABLE` (504).
- A successful read that comes back with no entries keeps the list already cached (served as degraded) instead of replacing it; a channel with no uploads and no cache still gets an empty feed.
- A reference that does not resolve to a channel returns `USER_NOT_FOUND` (404); one that cannot name a channel at all (empty, malformed, an unsupported URL form) returns `BAD_REQUEST` (400).

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

### `GET /api/v1/smzdm?channel=deals|post|haitao&limit=N`

`channel` defaults to `deals`; `post` is community articles and `haitao` is overseas deals. Any other value returns `BAD_REQUEST`.

```json
{"channel":"deals","title":"SMZDM Deals","description":"Selected deals from SMZDM","fetched_at":"2026-10-25T14:12:08Z","cached":false,
 "items":[{"id":"184734707","title":"Brand model 620ml 28.84 CNY (buy 2, coupon)","author":"chromes","text":"",
           "url":"https://www.smzdm.com/p/184734707/","html":"…",
           "images":["https://y.zdmimg.com/…png_a200.jpg"],"created_at":"2026-10-25T13:39:37Z"}]}
```

- `limit` defaults to 20 with a maximum of 30; `refresh=1` behaves as on the RSS routes.
- `author` is the poster's nickname on the deal channels and the writer on the article channel. It is always present; it is empty when the feed carries no nickname, or when the nickname cannot be separated from the blurb (the deal channels pack both into one `<description>`). Callers never have to parse `html` for `Posted by …`.
- `text` is empty for the common deal case (nickname-only entries); the price and conditions live in `title`. Article entries carry the blurb in `text`.
- `price` is `null` on these channels: the RSS slices carry no machine-readable amount.

### `GET /api/v1/smzdm?channel=fenlei&slug=<slug>&limit=N`

One SMZDM category page. `slug` is required for this channel (`BAD_REQUEST` without it); `limit` defaults to 20 with a maximum of 30.

```json
{"channel":"fenlei","slug":"tixuhuli","title":"SMZDM Deals: tixuhuli",
 "description":"Good-price cards from the SMZDM category tixuhuli",
 "fetched_at":"2026-10-28T06:51:57Z","cached":false,
 "items":[{"id":"184919040","title":"Gillette classic razor 1 handle 3 blades","author":"",
           "text":"7.9 CNY (coupon)","url":"https://www.smzdm.com/p/184919040/","html":"…",
           "images":["https://y.zdmimg.com/…jpg_a100.jpg"],"created_at":"2026-10-28T06:51:57Z",
           "price":7.9,"slug":"tixuhuli","date_source":"fetched"}]}
```

- `price` is the leading amount of the card's display price as a number (`7.9`), or `null` when the card carries none. `text` keeps the full display string (`7.9 CNY (coupon)`), so the conditions survive.
- `slug` repeats the category on every item and on the envelope, which makes a mixed feed of several categories self-describing.
- `author` is always empty: category cards carry no poster.
- `created_at` is the page read time (the page publishes no per-card timestamp for a good price), so every item of one read shares the same instant and `fetched_at` repeats it. A cached read keeps that instant — it does not advance to the request time — so a polling reader can tell one batch from the next. It is always UTC. `date_source` says which of the two it is: `fetched` (read time standing in for a missing timestamp) or `card` (the card published its own). The field is absent on the RSS channels.
- One upstream read per slug per hour, `refresh=1` inside the window served from the cache, the six-hour slug cooldown on a challenge, and a failed read spending the rest of the hour: the same rules as the RSS route above.

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

### `GET /api/v1/youtube?ref=<channel>&limit=N`

```json
{"ref":"@GoogleDevelopers","channel_id":"UC_x5XG1OV2P6uZZ5FSM9Ttw","title":"Google for Developers",
 "description":"Latest videos from Google for Developers","fetched_at":"…","cached":false,"degraded":false,
 "source":"atom",
 "items":[{"id":"vid00000001","title":"…","text":"…","url":"https://www.youtube.com/watch?v=vid00000001",
           "html":"…","images":["https://i4.ytimg.com/vi/vid00000001/hqdefault.jpg"],
           "views":9862,"created_at":"2026-10-21T23:00:08Z"}]}
```

- `ref` is required and accepts a channel id, an `@handle`, or a channel URL — `/@name`, `/channel/UC…`, and the legacy `/c/…` and `/user/…` forms (YouTube serves them directly or redirects to the canonical channel page); `limit` defaults to 15 with a maximum of 15.
- `refresh=1` is supported and behaves the same as on the RSS routes.
- `source` is `atom`, `browse` or `stale`, the same value as the RSS route's `X-Feedmesh-Source` header.
- `views` is the play counter as an integer: exact on the Atom path, rounded on the fallback (`32K views` becomes `32000`).
- `degraded: true` means the list came from the last good cache entry because the upstream read failed; `cached` tells whether the TTL was still valid.
- `created_at` is exact on the Atom path; fallback items carry a day-granular approximation, so several uploads can share one timestamp and their order follows the upstream list.

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
| SMZDM articles | `smzdm:post` | 10 minutes |
| SMZDM overseas | `smzdm:haitao` | 5 minutes |
| SMZDM category page | `smzdm:fenlei:<slug>` | 60 minutes (also the per-slug upstream budget) |
| SMZDM category failure marker | `smzdm:fenlei:<slug>:retry` | 60 minutes: the rest of the budget window after a failed read |
| X timeline | `x:<handle>` | 40 minutes |
| YouTube uploads | `youtube:feed:<channel_id>` | 10 minutes (last good kept for 24 hours) |
| YouTube handle mapping | `youtube:channel:<ref>` | 24 hours |

- `refresh=1` skips reading the stale value and overwrites the cache on write-back; to keep the cache width it reads the width of the old entry. Both the RSS and the JSON endpoints support it.
- Cache entries record their width from the `limit` of the request, and neither a normal request nor `refresh=1` truncates a wider cache with a smaller `limit` (the entry is widened by fetching upstream as needed).
- Cache directory: `--cache-dir` / `FEEDMESH_CACHE`, default `~/.cache/feedmesh`; one JSON file per key, written atomically. The directory mode is 0700 and the file mode 0600; expired entries are cleaned at startup. `cache purge --all` deletes only entries it can verify were written by feedmesh (it checks the `key` inside the entry against the file name) and leaves other `.json` files in the same directory alone.

Implementation details (concurrency, circuit breaking, retries) are in [ARCHITECTURE.md](ARCHITECTURE.md).

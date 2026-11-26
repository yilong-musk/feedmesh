# API

All endpoints are `GET`, and by default the service listens only on `127.0.0.1:3000`. The `Content-Type` is `application/rss+xml; charset=utf-8` for RSS responses and `application/json` for JSON responses.

With `--token <token>` or `FEEDMESH_TOKEN` set, every request must carry `Authorization: Bearer <token>`, otherwise it returns 401. With `--rate-limit <n>` or `FEEDMESH_RATE_LIMIT` set, each remote IP may make at most n requests within any rolling minute, and anything above that returns 429. If the listener is bound to a non-loopback address and no limit is configured, the service applies 120 requests/minute/IP by default. Behind a reverse proxy `RemoteAddr` is usually the proxy IP, so rate limiting at the proxy layer as well is recommended.

Responses are gzip-compressed when the request sends `Accept-Encoding: gzip` (every response carries `Vary: Accept-Encoding`). RSS responses carry a weak `ETag`; a request whose `If-None-Match` matches it is answered with `304 Not Modified` and no body, so a reader whose copy is current downloads nothing. The validator ignores `<lastBuildDate>`, which records the read time. The channel `<language>` element is only present when the provider's content language is known (`zh-cn` for the Chinese platforms).

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
| `sub` | `/api/v1/reddit` only: subreddit name (`r/` prefix and case accepted) |
| `sort` | Reddit only: `hot` (default), `new` or `top` |
| `window` | Reddit only, `sort=top` only: `day` (default), `week`, `month`, `year` or `all` |

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

### `GET /rss/douyin/{sec_uid}`

A creator's video timeline, read from the public profile page through a headless Chromium: Douyin signs every API call with a browser-generated token, so the request is issued from inside the page instead of being reimplemented. No account is needed; the browser uses a visitor identity stored under the cache directory.

```bash
curl -s 'http://127.0.0.1:3000/rss/douyin/MS4wLjABAAAAlVAfjJp4JQqjNIFKLDwWDHjjcS11yIMJ2a1kRhLiAD8?limit=20'
curl -s 'http://127.0.0.1:3000/rss/douyin/<sec_uid>.xml'
```

- 20 items by default, maximum 44 (one page). Pinned videos are marked but ordered by publish time like every other item.
- Cached for 10 minutes, `Cache-Control: max-age=600`; the last good list is kept for six hours and served with `X-Feedmesh-Degraded: 1` when the upstream read fails.
- A cold read starts Chromium and waits for the page's signer (about 15 seconds), so the first request after the cache expires can take half a minute. The browser is closed after two minutes without use.
- `link`/`guid` is `https://www.douyin.com/video/<aweme_id>`; the body carries the caption, the cover image and the duration/resolution line. The item `title` is the whole caption, not a shortened label. The play address expires within hours, so it is not embedded in the feed.
- `limit` above 44 returns 44; only the first page is available without a logged-in cookie.

### `GET /rss/tiktok/{handle}`

A creator's video timeline, read from the public profile page plus the web client's creator endpoint. No account or cookie is needed: the profile page sits behind a Slardar WAF proof-of-work puzzle, and the provider solves it with the standard library (SHA-256 search) instead of a browser.

```bash
curl -s 'http://127.0.0.1:3000/rss/tiktok/nasa?limit=10'
curl -s 'http://127.0.0.1:3000/rss/tiktok/nasa.xml'
```

- `{handle}` is the account name without `@`; a `secUid` (`MS4wLjABAAAA…`) also works.
- 20 items by default, maximum 45 (three upstream pages of 15). The upstream rejects any other page size.
- Cached for 10 minutes, `Cache-Control: max-age=600`; the last good list is kept for six hours and served with `X-Feedmesh-Degraded: 1` when the upstream read fails.
- `link`/`guid` is `https://www.tiktok.com/@{author}/video/<aweme_id>`; the body carries the caption, the cover image, duration/resolution, engagement counters and the music credit. The item `title` is the whole caption. Play addresses expire within hours, so they are not embedded in the feed.
- Private accounts and region-gated posts answer `USER_NOT_FOUND` (404); a malformed handle answers `BAD_REQUEST` (400); an unsolvable challenge answers `UPSTREAM_UNAVAILABLE` (504).

### `GET /rss/bilibili/{mid}`

A creator's uploads, read from the public space API. Bilibili requires a WBI signature; the provider computes it with the standard library (MD5 + a fixed permutation table), so no browser is involved.

```bash
curl -s 'http://127.0.0.1:3000/rss/bilibili/946974?limit=10'
curl -s 'http://127.0.0.1:3000/rss/bilibili/946974.xml'
```

- `{mid}` is the numeric account id from `space.bilibili.com/<mid>`.
- 20 items by default, maximum 50 (one upstream page). `refresh=1` forces a read.
- Cached for 10 minutes, `Cache-Control: max-age=600`; the last good list is kept for six hours and served with `X-Feedmesh-Degraded: 1` when the upstream read fails.
- `link`/`guid` is `https://www.bilibili.com/video/<bvid>`; the body carries the cover, description, duration, play/danmaku/comment counters and the category. The item `title` is the upload title.
- Bilibili rate-limits datacenter IPs: the provider retries a `412`/`-352` answer up to six times over about nine seconds. If every attempt is blocked the route returns `UPSTREAM_BLOCKED` (502) — retry later or use the cached copy.
- A missing account returns `USER_NOT_FOUND` (404); a non-numeric mid returns `BAD_REQUEST` (400).

### `GET /rss/telegram/{channel}`

A public Telegram channel, read from the `t.me/s/<channel>` web preview. No account, bot token or API key is involved; only channels that have the public preview enabled are readable.

```bash
curl -s 'http://127.0.0.1:3000/rss/telegram/sspai?limit=10'
curl -s 'http://127.0.0.1:3000/rss/telegram/sspai.xml'
```

- `{channel}` accepts `@name`, `name`, `t.me/name` and `https://t.me/s/name`; the name is case-insensitive.
- 20 items by default, maximum 20: one preview page carries about that many posts.
- Cached for 10 minutes, `Cache-Control: max-age=600`; the last good list is kept for six hours and served with `X-Feedmesh-Degraded: 1` when the upstream read fails.
- `link`/`guid` is `https://t.me/<channel>/<message_id>`; the body carries the post text as sanitised HTML, every photo of an album, a video still, a link card and a meta line with the author signature, forward source and view counter. The item `title` is the whole caption.
- Video posts expose the still frame only: the preview's `<video src>` is a signed URL that expires, so embedding it would leave a dead player behind. Media RSS repeats the photos.
- A missing channel, or one whose preview is disabled (private channels and groups), returns `USER_NOT_FOUND` (404); a malformed name returns `BAD_REQUEST` (400); a preview with no visible post returns `UPSTREAM_UNAVAILABLE` (504) instead of caching an empty feed; a `429` from t.me returns `RATE_LIMITED`.

### `GET /rss/pinterest/{user}`, `/rss/pinterest/{user}/{board}`

Pinterest profile or board pins, read from Pinterest's public RSS feeds. No account, cookie, token or browser is involved: both the profile feed (`/<user>/feed.rss`) and a board feed (`/<user>/<board>.rss`) are served anonymously.

```bash
curl -s 'http://127.0.0.1:3000/rss/pinterest/nasa?limit=10'
curl -s 'http://127.0.0.1:3000/rss/pinterest/nasa/earth'
```

- `{user}` accepts a bare profile name, `@handle` or a profile URL; `{board}` is the board slug from the board URL (`https://www.pinterest.com/<user>/<board>/`). Pinterest treats both case-insensitively, and the cache key lowercases them.
- One feed slice carries at most 25 pins and has no pagination, so 25 is both the default and the maximum. `refresh=1` forces a read.
- Cached for 10 minutes, `Cache-Control: max-age=600`; the last good result is kept for six hours and served with `X-Feedmesh-Degraded: 1` when the upstream read fails.
- The feed keeps Pinterest's own order: a pin added to a board today can carry an old creation date, and re-sorting by `pubDate` would bury it.
- `link`/`guid` is the pin permalink (`https://www.pinterest.com/pin/<id>/`; Pinterest serves both numeric and opaque ids). The item `title` is the pin title, falling back to the caption and then `(no content)`; the body carries the 736px thumbnail (upgraded from the 236px URL in the feed), the caption and a link to the pin page.
- A missing profile returns `USER_NOT_FOUND` (404) with `User not found`; a missing board returns `USER_NOT_FOUND` (404) with `Board not found`; a malformed name returns `BAD_REQUEST` (400); a `429` returns `RATE_LIMITED` (429).
- Pinterest's `robots.txt` disallows generic crawlers (`User-agent: *` → `Disallow: /`) while the RSS endpoints themselves are public and recommended to feed readers by third-party documentation. Operators should decide whether that fits their use.

### `GET /rss/polymarket/new`, `/rss/polymarket/hot`, `/rss/polymarket/event/{slug}`, `/rss/polymarket/user/{address}`

Prediction-market feeds read from Polymarket's public APIs. No account, API key, cookie or browser is involved: market data comes from the Gamma API and wallet activity from Data API v2.

```bash
curl -s 'http://127.0.0.1:3000/rss/polymarket/new?limit=10'
curl -s 'http://127.0.0.1:3000/rss/polymarket/hot?tag=politics'
curl -s 'http://127.0.0.1:3000/rss/polymarket/event/democratic-presidential-nominee-2028'
curl -s 'http://127.0.0.1:3000/rss/polymarket/user/0x7c3db723f1d4d8cb9c550095203b686cb11e5c6b'
```

- `new` lists the newest events (`order=createdAt`), `hot` ranks events by 24-hour volume, `event/{slug}` lists one event's open markets by volume, and `user/{address}` lists a wallet's newest actions.
- `new`/`hot` accept `tag=<slug>` (e.g. `politics`, `sports`, `crypto`) and `limit` up to 50; `event` accepts `limit` up to 50; `user` accepts `limit` up to 100 (default 30). `refresh=1` forces a read.
- The newest-events list always sends `exclude_tag_id=102169` (Polymarket's own `Hide From New` tag). Without it, roughly 80% of the newest events are five-minute crypto `Up or Down` markets.
- Event items carry the title, link, cover image and a summary of the top markets with their current odds (`Yes 47.5% · No 52.5%`), plus 24-hour volume, liquidity and end date. Single-event items are one market each; wallet items are one trade or redemption each.
- Cache: event lists fresh for 10 minutes, single events and wallet activity for 5 minutes; the last good entry is kept for six hours. `Cache-Control` follows the same TTL.
- Odds are a snapshot. RSS readers do not re-notify an item whose content changes, so an event feed is a dashboard, not an alert; use the wallet feed or the newest-events feed when a new item should trigger a notification.
- A missing event returns `USER_NOT_FOUND` (404); a malformed tag, slug or address returns `BAD_REQUEST` (400); upstream `429` returns `RATE_LIMITED`.
- The Gamma API caps pages at 50 events without gzip; the Go HTTP client negotiates gzip automatically, so `limit=50` works and one page stays around 100–150 KB after the summary trim.

### `GET /rss/linuxdo`, `/rss/linuxdo/top`, `/rss/linuxdo/hot`, `/rss/linuxdo/c/{slug}/{id}`, `/rss/linuxdo/tag/{tag}`

LINUX DO forum topics, read from the official Discourse RSS feeds. No account or API key is involved. Only `.rss` paths are requested: the forum's JSON and HTML paths answer a Cloudflare challenge that would also take RSS down for several minutes.

```bash
curl -s 'http://127.0.0.1:3000/rss/linuxdo?limit=5'
curl -s 'http://127.0.0.1:3000/rss/linuxdo/top?period=weekly&limit=10'
curl -s 'http://127.0.0.1:3000/rss/linuxdo/hot'
curl -s 'http://127.0.0.1:3000/rss/linuxdo/c/develop/4'
curl -s 'http://127.0.0.1:3000/rss/linuxdo/tag/chatgpt'
```

- `latest` defaults to `order=created` (stable creation time); `order=activity` follows the last-reply order. `top` takes `period=daily|weekly|monthly|quarterly|yearly|all` and defaults to `daily`. `hot` takes no parameters.
- A category accepts `{slug}/{id}` from the forum URL (`develop/4`) or the id alone (`/rss/linuxdo/c/4`), which follows the forum's redirect to the canonical slug. A tag takes the tag name.
- 30 items by default, maximum 90 (three RSS pages; category pages carry 25). `refresh=1` forces a read.
- Cached for 15 minutes, `Cache-Control: max-age=900`; the last good list is kept for 24 hours and served with `X-Feedmesh-Degraded: 1` when a read is challenged.
- `link`/`guid` is `https://linux.do/t/topic/<id>`; the body carries the first post as sanitised HTML, the category, author, pinned/closed/archived state and Media RSS images. The item `title` is the topic title. Discourse's reply count and "read the full topic" footer stay in the body; the feed does not expose reply or view counts as fields.
- A malformed reference returns `BAD_REQUEST` (400); a missing category or tag returns `USER_NOT_FOUND` (404); a Cloudflare challenge returns `UPSTREAM_BLOCKED` (502). A challenge pauses every LINUX DO feed for ten minutes instead of retrying.

### `GET /rss/reddit/{sub}`

A subreddit listing, read from the public Atom document (`/r/<sub>/<sort>/.rss`); no account, no OAuth application and no API key. `{sub}` is the subreddit name (`r/` prefix and case accepted); `/rss/reddit/r/{sub}` accepts the name exactly as it is copied from the site, and `.xml` is optional.

```bash
curl -s 'http://127.0.0.1:3000/rss/reddit/programming?limit=5'
curl -s 'http://127.0.0.1:3000/rss/reddit/programming?sort=top&window=week'
curl -s 'http://127.0.0.1:3000/rss/reddit/r/rust.xml'
```

- Listing: `sort` is `hot` (default), `new` or `top`; `window` (`day` by default, `week`, `month`, `year`, `all`) applies to `top` only and is ignored otherwise. Each `(sub, sort, window)` triple is a cache key of its own.
- 20 items by default, maximum 25: one Atom page carries at most 25 entries and the document has no pagination.
- Cached for 10 minutes (`Cache-Control: max-age=600`); a degraded document is sent with `max-age=60`.
- `X-Feedmesh-Source` is `atom` for a real read and `stale` when the last good listing answered; a degraded answer also carries `X-Feedmesh-Degraded: 1`.
- Entry order is the order the document carries: `hot` is a ranking (not a time series), and `new`/`top` are already ordered upstream.
- `link`/`guid` is the submission's comment page (`https://www.reddit.com/r/<sub>/comments/<id>/<slug>/`); the outbound link of a link submission travels in the item body and in the JSON `text` field.
- The document carries no score, comment count, thumbnail or self text. `images` is therefore always empty, and `text` is the outbound link (empty for a self post).
- Channel `title` is `r/<sub> <sort>` (plus ` (<window>)` for `top`), so a reader subscribed to both `hot` and `top` can tell them apart; `description` is the subreddit's public description.
- **Pacing**: one global read slot is shared by every subreddit and sort. At most one upstream request is in flight, two reads are at least 8 seconds apart, and after a reply that reports its quota spent (`x-ratelimit-remaining: 0`) no read is made until that reply's `x-ratelimit-reset` has passed — the measured window on a datacenter address is one read per ~60 seconds. A request that arrives while the slot is busy is answered from the last good listing (degraded) or fails with `UPSTREAM_UNAVAILABLE`; it is never queued behind another read, because a reader would time out first.
- A 429 reply is never retried: its reset header closes the window for every subreddit at once, so one refusal costs exactly one request.
- Read failures and limiter refusals degrade to the last good listing (up to 24 hours old) with `X-Feedmesh-Degraded: 1` and `X-Feedmesh-Source: stale`; with no cached listing the route returns `UPSTREAM_UNAVAILABLE` (504).
- A successful read that comes back with no entries keeps the cached listing instead of blanking the feed.
- An unknown subreddit returns `USER_NOT_FOUND` (404): the site answers such a name with a redirect to its subreddit search rather than a 404, and the client reads that redirect instead of following it (following it would spend a second request in the same quota window). A name that cannot be a subreddit at all (empty, spaces, slashes) returns `BAD_REQUEST` (400).
- Every upstream read is logged (`reddit upstream read`) with the subreddit, sort, status and duration, so the pacing above can be audited from the journal.

---

## JSON endpoints

Every `/api/v1/*` feed endpoint (except the two downloader-specific `/timeline` adapters below) returns the same envelope and item shape. Timestamps are RFC 3339 (UTC); `items` and `images` are always arrays (`[]` when empty, never `null`). Degraded responses carry both `"degraded": true` and the `X-Feedmesh-Degraded: 1` header; providers with multiple upstream paths also set `"source"` and `X-Feedmesh-Source`.

### Unified envelope

```json
{
  "provider": "x",
  "title": "4KAestheticHQ / X",
  "description": "@4KAestheticHQ latest posts (via Nitter instances)",
  "fetched_at": "2026-11-04T06:50:00Z",
  "cached": false,
  "degraded": false,
  "source": "atom",
  "meta": {"handle": "4kaesthetichq"},
  "items": [
    {
      "id": "2117824259859034220",
      "title": "…",
      "author": "4KAestheticHQ",
      "text": "…",
      "html": "…",
      "url": "https://x.com/4KAestheticHQ/status/2117824259859034220",
      "images": ["https://pbs.twimg.com/media/HWQF3-EWQAEkvBh.jpg?name=orig"],
      "video": {
        "url": "https://video.twimg.com/…/vid/avc1/720x1558/….mp4",
        "thumbnail": "https://pbs.twimg.com/amplify_video_thumb/…jpg",
        "duration_ms": 4308,
        "views": 1234
      },
      "created_at": "2026-11-04T06:49:32Z",
      "extra": {
        "author_id": "1507222724757925889",
        "card_image": "https://…",
        "likes": 8,
        "replies": 0,
        "quoted": {
          "id": "2117127236843184565",
          "url": "https://x.com/…/status/2117127236843184565",
          "author": "4KAestheticHQ",
          "author_name": "4K Wallpapers | Aesthetic HQ",
          "text": "…",
          "images": ["https://pbs.twimg.com/media/…jpg?name=orig"]
        }
      }
    }
  ]
}
```

- `source` and `meta` are omitted when unused.
- On items, `author`, `video` and `extra` are omitted when empty or absent.
- `feedmesh fetch <uid> --json` uses the same shape plus `"ok": true` and `"count"`.

### Per-endpoint `provider`, `meta` and `extra`

| Route | `provider` | `meta` | Item `video` / `extra` |
|---|---|---|---|
| `GET /api/v1/feed?uid=<uid>&limit=N` | `weibo` | `{"uid","screen_name"}` | `video.url`; `extra: {"bid","page_url"}` |
| `GET /api/v1/hot?limit=N` | `hot` | — | `extra: {"rank","heat"}` |
| `GET /api/v1/sspai?limit=N&full=1` | `sspai` | — | — |
| `GET /api/v1/v2ex?section=latest\|hot&limit=N` | `v2ex` | `{"section"}` | — |
| `GET /api/v1/smzdm?channel=deals\|post\|haitao&limit=N` | `smzdm` | `{"channel"}` | — (`author` carries the poster/writer when present) |
| `GET /api/v1/smzdm?channel=fenlei&slug=<slug>&limit=N` | `smzdm` | `{"channel":"fenlei","slug"}` | `extra: {"price","slug","date_source"}` (`price` is a number or `null`; `date_source` is `fetched` or `card`) |
| `GET /api/v1/x?handle=<handle>&limit=N` | `x` | `{"handle"}` | `video: {"url","thumbnail","duration_ms","views"}`; `extra: {"author_id","card_image","likes","replies","quoted"}` |
| `GET /api/v1/youtube?ref=<channel>&limit=N` | `youtube` | `{"ref","channel_id"}` | `source`: `atom` \| `browse` \| `stale`; `extra: {"views"}` |
| `GET /api/v1/douyin?user=<sec_uid>&limit=N` | `douyin` | `{"user"}` | — |
| `GET /api/v1/tiktok?user=<handle>&limit=N` | `tiktok` | `{"user"}` | — |
| `GET /api/v1/bilibili?user=<mid>&limit=N` | `bilibili` | `{"user"}` | `extra: {"views"}` |
| `GET /api/v1/telegram?channel=<channel>&limit=N` | `telegram` | `{"channel"}` | `video.thumbnail` (on video posts); `extra: {"views"}` |
| `GET /api/v1/pinterest?user=<user>&board=<board>&limit=N` | `pinterest` | `{"user","board"?}` | — |
| `GET /api/v1/polymarket?feed=new\|hot\|event\|user` | `polymarket` | `{"feed","tag"?,"slug"?,"address"?}` | `extra: {"volume_24h","liquidity","ends_at"}` (omitted on wallet `user` activity) |
| `GET /api/v1/linuxdo?feed=<kind>&limit=N` | `linuxdo` | `{"feed"}` | `extra: {"category","pinned","closed","archived"}` |
| `GET /api/v1/reddit?sub=<sub>&sort=<sort>&window=<window>&limit=N` | `reddit` | `{"sub","sort","window"}` | `source`: `atom` \| `stale` |

### Provider-specific notes

- **SMZDM**: `channel` defaults to `deals`; `post` is community articles, `haitao` is overseas deals, `fenlei` requires `slug`. `limit` defaults to 20 (max 30). For `fenlei`, `created_at` is the page read time (shared across the cached hour) and `extra.date_source` reports `fetched` or `card`; one upstream read per slug per hour applies.
- **X**: `handle` is required (`@` allowed); `limit` defaults to 20 (max 20). `images` holds only the post's own photos (`?name=orig`); video posters live in `video.thumbnail`, link-card banners in `extra.card_image`, and quoted-post media inside `extra.quoted`. Enrichment (`video.url`, `duration_ms`, `views`, `likes`, `replies`) costs one embed request per post (at most 4 concurrent, 6 s each, 15 s per page) and can be disabled with `--no-x-enrich` / `FEEDMESH_X_ENRICH=0`.
- **YouTube**: `ref` accepts a channel id, `@handle`, or channel URL (`/channel/UC…`, `/@name`, `/c/…`, `/user/…`); `limit` defaults to 15 (max 15). `extra.views` is exact on `atom` and rounded on `browse`.
- **Polymarket**: `feed` defaults to `new`; `tag=` filters `new`/`hot`, `slug=` selects an `event`, `address=` selects wallet `user` activity.
- **LINUX DO**: `feed` defaults to `latest` (`order=created|activity`), `top` (`period=`), `hot`, `category` (`category=<slug>/<id>` or `<id>`), or `tag` (`tag=`); `limit` defaults to 30 (max 90). `meta.feed` echoes the canonical key (e.g. `latest:created`).
- **Reddit**: `sub` accepts names with or without `r/`; `sort` defaults to `hot`, `window` to `day`, `limit` to 20 (max 25). `author` drops the `/u/` prefix; `text` is the outbound link for link posts and empty for self posts.

### Downloader timeline endpoints (dedicated schema)

These two endpoints keep the fixed downloader schema (`aweme_id`, Unix `create_time`, `play_url`, `cover_url`) so an external download tool can consume them without touching a browser or solving a WAF challenge.

#### `GET /api/v1/douyin/timeline?user=<sec_uid>&limit=N`

```json
{"author":"Creator","sec_uid":"MS4w…","fetched_at":"…","source":"douyin",
 "items":[{"aweme_id":"7706127120845719717","desc":"…","author":"Creator","sec_uid":"MS4w…",
           "create_time":1794222538,"duration_ms":93202,"width":1080,"height":1920,
           "is_top":0,"play_url":"https://v3-dy-o.zjcdn.com/…","cover_url":"https://p3-pic.douyinpic.com/…"}]}
```

- `play_url` is a time-limited, watermark-free MP4 address; download it promptly instead of storing it for later.

#### `GET /api/v1/tiktok/timeline?user=<handle>&limit=N`

```json
{"author":"NASA","user":"nasa","fetched_at":"…","source":"tiktok",
 "items":[{"aweme_id":"7708867568920038669","desc":"…","author":"nasa",
           "create_time":1794860611,"duration_ms":63000,"width":720,"height":1280,
           "is_top":0,"play_url":"https://v16-webapp-prime.tiktok.com/…","cover_url":"https://p16-common-sign.tiktokcdn.com/…"}]}
```

- `play_url` is a time-limited, watermark-free MP4 address (highest-bitrate rendition); `is_top` is `1` for a pinned video. Photo posts carry `cover_url` with an empty `play_url`.

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
| Weibo hot search | `hot:top` | 5 minutes (last good kept for 1 hour) |
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
| `douyin:user:<sec_uid>` | 10 minutes (last good kept for 6 hours) |
| `tiktok:user:<handle>` | 10 minutes (last good kept for 6 hours) |
| `bilibili:user:<mid>` | 10 minutes (last good kept for 6 hours) |
| Reddit listing | `reddit:list:<sub>:<sort>:<window>` | 10 minutes (last good kept for 24 hours) |
| Telegram channel | `telegram:channel:<name>` | 10 minutes (last good kept for 6 hours) |
| LINUX DO feed | `linuxdo:feed:<kind>:<params>` | 15 minutes (last good kept for 24 hours) |
| Pinterest profile / board | `pinterest:user:<user>` / `pinterest:board:<user>/<board>` | 10 minutes (last good kept for 6 hours) |
| Polymarket event list | `polymarket:events:<order>:<tag>` | 10 minutes (last good kept for 6 hours) |
| Polymarket single event | `polymarket:event:<slug>` | 5 minutes (last good kept for 6 hours) |
| Polymarket wallet activity | `polymarket:activity:<address>` | 5 minutes (last good kept for 6 hours) |

- **Last good copy**: every entry is stored for at least 6 hours (a provider whose freshness TTL is longer keeps it longer; the hot-search ranking keeps its copy for one hour); the TTL above only decides when the entry stops counting as *fresh*. When an upstream read fails, the last stored value inside that window is served with a normal 200, `X-Feedmesh-Degraded: 1` and `Cache-Control: max-age=60`, so a reader keeps polling instead of marking the feed broken; the next successful read replaces it, and a key that just failed is not retried for 60 seconds (the last good copy is served straight away instead). Every JSON response carries a `degraded` field that is true when that copy is served. A list served this way may be narrower than the requested `limit` (it has whatever width the last good entry had). With nothing cached the route still returns `UPSTREAM_UNAVAILABLE` (504). This covers the Weibo timeline, profile, hot search, sspai, V2EX, SMZDM (including category pages), X, YouTube, Douyin, TikTok, Bilibili, Telegram, Pinterest, Polymarket, LINUX DO and Reddit.
- `refresh=1` skips reading the stale value and overwrites the cache on write-back; to keep the cache width it reads the width of the old entry. Both the RSS and the JSON endpoints support it.
- Cache entries record their width from the `limit` of the request, and neither a normal request nor `refresh=1` truncates a wider cache with a smaller `limit` (the entry is widened by fetching upstream as needed).
- Cache directory: `--cache-dir` / `FEEDMESH_CACHE`, default `~/.cache/feedmesh`; one JSON file per key, written atomically. The directory mode is 0700 and the file mode 0600; expired entries are cleaned at startup. `cache purge --all` deletes only entries it can verify were written by feedmesh (it checks the `key` inside the entry against the file name) and leaves other `.json` files in the same directory alone.

Implementation details (concurrency, circuit breaking, retries) are in [ARCHITECTURE.md](ARCHITECTURE.md).

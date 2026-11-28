# Supported platforms

**13 platforms and 19 content types** are currently wired up. Each platform below lists the content it supports and the matching endpoints; adding a platform only takes one provider — see the end of this page.

| Platform | Content | RSS | JSON |
|---|---|---|---|
| <img src="assets/weibo.png" width="20" height="20" align="absmiddle" alt="Weibo"> **Weibo** | Account timeline | `/rss/user/{uid}`<br>`/rss/domain/{name}` | `/api/v1/feed?uid=` |
| <img src="assets/weibo.png" width="20" height="20" align="absmiddle" alt="Weibo"> **Weibo** | Realtime hot search | `/rss/hot` | `/api/v1/hot` |
| <img src="assets/sspai.png" width="20" height="20" align="absmiddle" alt="sspai"> **sspai** | Latest articles | `/rss/sspai` | `/api/v1/sspai` |
| <img src="assets/v2ex.png" width="20" height="20" align="absmiddle" alt="V2EX"> **V2EX** | Latest topics | `/rss/v2ex` | `/api/v1/v2ex?section=latest` |
| <img src="assets/v2ex.png" width="20" height="20" align="absmiddle" alt="V2EX"> **V2EX** | Hot topics | `/rss/v2ex/hot` | `/api/v1/v2ex?section=hot` |
| <img src="assets/smzdm.png" width="20" height="20" align="absmiddle" alt="SMZDM"> **SMZDM** | Selected deals | `/rss/smzdm` | `/api/v1/smzdm` |
| <img src="assets/smzdm.png" width="20" height="20" align="absmiddle" alt="SMZDM"> **SMZDM** | Community articles | `/rss/smzdm/post` | `/api/v1/smzdm?channel=post` |
| <img src="assets/smzdm.png" width="20" height="20" align="absmiddle" alt="SMZDM"> **SMZDM** | Overseas deals | `/rss/smzdm/haitao` | `/api/v1/smzdm?channel=haitao` |
| <img src="assets/smzdm.png" width="20" height="20" align="absmiddle" alt="SMZDM"> **SMZDM** | Category good prices | `/rss/smzdm/fenlei/{slug}` | `/api/v1/smzdm?channel=fenlei&slug=` |
| <img src="assets/x.png" width="20" height="20" align="absmiddle" alt="X"> **X** | Account timeline | `/rss/x/{handle}` | `/api/v1/x?handle=` |
| <img src="assets/youtube.png" width="20" height="20" align="absmiddle" alt="YouTube"> **YouTube** | Channel uploads | `/rss/youtube/{ref}` | `/api/v1/youtube?ref=` |
| <img src="assets/reddit.png" width="20" height="20" align="absmiddle" alt="Reddit"> **Reddit** | Subreddit listing | `/rss/reddit/{sub}`<br>`/rss/reddit/r/{sub}` | `/api/v1/reddit?sub=` |
| <img src="assets/douyin.png" width="20" height="20" align="absmiddle" alt="Douyin"> **Douyin** | Creator videos | `/rss/douyin/{sec_uid}` | `/api/v1/douyin?user=` |
| <img src="assets/tiktok.png" width="20" height="20" align="absmiddle" alt="TikTok"> **TikTok** | Creator videos | `/rss/tiktok/{handle}` | `/api/v1/tiktok?user=` |
| <img src="assets/bilibili.png" width="20" height="20" align="absmiddle" alt="Bilibili"> **Bilibili** | Creator videos | `/rss/bilibili/{mid}` | `/api/v1/bilibili?user=` |
| <img src="assets/telegram.png" width="20" height="20" align="absmiddle" alt="Telegram"> **Telegram** | Public channel posts | `/rss/telegram/{channel}` | `/api/v1/telegram?channel=` |
| <img src="assets/linuxdo.png" width="20" height="20" align="absmiddle" alt="LINUX DO"> **LINUX DO** | Forum topics | `/rss/linuxdo`<br>`/rss/linuxdo/top`<br>`/rss/linuxdo/hot`<br>`/rss/linuxdo/c/{slug}/{id}`<br>`/rss/linuxdo/tag/{tag}` | `/api/v1/linuxdo?feed=` |
| <img src="assets/apple.png" width="20" height="20" align="absmiddle" alt="Apple Podcasts"> **Apple Podcasts** | Show episodes | `/rss/apple/{id}` | `/api/v1/apple?id=` |
| <img src="assets/apple.png" width="20" height="20" align="absmiddle" alt="Apple Podcasts"> **Apple Podcasts** | Top shows | `/rss/apple/top` | `/api/v1/apple?chart=top&country=` |

Common parameters: `limit`, `refresh=1` (skip the stale cache and force an upstream fetch, keeping the cache width); the `.xml` suffix on an RSS path is equivalent to no suffix. Full contract: [API.md](API.md).

---

## <img src="assets/weibo.png" width="28" height="28" align="absmiddle" alt="Weibo"> Weibo

Official site <https://weibo.com> · Data source `m.weibo.cn` (visitor cookie, anonymous)

| Content | RSS | JSON | Default | Max | Cache |
|---|---|---|---|---|---|
| Account timeline | `/rss/user/{uid}`, `/rss/domain/{name}` | `/api/v1/feed?uid=` | 10 | 10 | 5 minutes |
| Realtime hot search | `/rss/hot`, `/rss/hot.xml` | `/api/v1/hot` | 20 | 50 | 5 minutes |

### Account timeline

- Parameters: `limit` 1–10, `refresh=1`; `domain` is resolved to a uid first.
- Resilience: a timeline, profile or hot-search read that fails (timeout, 5xx, breaker open) serves the last good value for up to 6 hours (one hour for hot search) with `X-Feedmesh-Degraded: 1` and `max-age=60`; a profile that cannot be read does not fail the feed — the cached screen name is used and the document is marked degraded. Nothing cached still returns 504.
- Output: post body HTML, images (large size with real width and height), repost quote block, full text of long posts, video enclosure, external link cards.
- Upstream: `container/getIndex`, at most 10 posts per page; long posts additionally go through `statuses/extend` (cached for 24 hours).

### Realtime hot search

- Rules: keep only the main ranking with `realpos` 1–50; drop the "Recommended" slots that carry no `realpos`; the pinned `hotgov` entry is not merged in.
- Ranking uses `realpos` rather than the array index, so recommended slots cannot shift the positions.
- `guid` uses the search URL, so a rank change does not make readers push the item again.
- Resilience: a failed read serves the last good ranking for **one hour** only (`X-Feedmesh-Degraded: 1`, `max-age=60`, `"degraded": true` in JSON). A ranking older than that is not served at all — it would still be labelled realtime while being hours out of date — so the route answers 504 until upstream returns.

### Limitations

- Video direct links are **signed URLs that expire in about 1 hour** and require `Referer: https://weibo.com/`; use `page_url` for long-term archiving.
- The timeline endpoint returns at most 10 posts per page; `limit` cannot page beyond that.

---

## <img src="assets/sspai.png" width="28" height="28" align="absmiddle" alt="sspai"> sspai

Official site <https://sspai.com> · Data source: sspai.com JSON API (the official RSS only carries summaries)

| Content | RSS | JSON | Default | Max | Cache |
|---|---|---|---|---|---|
| Latest articles | `/rss/sspai`, `/rss/sspai.xml` | `/api/v1/sspai` | 10 | 30 | list 10 minutes · body 24 hours |

- Parameters: `limit` 1–30, `full=1` (fetch the article body), `refresh=1`.
- Default output is cover + author + summary; with `full=1` the body is fetched and cached for 24 hours, and keywords are added (the list API does not return them).
- The upstream is slow: 3–7 s for the list and 3–20 s per article for the body, so `full` is off by default.
- Member-only articles return just the summary, labelled "Member-only article; summary shown."; the paywall is not bypassed.

---

## <img src="assets/v2ex.png" width="28" height="28" align="absmiddle" alt="V2EX"> V2EX

Official site <https://www.v2ex.com> · Data sources: official Atom + hot page scraping

| Content | RSS | JSON | Default | Max | Cache |
|---|---|---|---|---|---|
| Latest topics | `/rss/v2ex` | `/api/v1/v2ex?section=latest` | 20 | 50 | 10 minutes |
| Hot topics | `/rss/v2ex/hot` | `/api/v1/v2ex?section=hot` | 20 | 50 | 10 minutes |

### Latest topics

- Official Atom `https://www.v2ex.com/index.xml`, 50 entries, with full body, author, and publish time.
- Parameters: `limit` 1–50, `refresh=1`.

### Hot topics

- The official site has **no** hot feed: we scrape `https://www.v2ex.com/?tab=hot`, filter out the "Promotions" node, and sort by reply count descending.
- The ranking exists only on that page. `api/topics/hot.json` returns a different list (today's most-discussed topics, 10 items), so it is **not** used as a fallback: a page that cannot be read (network error, Cloudflare challenge, fewer than five parsed topics) fails the request, and the reader is served the last good hot list with `X-Feedmesh-Degraded: 1` instead of a list with different semantics.
- Cloudflare answers the HTML page with a JS challenge (403, "Just a moment...") when the request carries `Accept-Language: zh-CN`; the client therefore sends only `User-Agent` and `Accept: */*`. The Atom feed is unaffected.
- Parameters: `limit` 1–50, `refresh=1`.

---

## <img src="assets/smzdm.png" width="28" height="28" align="absmiddle" alt="SMZDM"> SMZDM

Official site <https://www.smzdm.com> · Data source: the official deal RSS on `http://feed.smzdm.com` (the HTTPS host answers 403)

| Content | RSS | JSON | Default | Max | Cache |
|---|---|---|---|---|---|
| Selected deals | `/rss/smzdm`, `/rss/smzdm.xml` | `/api/v1/smzdm` | 20 | 30 | 5 minutes |
| Community articles | `/rss/smzdm/post`, `/rss/smzdm/post.xml` | `/api/v1/smzdm?channel=post` | 20 | 30 | 10 minutes |
| Overseas deals | `/rss/smzdm/haitao`, `/rss/smzdm/haitao.xml` | `/api/v1/smzdm?channel=haitao` | 20 | 30 | 5 minutes |
| Category good prices | `/rss/smzdm/fenlei/{slug}`, `.xml` | `/api/v1/smzdm?channel=fenlei&slug=` | 20 | 30 | 60 minutes |

- The subscription page advertises five channels; **three serve plain RSS** and are wired up. `faxian` and `news` answer 302/202 with a WAF challenge (`probe.js`) and are never requested.
- Every channel returns 30 entries per call. The two deal channels **rotate**: two fetches minutes apart can share 0–18 items, and entries inside one slice are not strictly time-ordered, so the client sorts by `pubDate` before truncating. A reader that keeps history sees a wider set than any single fetch.
- Selected and overseas deals: the title carries brand, model, capacity and price; `link` points at the deal page (`/p/<id>/`); the cover comes from `focus_pic` and also goes into Media RSS. `content:encoded` usually carries only the "this article comes from smzdm" footer — when it carries a real blurb (`itemprop="description"`), that becomes the item text. `description` carries the poster's nickname, and on blurb-bearing entries it is nickname + blurb concatenated; the client splits it on the blurb (a punctuation-free field of at most 14 characters counts as a nickname).
- Community articles: permalinks are slugs (`/p/<slug>/`), `<author>` carries the writer and `<description>` the blurb; the feed ships no cover image.
- `pubDate` is Beijing time when the zone is omitted; the API view converts it to UTC. An unparsable timestamp falls back to the fetch time.
- Search (`search.smzdm.com`) answers automated requests with a WAF challenge (HTTP 202 + `probe.js`, or a Tencent CAPTCHA): it is never requested, and no challenge is ever solved. Category pages, by contrast, are plain server-rendered HTML and are read on a strict budget.
- Parameters: `limit` 1–30, `refresh=1`.

### Category good prices

`https://www.smzdm.com/fenlei/{slug}/` lists the good prices of one interest category (1111 slugs exist; `tixuhuli` is shaving, `tixuchumao` shaving and hair removal). It is the way to see deals **outside** the curated RSS slice: the RSS channels are an editorially filtered pool, while a category page shows what the site itself lists under that interest. Shopping agents that only read the RSS channels miss those deals entirely.

- The page is 600–720 KB and server-rendered; the cards live in the Nuxt payload script (`__NUXT_DATA__`), which the client reads instead of scraping the markup. One page carries 16–20 cards. A cold read measured 8 s when warm and up to 51 s under load, so category pages use their own 60 s timeout.
- Only good-price cards survive: article, review and repost cards share the page (`post.smzdm.com` permalinks, no price) and are dropped, as is any card without a readable price.
- `price` is the leading amount of the display price (`7.9`); `text` keeps the whole display string so coupons and multi-buy conditions survive. Category cards carry no poster, so `author` is empty.
- The page publishes no timestamp for a good-price card, so `created_at` is the page read time, and `date_source` is `fetched` to say so; an article card that does carry one gets it parsed (a `MM-DD HH:MM` stamp that lands more than a day in the future belongs to the previous year) and is published as `card`. Every item of one read shares the same instant, and a cache hit keeps that instant instead of advancing to the request time, so a polling reader can tell one batch from the next.
- **Budget**: at most one page read per slug per hour (the cache TTL is that budget), one read in flight at a time across slugs, and `refresh=1` inside the hour is served from the cache. Every read is logged (`fenlei page read`) so the budget can be audited.
- **Failed reads spend the same budget**: an HTTP error, a transport error, or a large page with no cards records `smzdm:fenlei:<slug>:retry`. When a last good page exists, the rest of the hour serves that page as a degraded document (`X-Feedmesh-Degraded: 1`, `max-age=60`, `"degraded": true` in JSON) instead of re-reading 600–720 KB on every poll; with nothing cached, the rest of the hour answers `UPSTREAM_UNAVAILABLE` with the retry time. The marker lasts one budget window, not the six hours the challenge cooldown uses.
- **Risk control**: a challenge (HTTP 202, `TCaptcha`, or an empty shell under 20 KB with no cards) returns `UPSTREAM_BLOCKED` and puts the slug on a six-hour cooldown (`BREAKER_OPEN` after that); nothing is retried. A large page without cards is treated as a layout change instead, so it does not burn the slug's cooldown.
- `robots.txt` disallows three category paths (`3cjiadian`, `jujiashenghuo`, `shishangyundong`) and any URL carrying a query string; both are honoured — the category URL is always bare, and those slugs are refused with `BAD_REQUEST`.

---

## <img src="assets/x.png" width="28" height="28" align="absmiddle" alt="X"> X (Twitter)

Official site <https://x.com> · Data source: RSS from public Nitter instances (anonymous, no account, no API key)

| Content | RSS | JSON | Default | Max | Cache |
|---|---|---|---|---|---|
| Account timeline | `/rss/x/{handle}` | `/api/v1/x?handle=` | 20 | 20 | 40 minutes |

### Account timeline

- The instance pool is pulled from `https://status.d420.de/api/v1/instances` (filtered on `rss=true`, `points ≥ 60`, non-bad hosts) and reused for 30 minutes; if the directory is unavailable or every returned node is a private-network or invalid address, it falls back to the built-in list (kareem.one, meowing.monster), and the directory is not retried for 30 seconds after a failure.
- The directory is aggressively rate-limited per client IP (a second read right after the first is answered with 429), so a cached pool is kept across directory failures. Instance scores alone do not decide the attempt order: an instance that failed within the last 10 minutes is ranked behind the healthy ones (a soft hint, never a ban — when every candidate is cooling down they are retried in their original order, and a success clears the record immediately). That keeps a stale pool full of dead high-score mirrors from costing a round trip on every request.
- Instance addresses are accepted only if they are https, carry no userinfo, are not IP literals, and are not `.local`/`.internal`; at dial time the resolved IP is also checked to be a public address (not loopback, RFC1918, link-local, and so on), so a third-party directory cannot steer requests at the local host or the internal network. Instances configured explicitly are trusted by the operator.
- Pool nodes are tried in score order with the health hint above (at most 3 directory nodes per request; 20 s per attempt by default, 25 s max); a timeout, a 5xx, or 0 returned items moves on to the next instance. Operator-configured fallbacks are always eligible and do not spend the directory budget, and one timeline call is bounded to 45 seconds in total. If everything fails it returns `UPSTREAM_UNAVAILABLE` (HTTP 504).
- Every failed instance is logged with its handle, host and error (`nitter instance failed`), and every 5xx response is logged with its path and error code. The HTTP error itself stays generic (`xnitter: all instances failed (N tried)`) so the mirror pool is not exposed to callers; the journal is where the detail lives.
- Some instances sit behind an Anubis challenge (`anubis_challenge` / `preact_info`): the client solves the SHA-256 PoW itself, caches the cookie, and retries — no browser required.
- Instance links in the body are rewritten to `https://x.com/...` with the query preserved; Nitter's `/pic/` image proxy is restored to `pbs.twimg.com`, while other image domains are left alone.
- Photos are requested at the original size (`?name=orig`). The bare CDN URL serves a ~1200 px rendition — measured 672×1200 / 228 KB versus 1146×2047 / 516 KB for the original. Card banners keep their own size parameter and video posters are already full size.
- Only the post's own photos land in `images`. A quoted post is rendered by Nitter as a `blockquote`, so its media is parsed into `quoted` and never counted as the entry's own (measured on a live feed: 4 images reported where the post had 2, all four from the quote). Video posters become `video.thumbnail`, link-card banners `card_image`.
- Nitter's RSS carries no playable media and its HTML pages are behind Cloudflare/Gandalf on the instances that serve RSS, so video is enriched from X's public embed endpoint (`cdn.syndication.twimg.com`) once per post: highest-resolution MP4 variant, duration, view count, likes, replies and the quoted post. Four concurrent reads, 6 s each, 15 s per page; a failed read leaves the RSS item untouched. Disable it with `--no-x-enrich` or `FEEDMESH_X_ENRICH=0`, which keeps images and posters but drops video URLs and counts.
- `doctor --x-handle <handle>` walks the pool and prints one line per instance (status, latency, item count). A rate-limited directory keeps the fallback list as a note instead of failing the run.
- Repost entries are prefixed with `🔁` in the title, and Nitter's original `RT by @user:` prefix is stripped.
- Parameters: `limit` 1–20, `refresh=1`.

### Limitations

- **Depends on third-party volunteer instances**: the pool holds only a handful of healthy nodes and can go down entirely at any time. The service itself stays up (the other platforms are unaffected), but `/rss/x/*` returns an upstream error.
- **One page only**: the RSS route returns at most 20 posts, ignores `?cursor=`, and has no `since_id`, so a burst of more than 20 posts inside the cache window loses the overflow. Pagination would require the paid X API.
- **Engagement is partial**: the embed endpoint returns likes, replies and view counts, but no retweet or quote counters. Nitter's RSS has none at all.
- Poll at a low rate, honoring `<ttl>40</ttl>` (the server already caches for 40 minutes); aggressive polling gets rate-limited by the instances.
- Public accounts only; protected accounts and login-only content cannot be fetched.

---

## <img src="assets/youtube.png" width="28" height="28" align="absmiddle" alt="YouTube"> YouTube

Official site <https://youtube.com> · Data source: the channel upload feed (`/feeds/videos.xml`; anonymous, no API key, no account)

| Content | RSS | JSON | Default | Max | Cache |
|---|---|---|---|---|---|
| Channel uploads | `/rss/youtube/{ref}` | `/api/v1/youtube?ref=` | 15 | 15 | 10 minutes |

### Channel uploads

- `{ref}` in the RSS path is a channel id (24 characters) or a handle (`@` optional); a pasted channel URL carries slashes and only works through the JSON endpoint's `ref=` parameter, which accepts `/@name`, `/channel/UC…` and the legacy `/c/…` and `/user/…` forms (YouTube serves the last two directly or redirects them to the canonical channel page) and ignores `/videos`, `/shorts`, `/streams` tails and query strings.
- Turning a handle into an id costs one channel-page read (about 1.4 MB) and is cached for 24 hours; a handle and every spelling of it (`@name`, `name`, a pasted `/@name` URL) share one mapping, while `/c/…` and `/user/…` URLs resolve on their own (the same name can point at a different channel there), and ids cost no extra request. A page that answers 404 is not retried. An expired mapping is reused when the page cannot be read (an outage window, or a consent/bot page instead of the channel), so a failing channel page does not blank a feed whose list is cached.
- The feed carries exactly 15 entries and has no pagination. A channel that uploads more than that between two polls loses the overflow; poll every 10 minutes or accept the ceiling.
- Entries are the upload stream, so Shorts and livestream replays are mixed in with regular videos. `link`/`guid` is always `https://www.youtube.com/watch?v=<id>`, which resolves for both.
- Fields: title, description, thumbnail (the Atom entries carry `hqdefault`, 480×360; the fallback carries its own, up to `hq720`), publish time, view count. Duration is not in the feed and is not published.
- `X-Feedmesh-Source` reports which upstream answered: `atom`, `browse` (the web-client fallback) or `stale` (the last good cached list). It is the operator signal for an outage window.
- One feed read is up to 3 attempts (2 s, then 4 s apart), and the whole lookup — channel page and feed, retries plus the fallback — is bounded to 30 seconds. When all attempts fail, the client switches to the web client's browse endpoint, which returns up to 30 entries with relative timestamps ("5 days ago") and no description; those items carry approximate dates (several uploads can share one) and repeat the title as their text. If that also fails, the last good list is served with `X-Feedmesh-Degraded: 1` for up to 24 hours. A successful read that comes back with no entries keeps the cached list too, so a partial answer cannot blank a feed.

### Limitations

- **The endpoint is not always available**: it answers 404 or 500 for short windows even for channels that exist (RSS-Bridge #2113, newsboat #3269, Google's developer forum). The service retries, then falls back to the web client endpoint, then to the last good list, so readers normally see a feed; only when both sources fail and nothing is cached does the route return 504.
- The fallback's dates are approximate (day-level relative text) and its entries have no description, so a feed built during an outage looks thinner than one built from the Atom feed. The next successful Atom read replaces it.
- Public channels only: members-only and private uploads never appear.
- Metadata only: no duration, resolution or live status.

---

## <img src="assets/reddit.png" width="28" height="28" align="absmiddle" alt="Reddit"> Reddit

Official site <https://www.reddit.com> · Data source: the public Atom listing (`/r/{sub}/{sort}/.rss`; anonymous, no OAuth application, no API key)

| Content | RSS | JSON | Default | Max | Cache |
|---|---|---|---|---|---|
| Subreddit listing | `/rss/reddit/{sub}`, `/rss/reddit/r/{sub}` | `/api/v1/reddit?sub=` | 20 | 25 | 10 minutes |

### Subreddit listing

- `{sub}` is the subreddit name; a `r/` prefix and any case are accepted, so `/rss/reddit/r/Rust.xml` works. The JSON route takes the name in `sub=`.
- `sort` is `hot` (default), `new` or `top`; `window` (`day` by default, `week`, `month`, `year`, `all`) selects the period of `top` and is ignored by the other sorts. Each `(sub, sort, window)` triple is cached on its own.
- One Atom page carries 25 entries and the document has no pagination, so 25 is the maximum.
- Fields: submission id (`t3_…`), title, submitter (`author`, without the `/u/` prefix), publish time to the second, the comment-page permalink and the outbound link of a link submission. The document carries **no score, comment count, thumbnail or self text** — those are JSON-only, and the anonymous JSON endpoints answer 403 to datacenter addresses, so they are not used at all.
- Entry order is the document order: `hot` is a ranking, `new` and `top` are already ordered upstream, so the service never re-sorts.
- Cache: one entry per `(sub, sort, window)`, fresh for 10 minutes; the last good listing is kept for 24 hours and served with `X-Feedmesh-Degraded: 1` and `X-Feedmesh-Source: stale` when a read fails or the limiter refuses it.
- **Pacing (hard constraint)**: one global read slot — at most one request in flight, at least 8 seconds between two reads, and no read at all while a reported quota window (`x-ratelimit-reset`) is open. Measured on this datacenter address: **one read per ~60 seconds**, after which the next request answers 429 with an empty body. A request that cannot read now degrades or fails; it is never queued, retried, replayed or moved to another address.
- Every read is logged (`reddit upstream read`) with the subreddit, sort, status and duration, so the pacing can be audited from the journal.
- An unknown subreddit: the site redirects to its subreddit search instead of answering 404. That redirect is classified as `USER_NOT_FOUND` (404) and is not followed.

### Limitations and terms of service

- **Anonymous RSS is not an officially supported automation path.** Reddit's developer terms describe OAuth applications; the Atom listing is a reader feature that the site rate limits without notice. Treat this provider as best effort: it can start answering 403/429 at any time, and nothing on our side needs to change for it to break.
- **Low frequency is the design**: one read per quota window, a 10-minute cache per listing, no pagination, no retry storm, and no attempt to evade a block (no IP rotation, no proxies, no user-agent games). A listing that cannot be read degrades to the last good one instead of hammering the site.
- If the source stops working, the honest fix is to stop calling it. The supported path would be Reddit's OAuth Data API, which contradicts this service's zero-credential constraint and is out of scope.
- Scores, comment counts and thumbnails are unavailable here; a client that needs them needs the credentialed API.
- Public subreddits only: private, quarantined and banned ones answer a redirect or an error.

---

## <img src="assets/douyin.png" width="28" height="28" align="absmiddle" alt="Douyin"> Douyin

Official site <https://www.douyin.com> · Data source: the public creator page, driven by a headless Chromium (anonymous visitor identity; no account)

| Content | RSS | JSON | Default | Max | Cache |
|---|---|---|---|---|---|
| Creator videos | `/rss/douyin/{sec_uid}` | `/api/v1/douyin?user=` | 20 | 44 | 10 minutes |

### Creator videos

- `{sec_uid}` is the `MS4w…` segment of `https://www.douyin.com/user/<sec_uid>`; the JSON route takes it in `user=`. The downloader-facing view is `/api/v1/douyin/timeline?user=`.
- Douyin signs every web API call with a browser-generated token, so the service does **not** reimplement the signature: it starts Chromium with `--remote-debugging-pipe`, opens `douyin.com`, and issues the request from inside the page. The DevTools protocol is implemented with the standard library; the binary stays free of third-party Go modules.
- Chromium is a runtime dependency: set `FEEDMESH_CHROME` to an explicit binary when the default lookup (`chromium`, `chromium-browser`, `google-chrome`, or the Hermes bundled build) does not find one. The visitor profile lives in `<cache>/douyin-profile`.
- The page needs about 15 seconds before its signer is ready, so a cold read takes roughly half a minute; the browser is closed after two minutes without a request. Cached reads are served in milliseconds.
- One page carries at most 44 videos. Guest sessions cannot paginate (`max_cursor` answers an empty body), so a creator with a longer history is covered by the newest page only.
- The cache is width-aware like the other list providers: a `?limit=3` read is widened in place by a later `?limit=20` read (the upstream `count` follows the request), so a small read never narrows the creator's cache for the rest of the TTL.
- Pinned videos (`is_top`) arrive first upstream; the service re-sorts every item by publish time and keeps the pinned flag in the body text.
- The play address (`play_url`) is a time-limited watermark-free MP4 URL. It is exposed by `/api/v1/douyin/timeline` for the download tool and never embedded in the RSS document, because a stored address expires within hours.
- The caption (`desc`) is the only text field Douyin provides, so it is kept verbatim in both the item title and the body: no first-line rule and no length cap.
- Cache: fresh for 10 minutes; the last good list is kept for six hours and served with `X-Feedmesh-Degraded: 1` when a read fails. An invalid `sec_uid` returns `BAD_REQUEST` (400); an unavailable page returns `UPSTREAM_UNAVAILABLE` (504).

---

## <img src="assets/tiktok.png" width="28" height="28" align="absmiddle" alt="TikTok"> TikTok

Official site <https://www.tiktok.com> · Data source: the public profile page and the web client's creator endpoint (anonymous; no account, no cookie)

| Content | RSS | JSON | Default | Max | Cache |
|---|---|---|---|---|---|
| Creator videos | `/rss/tiktok/{handle}` | `/api/v1/tiktok?user=` | 20 | 45 | 10 minutes |

### Creator videos

- `{handle}` is the `@name` without the `@`; the JSON route takes it in `user=`. A `secUid` (`MS4wLjABAAAA…`) is accepted too, but then the display name is recovered from the first video instead of the profile page. The downloader-facing view is `/api/v1/tiktok/timeline?user=`.
- TikTok serves profile pages behind a Slardar WAF proof-of-work puzzle. The puzzle is pure SHA-256 — find the integer whose digest matches a target — so the provider solves it with `crypto/sha256` in a millisecond and retries the page with the returned cookie. No browser, no JavaScript engine and no third-party module is involved; a cold read takes about 0.8 seconds.
- The creator endpoint returns at most 15 items per page and rejects any other `count` with `invalid count parameter`, so the provider walks up to three pages (45 items) using the newest item's `createTime` as the next cursor.
- Item `title` is the whole caption: TikTok has no separate title field, and shortening it here would be the only place data gets shorter. The body carries the caption, the cover image, duration/resolution, play/like/comment/share counters and the music credit.
- The play address (`play_url`) is a time-limited, watermark-free MP4 URL for the highest-bitrate rendition. It is exposed by `/api/v1/tiktok/timeline` for the download tool and never embedded in the RSS document, because a stored address expires within hours. Photo posts carry a cover but no play address.
- Cache: fresh for 10 minutes; the last good list is kept for six hours and served with `X-Feedmesh-Degraded: 1` when a read fails. The cache is width-aware like the other list providers.
- Limits worth knowing: private accounts and content that TikTok gates by region answer `USER_NOT_FOUND` (404) — the same post works or fails depending on the viewer's IP, so a failure is not always a bad handle. Hashtags, sounds and search are not exposed: only creator timelines are available. A handle that does not exist returns `USER_NOT_FOUND` (404); a malformed one returns `BAD_REQUEST` (400); an unsolvable challenge returns `UPSTREAM_UNAVAILABLE` (504).

---

## <img src="assets/bilibili.png" width="28" height="28" align="absmiddle" alt="Bilibili"> Bilibili

Official site <https://www.bilibili.com> · Data source: the public space API with a WBI signature (anonymous visitor cookie; no account)

| Content | RSS | JSON | Default | Max | Cache |
|---|---|---|---|---|---|
| Creator videos | `/rss/bilibili/{mid}` | `/api/v1/bilibili?user=` | 20 | 50 | 10 minutes |

### Creator videos

- `{mid}` is the numeric account id from `space.bilibili.com/<mid>`; the JSON route takes it in `user=`.
- Bilibili signs every space request with WBI: two rotating image names from `/x/web-interface/nav` are permuted through a fixed 64-entry table to build a mixin key, then the sorted query plus the key is hashed with MD5 into `w_rid`. The algorithm is implemented with `crypto/md5` and the standard library only — no browser, no JavaScript engine, no third-party module.
- The API also wants a visitor identity: `buvid3`/`buvid4` are fetched once per process from `/x/frontend/finger/spi`, and the browser-fingerprint parameters (`dm_img_list`, `dm_img_str`, `dm_cover_img_str`, `dm_img_inter`) are generated per request.
- Bilibili rate-limits datacenter IPs aggressively: a signed request often answers `412 Precondition Failed` or `code -352`, and the same request succeeds moments later. The provider retries up to six times with a short backoff (about nine seconds total), which lifts the cold-read success rate to roughly 80% from this host; the ten-minute cache keeps the steady-state request rate low.
- The profile card (`/x/web-interface/card`) does not need a signature and supplies the display name, avatar, signature and follower count; the video list comes from `/x/space/wbi/arc/search` with `order=pubdate` and up to 50 items per page.
- Item `title` is the upload title, `text` is the description, and the body carries the cover, description, duration, play/danmaku/comment counters and the category name. `views` is the play counter.
- Cache: fresh for 10 minutes; the last good list is kept for six hours and served with `X-Feedmesh-Degraded: 1` when a read fails. The cache is width-aware like the other list providers.
- Limits worth knowing: only a creator's uploads are exposed (no dynamics, collections, live or search). A missing mid returns `USER_NOT_FOUND` (404); a non-numeric one returns `BAD_REQUEST` (400); an exhausted rate-limit retry returns `UPSTREAM_BLOCKED` (502). Because the block is IP-based, a cookie from a logged-in browser (not required today) is the usual way to raise the success rate further.

---

## <img src="assets/telegram.png" width="28" height="28" align="absmiddle" alt="Telegram"> Telegram

Official site <https://telegram.org> · Data source: the public web preview at `t.me/s/<channel>` (anonymous; no account, bot token or API key)

| Content | RSS | JSON | Default | Max | Cache |
|---|---|---|---|---|---|
| Public channel posts | `/rss/telegram/{channel}` | `/api/v1/telegram?channel=` | 20 | 20 | 10 minutes |

### Public channel posts

- `{channel}` is the public username from `t.me/<channel>`, with or without `@`; a full `t.me/s/<channel>` URL also works. The JSON route takes the same value in `channel=`.
- Only channels that have the public web preview enabled can be read. Private channels and groups have no anonymous view; they return `USER_NOT_FOUND` (404), because reading them would require a logged-in account, which this service deliberately does not keep.
- One preview page carries about 20 posts, so 20 is both the default and the maximum. `refresh=1` forces a read; the ten-minute cache keeps polling gentle on t.me.
- The body carries the post text as sanitised HTML, every photo of an album, a video still, a link card (cover, title, summary and site) and a meta line with the author signature, forward source and view counter. The item `title` is the whole caption — Telegram has no separate title field — and Media RSS repeats the photos.
- Video messages expose the still frame only. The `<video src>` on the preview page is a signed URL that expires; embedding it would leave a dead player in an archived feed. Use the post permalink (`https://t.me/<channel>/<id>`) to play it.
- Per-post author signatures (`from_author`) are published as `author` in the JSON view and in the body meta line; the channel title is the feed title with the suffix `on Telegram`.
- Cache: fresh for 10 minutes; the last good list is kept for six hours and served with `X-Feedmesh-Degraded: 1` when a read fails. The cache is width-aware like the other list providers.
- A malformed username returns `BAD_REQUEST` (400). A 200 page with no visible post returns `UPSTREAM_UNAVAILABLE` (504) instead of caching an empty feed; a `429` from t.me returns `RATE_LIMITED` (429).
- The preview page is parsed with the standard library only (regexp plus a balanced-`div` scan). If t.me changes the widget markup, the parser reports `UPSTREAM_UNAVAILABLE` rather than emitting half-empty posts.

---

## <img src="assets/linuxdo.png" width="28" height="28" align="absmiddle" alt="LINUX DO"> LINUX DO

Official site <https://linux.do> · Data source: the forum's official Discourse RSS feeds (anonymous; no account, cookie or API key)

| Content | RSS | JSON | Default | Max | Cache |
|---|---|---|---|---|---|
| Latest topics | `/rss/linuxdo` | `/api/v1/linuxdo?feed=latest` | 30 | 90 | 15 minutes |
| Top topics | `/rss/linuxdo/top?period=daily` | `/api/v1/linuxdo?feed=top&period=` | 30 | 90 | 15 minutes |
| Hot topics | `/rss/linuxdo/hot` | `/api/v1/linuxdo?feed=hot` | 30 | 90 | 15 minutes |
| Category topics | `/rss/linuxdo/c/{slug}/{id}` | `/api/v1/linuxdo?feed=category&category={slug}/{id}` | 30 | 90 | 15 minutes |
| Tag topics | `/rss/linuxdo/tag/{tag}` | `/api/v1/linuxdo?feed=tag&tag=` | 30 | 90 | 15 minutes |

### Forum topics

- Feeds are read from the official `.rss` paths only. The forum's JSON and HTML paths answer a Cloudflare managed challenge, and once that challenge fires it also blocks RSS for several minutes; the provider therefore never requests them.
- The client keeps a cookie jar (Cloudflare's visitor cookie) and a global read slot: at most one request every five seconds across every LINUX DO feed. A challenge pauses all feeds for ten minutes instead of retrying, because retrying extends the block.
- `latest` defaults to `?order=created` (stable creation order); pass `order=activity` to follow the forum's last-reply order. Top and hot are rankings and keep the upstream order; the other feeds are sorted by publication time.
- The RSS feeds carry 30 topics per page (25 for categories). `limit` may go up to 90 and the client walks up to three pages; the cache is width-aware like the other list providers.
- A category accepts both `{slug}/{id}` (recommended, e.g. `develop/4`) and an id-only path (`/rss/linuxdo/c/4`), which follows the forum's redirect to the canonical slug.
- Each item carries the first post as sanitised HTML, the author, the category, the topic state (pinned/closed/archived), the topic permalink and Media RSS images. The body keeps Discourse's reply count and "read the full topic" footer; the forum's RSS does not expose reply counts, view counts or tags as separate fields.
- Cache: fresh for 15 minutes; the last good list is kept for 24 hours and served with `X-Feedmesh-Degraded: 1` when Cloudflare challenges a read. With nothing cached a challenge returns `UPSTREAM_BLOCKED` (502); a missing category or tag returns `USER_NOT_FOUND` (404); a malformed reference returns `BAD_REQUEST` (400).
- The forum is high-traffic: one page covers roughly the last 15 minutes of topics, so this is a rolling window, not a complete archive. It is also a cloud-hosted IP; Cloudflare may challenge it more often than a residential connection.

---

## <img src="assets/pinterest.png" width="28" height="28" align="absmiddle" alt="Pinterest"> Pinterest

Official site <https://www.pinterest.com> · Data source: the public RSS feeds at `/<user>/feed.rss` and `/<user>/<board>.rss` (anonymous; no account, cookie, token or browser)

| Content | RSS | JSON | Default | Max | Cache |
|---|---|---|---|---|---|
| Profile pins | `/rss/pinterest/{user}` | `/api/v1/pinterest?user=` | 20 | 25 | 10 minutes |
| Board pins | `/rss/pinterest/{user}/{board}` | `/api/v1/pinterest?user=&board=` | 20 | 25 | 10 minutes |

### Profile and board pins

- `{user}` accepts a bare profile name (`nasa`), an `@handle` or a profile URL. `{board}` is the slug from the board URL (`https://www.pinterest.com/<user>/<board>/`). Both are case-insensitive upstream; the cache key lowercases them.
- One feed slice carries at most 25 pins and has no pagination, so 25 is both the default and the maximum. Polling more often than the reader needs only adds Pinterest traffic; the cache is fresh for ten minutes.
- The feed keeps Pinterest's own order. A pin added to a board today can carry an old creation date, and re-sorting by `pubDate` would bury it. The profile feed is newest-first upstream.
- Each item carries the pin permalink, the pin title (falling back to the caption and then `(no content)`), the caption as plain text, and one image. The feed stamps every thumbnail as `/236x/`; the provider upgrades it to `/736x/` before publishing. The `/originals/` path is not used: four of six tested pins answered 403 there, while 736x answered for every one.
- `refresh=1` forces a read. The last good result is kept for six hours and served with `X-Feedmesh-Degraded: 1` when the read fails.
- A missing profile or board returns `USER_NOT_FOUND` (404), with `User not found` or `Board not found` in the message; a malformed name returns `BAD_REQUEST` (400); a `429` returns `RATE_LIMITED` (429). A 200 answer that is an HTML shell instead of RSS returns `UPSTREAM_UNAVAILABLE` (504) and is never cached.
- The endpoints are public and have been recommended to feed readers for years, but Pinterest's `robots.txt` says `User-agent: *` → `Disallow: /`: generic crawlers are not on its allowlist. That is a policy decision for the operator, not a technical limit of this provider.
- The official API v5 is not an alternative for this use case: it requires a registered app and OAuth, trial-created entities are sandboxed, and apps registered after 2026-09-14 can receive `PINNER_DATA_ACCESS_DENIED` for personal (non-business) boards and pins. Standard access needs a business account, a privacy policy and a recorded review.

---

## <img src="assets/polymarket.png" width="28" height="28" align="absmiddle" alt="Polymarket"> Polymarket

Official site <https://polymarket.com> · Data source: the public Gamma API (`gamma-api.polymarket.com`) and Data API v2 (`data-api.polymarket.com/v2`), both anonymous

| Content | RSS | JSON | Default | Max | Cache |
|---|---|---|---|---|---|
| Newest events | `/rss/polymarket/new` | `/api/v1/polymarket?feed=new` | 20 | 50 | 10 minutes |
| Trending events | `/rss/polymarket/hot` | `/api/v1/polymarket?feed=hot` | 20 | 50 | 10 minutes |
| One event's markets | `/rss/polymarket/event/{slug}` | `/api/v1/polymarket?feed=event&slug=` | 20 | 50 | 5 minutes |
| Wallet activity | `/rss/polymarket/user/{address}` | `/api/v1/polymarket?feed=user&address=` | 30 | 100 | 5 minutes |

### Events, odds and wallet activity

- `new` reads `order=createdAt`; `hot` reads `order=volume24hr`. Both accept `tag=<slug>` (for example `politics`, `sports`, `crypto`; the tag must be a lowercase slug).
- **The newest list always excludes tag `102169` (`Hide From New`).** In a measured sample, 83 of the 100 newest events were five-minute crypto `Up or Down` markets; with the exclusion, 20 of 20 were normal events. Without it the feed is unusable.
- The Gamma API is generous but returns large documents: a 50-event page is ~725 KB gzip-decoded before trimming, because multi-market events embed every market. The provider keeps only the top three markets per event for list views, which is what the RSS body shows. Go's HTTP client negotiates gzip automatically; without it Gamma answers `406 gzip_required` for pages above 50.
- `event/{slug}` sorts markets by volume and caps them at the request limit. Odds are rendered as `Yes 47.5% · No 52.5%`; a market's 24-hour price change is shown in percentage points when present.
- `user/{address}` reads Data API v2 `/activity` (`sort_by=TIMESTAMP`). Each item is one trade, split, merge, redeem or reward, with side, outcome, price, size and USDC value. Addresses are validated as `0x` + 40 hex characters and lowercased.
- **RSS semantics**: odds are a snapshot, not an event. Readers do not re-notify an item whose content changed, so the single-event feed is a dashboard. Use `new` or `user` when a new item should trigger a notification.
- Cache: lists 10 minutes, single event and wallet 5 minutes; last good entry kept for six hours. `refresh=1` forces a read.
- Errors: unknown event slug → `USER_NOT_FOUND` (404); malformed tag/slug/address → `BAD_REQUEST` (400); upstream rejection → `BAD_REQUEST` or `UPSTREAM_UNAVAILABLE`; `429` → `RATE_LIMITED`.
- There is no official RSS; the APIs are documented public reads with no key. Rate limits are tiered by Builder status and not published as a fixed number for anonymous reads, so the ten-minute cache is the main protection.

---

## <img src="assets/apple.png" width="28" height="28" align="absmiddle" alt="Apple Podcasts"> Apple Podcasts

Official site <https://podcasts.apple.com> · Data sources: iTunes Lookup (no key) and the publisher's own RSS feed; the chart is Apple's public marketing-tools JSON

| Content | RSS | JSON | Default | Max | Cache |
|---|---|---|---|---|---|
| Show episodes | `/rss/apple/{id}` | `/api/v1/apple?id=` | 20 | 50 | 30 minutes |
| Top shows | `/rss/apple/top` | `/api/v1/apple?chart=top&country=` | 20 | 50 | 30 minutes |

### Show episodes

- `{id}` is the numeric Apple show id (`1200361736`) or a `podcasts.apple.com` URL that contains `/id1200361736`. The JSON route takes it in `id=`.
- Apple does not host the audio. Lookup returns the publisher `feedUrl`; the provider fetches that RSS and keeps the newest 50 episodes. Show notes stay as the publisher wrote them.
- The audio file is an `enclosure` (`audio/mpeg`, `audio/mp4`, …) with the publisher's byte length when the feed has one. `extra.duration` is the episode length in seconds.
- A publisher URL that is not public https (loopback, a private address, or a name that resolves to one) is refused and never fetched.
- Parameters: `limit` 1–50, `refresh=1`.
- Cache: fresh for 30 minutes; the last good list is kept for six hours and served with `X-Feedmesh-Degraded: 1` when a read fails.
- A missing show, or a show with no `feedUrl`, returns `USER_NOT_FOUND` (404). A malformed id returns `BAD_REQUEST` (400). Lookup is documented at about 20 calls a minute; the cache is what keeps a reader poll inside that.

### Top shows

- `country` is a two-letter storefront (`us` by default, also `cn`, `jp`, …). The chart has no episode audio: each item is a show, and its `guid` is the Apple show URL, so a rank change does not make readers push the same show again.
- The upstream chart page is 50 rows. `limit` only trims the answer.

---

## Adding a platform

1. Create `internal/<platform>/client.go`; it only handles HTTP and parsing and returns the neutral `Article` struct.
2. Add a field to `feed.Clients`, implement `feed.Get<Platform>()`, and map the result into `feed.Item` (HTML, Images, CreatedAt, URL).
3. Register both the RSS and the JSON route in `httpapi/server.go`.
4. Pin responses with `httptest`, covering parsing, sorting/filtering, cache hits, and empty responses.
5. Update this file, [API.md](API.md), and the platform table in the README, and drop the platform icon into `docs/assets/`.

## Trademarks

The platform icons are trademarks of their respective owners, used solely to identify the data source; they do not imply any affiliation or endorsement.

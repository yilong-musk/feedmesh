# Supported platforms

**5 platforms and 9 content types** are currently wired up. Each platform below lists the content it supports and the matching endpoints; adding a platform only takes one provider — see the end of this page.

| Platform | Content | RSS | JSON |
|---|---|---|---|
| <img src="assets/weibo.png" width="20" height="20" alt="Weibo"> **Weibo** | Account timeline | `/rss/user/{uid}`<br>`/rss/domain/{name}` | `/api/v1/feed?uid=` |
| <img src="assets/weibo.png" width="20" height="20" alt="Weibo"> **Weibo** | Realtime hot search | `/rss/hot` | `/api/v1/hot` |
| <img src="assets/sspai.png" width="20" height="20" alt="sspai"> **sspai** | Latest articles | `/rss/sspai` | `/api/v1/sspai` |
| <img src="assets/v2ex.png" width="20" height="20" alt="V2EX"> **V2EX** | Latest topics | `/rss/v2ex` | `/api/v1/v2ex?section=latest` |
| <img src="assets/v2ex.png" width="20" height="20" alt="V2EX"> **V2EX** | Hot topics | `/rss/v2ex/hot` | `/api/v1/v2ex?section=hot` |
| <img src="assets/smzdm.png" width="20" height="20" alt="SMZDM"> **SMZDM** | Selected deals | `/rss/smzdm` | `/api/v1/smzdm` |
| <img src="assets/smzdm.png" width="20" height="20" alt="SMZDM"> **SMZDM** | Community articles | `/rss/smzdm/post` | `/api/v1/smzdm?channel=post` |
| <img src="assets/smzdm.png" width="20" height="20" alt="SMZDM"> **SMZDM** | Overseas deals | `/rss/smzdm/haitao` | `/api/v1/smzdm?channel=haitao` |
| <img src="assets/x.png" width="20" height="20" alt="X"> **X** | Account timeline | `/rss/x/{handle}` | `/api/v1/x?handle=` |

Common parameters: `limit`, `refresh=1` (skip the stale cache and force an upstream fetch, keeping the cache width); the `.xml` suffix on an RSS path is equivalent to no suffix. Full contract: [API.md](API.md).

---

## <img src="assets/weibo.png" width="28" height="28" alt="Weibo"> Weibo

Official site <https://weibo.com> · Data source `m.weibo.cn` (visitor cookie, anonymous)

| Content | RSS | JSON | Default | Max | Cache |
|---|---|---|---|---|---|
| Account timeline | `/rss/user/{uid}`, `/rss/domain/{name}` | `/api/v1/feed?uid=` | 10 | 10 | 5 minutes |
| Realtime hot search | `/rss/hot`, `/rss/hot.xml` | `/api/v1/hot` | 20 | 50 | 5 minutes |

### Account timeline

- Parameters: `limit` 1–10, `refresh=1`; `domain` is resolved to a uid first.
- Output: post body HTML, images (large size with real width and height), repost quote block, full text of long posts, video enclosure, external link cards.
- Upstream: `container/getIndex`, at most 10 posts per page; long posts additionally go through `statuses/extend` (cached for 24 hours).

### Realtime hot search

- Rules: keep only the main ranking with `realpos` 1–50; drop the "Recommended" slots that carry no `realpos`; the pinned `hotgov` entry is not merged in.
- Ranking uses `realpos` rather than the array index, so recommended slots cannot shift the positions.
- `guid` uses the search URL, so a rank change does not make readers push the item again.

### Limitations

- Video direct links are **signed URLs that expire in about 1 hour** and require `Referer: https://weibo.com/`; use `page_url` for long-term archiving.
- The timeline endpoint returns at most 10 posts per page; `limit` cannot page beyond that.

---

## <img src="assets/sspai.png" width="28" height="28" alt="sspai"> sspai

Official site <https://sspai.com> · Data source: sspai.com JSON API (the official RSS only carries summaries)

| Content | RSS | JSON | Default | Max | Cache |
|---|---|---|---|---|---|
| Latest articles | `/rss/sspai`, `/rss/sspai.xml` | `/api/v1/sspai` | 10 | 30 | list 10 minutes · body 24 hours |

- Parameters: `limit` 1–30, `full=1` (fetch the article body), `refresh=1`.
- Default output is cover + author + summary; with `full=1` the body is fetched and cached for 24 hours, and keywords are added (the list API does not return them).
- The upstream is slow: 3–7 s for the list and 3–20 s per article for the body, so `full` is off by default.
- Member-only articles return just the summary, labelled "Member-only article; summary shown."; the paywall is not bypassed.

---

## <img src="assets/v2ex.png" width="28" height="28" alt="V2EX"> V2EX

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
- Fallback: when page scraping yields fewer than 5 topics, fall back to `api/topics/hot.json` (10 topics); only fail when both paths fail.
- Parameters: `limit` 1–50, `refresh=1`.

---

## <img src="assets/smzdm.png" width="28" height="28" alt="SMZDM"> SMZDM

Official site <https://www.smzdm.com> · Data source: the official deal RSS on `http://feed.smzdm.com` (the HTTPS host answers 403)

| Content | RSS | JSON | Default | Max | Cache |
|---|---|---|---|---|---|
| Selected deals | `/rss/smzdm`, `/rss/smzdm.xml` | `/api/v1/smzdm` | 20 | 30 | 5 minutes |
| Community articles | `/rss/smzdm/post`, `/rss/smzdm/post.xml` | `/api/v1/smzdm?channel=post` | 20 | 30 | 10 minutes |
| Overseas deals | `/rss/smzdm/haitao`, `/rss/smzdm/haitao.xml` | `/api/v1/smzdm?channel=haitao` | 20 | 30 | 5 minutes |

- The subscription page advertises five channels; **three serve plain RSS** and are wired up. `faxian` and `news` answer 302/202 with a WAF challenge (`probe.js`) and are never requested.
- Every channel returns 30 entries per call. The two deal channels **rotate**: two fetches minutes apart can share 0–18 items, and entries inside one slice are not strictly time-ordered, so the client sorts by `pubDate` before truncating. A reader that keeps history sees a wider set than any single fetch.
- Selected and overseas deals: the title carries brand, model, capacity and price; `link` points at the deal page (`/p/<id>/`); the cover comes from `focus_pic` and also goes into Media RSS. `content:encoded` usually carries only the "this article comes from smzdm" footer — when it carries a real blurb (`itemprop="description"`), that becomes the item text. `description` carries the poster's nickname, and on blurb-bearing entries it is nickname + blurb concatenated; the client splits it on the blurb (a punctuation-free field of at most 14 characters counts as a nickname).
- Community articles: permalinks are slugs (`/p/<slug>/`), `<author>` carries the writer and `<description>` the blurb; the feed ships no cover image.
- `pubDate` is Beijing time when the zone is omitted; the API view converts it to UTC. An unparsable timestamp falls back to the fetch time.
- Only the RSS channels are used. Search and category pages answer automated requests with a WAF challenge (HTTP 202 + `probe.js`, or a Tencent CAPTCHA), so they are never requested, and the service stays clear of that risk.
- Parameters: `limit` 1–30, `refresh=1`.

---

## <img src="assets/x.png" width="28" height="28" alt="X"> X (Twitter)

Official site <https://x.com> · Data source: RSS from public Nitter instances (anonymous, no account, no API key)

| Content | RSS | JSON | Default | Max | Cache |
|---|---|---|---|---|---|
| Account timeline | `/rss/x/{handle}` | `/api/v1/x?handle=` | 20 | 20 | 40 minutes |

### Account timeline

- The instance pool is pulled from `https://status.d420.de/api/v1/instances` (filtered on `rss=true`, `points ≥ 60`, non-bad hosts) and reused for 30 minutes; if the directory is unavailable or every returned node is a private-network or invalid address, it falls back to the built-in list (kareem.one, meowing.monster, netbub), and failed results are cached for 30 seconds.
- Instance addresses are accepted only if they are https, carry no userinfo, are not IP literals, and are not `.local`/`.internal`; at dial time the resolved IP is also checked to be a public address (not loopback, RFC1918, link-local, and so on), so a third-party directory cannot steer requests at the local host or the internal network. Instances configured explicitly are trusted by the operator.
- Pool nodes are tried in score order (at most 3; 20 s per attempt by default, 25 s max); a timeout, a 5xx, or 0 returned items moves on to the next instance. After the whole pool fails it still tries the built-in fallback. If everything fails it returns `UPSTREAM_UNAVAILABLE` (HTTP 504).
- Some instances sit behind an Anubis challenge (`anubis_challenge` / `preact_info`): the client solves the SHA-256 PoW itself, caches the cookie, and retries — no browser required.
- Instance links in the body are rewritten to `https://x.com/...` with the query preserved; Nitter's `/pic/` image proxy is restored to `pbs.twimg.com`, while other image domains are left alone.
- Repost entries are prefixed with `🔁` in the title, and Nitter's original `RT by @user:` prefix is stripped.
- Parameters: `limit` 1–20, `refresh=1`.

### Limitations

- **Depends on third-party volunteer instances**: the pool holds only a handful of healthy nodes and can go down entirely at any time. The service itself stays up (the other platforms are unaffected), but `/rss/x/*` returns an upstream error.
- Poll at a low rate, honoring `<ttl>40</ttl>` (the server already caches for 40 minutes); aggressive polling gets rate-limited by the instances.
- Public accounts only; protected accounts and login-only content cannot be fetched.

---

## <img src="assets/youtube.png" width="28" height="28" alt="YouTube"> YouTube

Official site <https://youtube.com> · Data source: the channel upload feed (`/feeds/videos.xml`; anonymous, no API key, no account)

| Content | RSS | JSON | Default | Max | Cache |
|---|---|---|---|---|---|
| Channel uploads | `/rss/youtube/{ref}` | `/api/v1/youtube?ref=` | 15 | 15 | 10 minutes |

### Channel uploads

- `{ref}` in the RSS path is a channel id (24 characters) or a handle (`@` optional); a pasted channel URL carries slashes and only works through the JSON endpoint's `ref=` parameter, where `/videos`, `/shorts`, `/streams` tails and query strings are ignored.
- Turning a handle into an id costs one channel-page read (about 1.4 MB) and is cached for 24 hours; ids cost no extra request.
- The feed carries exactly 15 entries and has no pagination. A channel that uploads more than that between two polls loses the overflow; poll every 10 minutes or accept the ceiling.
- Entries are the upload stream, so Shorts and livestream replays are mixed in with regular videos. `link`/`guid` is always `https://www.youtube.com/watch?v=<id>`, which resolves for both.
- Fields: title, description, thumbnail (`hqdefault`, 480×360), publish time, view count. Duration is not in the feed and is not published.
- One feed read is up to 3 attempts (2 s, then 4 s apart). When all attempts fail, the client switches to the web client's browse endpoint, which returns up to 30 entries with relative timestamps ("5 days ago") and no description; those items carry approximate dates and repeat the title as their text. If that also fails, the last good list is served with `X-Feedmesh-Degraded: 1` for up to 24 hours.

### Limitations

- **The endpoint is not always available**: it answers 404 or 500 for short windows even for channels that exist (RSS-Bridge #2113, newsboat #3269, Google's developer forum). The service retries, then falls back to the web client endpoint, then to the last good list, so readers normally see a feed; only when both sources fail and nothing is cached does the route return 504.
- The fallback's dates are approximate (day-level relative text) and its entries have no description, so a feed built during an outage looks thinner than one built from the Atom feed. The next successful Atom read replaces it.
- Public channels only: members-only and private uploads never appear.
- Metadata only: no duration, resolution or live status.

---

## Adding a platform

1. Create `internal/<platform>/client.go`; it only handles HTTP and parsing and returns the neutral `Article` struct.
2. Add a field to `feed.Clients`, implement `feed.Get<Platform>()`, and map the result into `feed.Item` (HTML, Images, CreatedAt, URL).
3. Register both the RSS and the JSON route in `httpapi/server.go`.
4. Pin responses with `httptest`, covering parsing, sorting/filtering, cache hits, and empty responses.
5. Update this file, [API.md](API.md), and the platform table in the README, and drop the platform icon into `docs/assets/`.

## Trademarks

The platform icons are trademarks of their respective owners, used solely to identify the data source; they do not imply any affiliation or endorsement.

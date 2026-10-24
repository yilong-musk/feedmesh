# Supported platforms

**4 platforms and 6 content types** are currently wired up. Each platform below lists the content it supports and the matching endpoints; adding a platform only takes one provider — see the end of this page.

| Platform | Content | RSS | JSON |
|---|---|---|---|
| <img src="assets/weibo.png" width="20" height="20" alt="Weibo"> **Weibo** | Account timeline | `/rss/user/{uid}`<br>`/rss/domain/{name}` | `/api/v1/feed?uid=` |
| <img src="assets/weibo.png" width="20" height="20" alt="Weibo"> **Weibo** | Realtime hot search | `/rss/hot` | `/api/v1/hot` |
| <img src="assets/sspai.png" width="20" height="20" alt="sspai"> **sspai** | Latest articles | `/rss/sspai` | `/api/v1/sspai` |
| <img src="assets/v2ex.png" width="20" height="20" alt="V2EX"> **V2EX** | Latest topics | `/rss/v2ex` | `/api/v1/v2ex?section=latest` |
| <img src="assets/v2ex.png" width="20" height="20" alt="V2EX"> **V2EX** | Hot topics | `/rss/v2ex/hot` | `/api/v1/v2ex?section=hot` |
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

## Adding a platform

1. Create `internal/<platform>/client.go`; it only handles HTTP and parsing and returns the neutral `Article` struct.
2. Add a field to `feed.Clients`, implement `feed.Get<Platform>()`, and map the result into `feed.Item` (HTML, Images, CreatedAt, URL).
3. Register both the RSS and the JSON route in `httpapi/server.go`.
4. Pin responses with `httptest`, covering parsing, sorting/filtering, cache hits, and empty responses.
5. Update this file, [API.md](API.md), and the platform table in the README, and drop the platform icon into `docs/assets/`.

## Trademarks

The platform icons are trademarks of their respective owners, used solely to identify the data source; they do not imply any affiliation or endorsement.

# API

所有接口都是 `GET`，默认只监听 `127.0.0.1:3000`。RSS 响应的 `Content-Type` 为 `application/rss+xml; charset=utf-8`，JSON 响应为 `application/json`。

设置 `--token <token>` 或 `FEEDMESH_TOKEN` 后，所有请求必须带 `Authorization: Bearer <token>`，否则返回 401。设置 `--rate-limit <n>` 或 `FEEDMESH_RATE_LIMIT` 后，每个远端 IP 在任意滚动一分钟内最多 n 次请求，超出返回 429。若监听非 loopback 地址且未设置限额，服务默认启用 120 次/分钟/IP。反向代理场景下 `RemoteAddr` 通常是代理 IP，建议在代理层也做限流。

平台与路由的对应关系见 [PLATFORMS.md](PLATFORMS.md)。

---

## 通用参数

| 参数 | 说明 |
|---|---|
| `limit` | 返回条数；各平台默认值与上限见 [PLATFORMS.md](PLATFORMS.md) |
| `refresh=1` | 跳过旧值缓存、强制回源，并覆盖写回缓存（保留已有缓存宽度） |
| `full=1` | 仅少数派：抓取正文（默认关闭） |
| `section=latest\|hot` | 仅 `/api/v1/v2ex`：选择最新或最热 |
| `handle` | 仅 `/api/v1/x`：X 用户名（可带 `@`，1–15 位字母数字下划线） |

路径后缀 `.xml` 与不带后缀等价仅适用于 RSS 路由（如 `/rss/hot.xml`）；`/api/v1/*` 不接受 `.xml` 后缀。

---

## 健康检查

### `GET /healthz`

```json
{"status":"ok","version":"16ae5de","uptime_s":8,
 "cache":{"entries":5,"bytes":29433},
 "breaker":{"open":false,"until":null}}
```

`?probe=1` 额外做一次真实上游探活（结果 30 秒内复用，避免把上游打爆）：

```bash
curl -s 'http://127.0.0.1:3000/healthz?probe=1' | jq -c '.probe'
```

> `status` 只反映进程健康；依赖是否可用看 `probe` 与 `breaker`，两者分开才不会在上游挂掉时误报「一切正常」。

---

## RSS 接口

### `GET /rss/user/{uid}`

微博账号时间线。`/rss/user/{uid}.xml` 等价。

```bash
curl -s http://127.0.0.1:3000/rss/user/2803301701 | head -8
```

### `GET /rss/domain/{name}`

个性域名（如 `rmrb`）先解析成 uid，再返回同一格式。

### `GET /rss/hot`

微博实时热搜。`limit` 1–50（默认 20），只含正榜名次 1–50。

```bash
curl -s 'http://127.0.0.1:3000/rss/hot?limit=10'
curl -s 'http://127.0.0.1:3000/rss/hot?refresh=1'   # 绕过 5 分钟缓存
```

### `GET /rss/sspai`

少数派最新文章。默认摘要 + 封面；`full=1` 抓正文。

```bash
curl -s 'http://127.0.0.1:3000/rss/sspai?limit=10'
curl -s 'http://127.0.0.1:3000/rss/sspai?limit=5&full=1'
```

### `GET /rss/v2ex`

V2EX 官方 Atom 的最新主题（完整正文）。

```bash
curl -s 'http://127.0.0.1:3000/rss/v2ex?limit=20'
```

### `GET /rss/v2ex/hot`

V2EX 最热：过滤「推广」节点、按回复数降序；页面异常时回退官方 API。

```bash
curl -s 'http://127.0.0.1:3000/rss/v2ex/hot?limit=20'
```

### `GET /rss/x/{handle}`

X（Twitter）账号时间线，经公共 Nitter 实例获取；`{handle}` 可带 `@`。

```bash
curl -s 'http://127.0.0.1:3000/rss/x/elonmusk?limit=20'
```

- 默认 20 条，上限 20（一页大小）；缓存 40 分钟，`Cache-Control: max-age=2400`。
- 条目 `link`/`guid` 一律是 `https://x.com/{handle}/status/{id}`；正文内的实例链接也会改写。
- 转发标题前有 `🔁`；图片走 Media RSS。
- 所有实例都失败时返回 `UPSTREAM_UNAVAILABLE`（504）。

---

## JSON 接口

时间一律 RFC3339（UTC）；`images` 恒为数组（无图时 `[]`，不是 `null`）。

### `GET /api/v1/feed?uid=<uid>&limit=N`

```json
{"uid":"2803301701","screen_name":"人民日报","description":"…",
 "fetched_at":"2026-10-23T06:55:12Z","cached":true,
 "items":[{"id":"…","bid":"…","created_at":"2026-10-23T06:45:32Z",
           "text":"…","html":"…","url":"https://weibo.com/2803301701/RnZRCA7Kf",
           "images":[],"video_url":"…","page_url":"…"}]}
```

`video_url` / `page_url` 有值时才出现（`omitempty`）。

### `GET /api/v1/hot?limit=N`

```json
{"title":"微博热搜","description":"微博实时热搜榜","fetched_at":"2026-10-23T08:00:00Z",
 "cached":true,
 "items":[{"rank":1,"title":"1. 小城良方 剧情硬伤","text":"小城良方 剧情硬伤",
           "url":"https://s.weibo.com/weibo?q=…","html":"<p>热度 …</p>"}]}
```

### `GET /api/v1/sspai?limit=N&full=1`

```json
{"title":"少数派","description":"少数派最新文章","fetched_at":"…","cached":true,
 "items":[{"id":"117169","title":"…","text":"摘要","url":"https://sspai.com/post/117169",
           "html":"…","images":["https://rssfile.sspai.com/…png"],"created_at":"…"}]}
```

### `GET /api/v1/v2ex?section=latest|hot&limit=N`

```json
{"title":"V2EX 最热","description":"V2EX 最热主题（按回复数排序）","fetched_at":"…","cached":true,
 "items":[{"id":"1251126","title":"…","text":"…","url":"https://www.v2ex.com/t/1251126",
           "html":"…","images":[],"created_at":"…"}]}
```

### `GET /api/v1/x?handle=<handle>&limit=N`

```json
{"handle":"elonmusk","title":"elonmusk / X","description":"@elonmusk 的最新推文（经 Nitter 实例）",
 "fetched_at":"…","cached":true,
 "items":[{"id":"2113508310267674684","title":"Try Grok @Bot","text":"…",
           "url":"https://x.com/elonmusk/status/2113508310267674684","html":"…",
           "images":["https://pbs.twimg.com/media/…jpg"],"created_at":"2026-10-23T05:50:22Z"}]}
```

- `handle` 必填（可带 `@`）；`limit` 默认 20、上限 20。
- 支持 `refresh=1`，行为与 RSS 路由一致。

---

## 错误

统一信封、对应 HTTP 状态码，并返回 `X-Feedmesh-Code`（兼容旧名 `X-Weibo-RSS-Code`）：

```json
{"error":{"code":"USER_NOT_FOUND","message":"weibo user 999 not found: 这里还没有内容"}}
```

| code | HTTP |
|---|---|
| `BAD_REQUEST` | 400（方法不对为 405，带 `Allow` 头） |
| `UNAUTHORIZED` | 401（启用 `--token` 后缺少/错误 Bearer token） |
| `USER_NOT_FOUND` / `LOGIN_REQUIRED` | 404 |
| `UPSTREAM_BLOCKED` | 502 |
| `BREAKER_OPEN` | 503 |
| `NETWORK` / `TIMEOUT` / `UPSTREAM_UNAVAILABLE` | 504 |
| `RATE_LIMITED` | 429（带 `Retry-After`） |

---

## RSS 输出细节

- `description`：CDATA 包裹的 HTML。统一包含正文、配图（`<img>`，带 `width`/`height`、`loading="lazy"`）、转发引用块 `<blockquote>`、链接卡片（封面 + 标题 + 摘要 + 跳转）。
- `content:encoded`：与 `description` 同一份 HTML，供优先读全文命名空间的阅读器（Inoreader / Feedly / NetNewsWire）。
- Media RSS：首图 `<media:thumbnail>`，每张图 `<media:content medium="image">`；微博视频额外输出 `<media:content medium="video">` + `<enclosure type="video/mp4" length="0">`。
- `link` / `guid`：微博为 `https://weibo.com/<uid>/<bid>`（缺 bid 退到 `m.weibo.cn/detail/<id>`）；热搜用搜索 URL；少数派/V2EX 用原站 permalink。
- `pubDate`：RFC822。
- 转义覆盖 `& < > " '`；CDATA 内的 `]]>` 会被安全拆分，不会提前闭合。
- channel 的 `<title>` / `<link>` / `<description>` 恒存在（RSS 2.0 要求）。

### 已知限制

- 微博视频直链是**签名 URL，约 1 小时过期**（`Expires=`），长期归档请用 `page_url`；直链要求 `Referer: https://weibo.com/`，不带该头的播放器会收到 403。
- 少数派 `full=1` 依赖上游详情接口，单篇 3–20 秒；会员文章只返回摘要。
- V2EX 最热来自页面解析，结构变化时依赖官方 API 兜底。

---

## 缓存行为（API 视角）

| 平台 | 缓存 key | TTL |
|---|---|---|
| 微博时间线 | `timeline:<uid>` | 5 分钟 |
| 微博资料 | `profile:<uid>` | 1 小时 |
| 微博长文 | `longtext:<uid>:<id>` | 24 小时 |
| 微博域名 | `domain:<name>` | 24 小时 |
| 微博热搜 | `hot:top` | 5 分钟 |
| 少数派列表 | `sspai:list` | 10 分钟 |
| 少数派正文 | `sspai:detail:<id>` | 24 小时 |
| V2EX 最新/最热 | `v2ex:latest` / `v2ex:hot` | 10 分钟 |
| X 时间线 | `x:<handle>` | 40 分钟 |

- `refresh=1` 跳过读取旧值并覆盖写回；为保持缓存宽度，会读取旧条目的宽度。RSS 和 JSON 接口都支持。
- 缓存条目按请求的 `limit` 记录宽度，普通请求和 `refresh=1` 都不会用较小 `limit` 截断更宽的缓存（按需回源加宽）。
- 缓存目录：`--cache-dir` / `FEEDMESH_CACHE`，默认 `~/.cache/feedmesh`；一个 key 一个 JSON 文件，原子写。目录权限 0700，文件权限 0600；启动时会清理过期条目。`cache purge --all` 只删除能验证为 feedmesh 写入的条目（校验条目内的 `key` 与文件名），不会碰同目录下其他 `.json` 文件。

实现细节（并发、熔断、重试）见 [ARCHITECTURE.md](ARCHITECTURE.md)。

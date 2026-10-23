# 支持的平台

当前接入 **4 个平台、6 类内容**。每个平台下列出它支持的内容与对应接口；新增平台只需加一个 provider，见文末。

| 平台 | 支持的内容 | RSS | JSON |
|---|---|---|---|
| <img src="assets/weibo.png" width="20" height="20" alt="微博"> **微博** | 账号时间线 | `/rss/user/{uid}`<br>`/rss/domain/{name}` | `/api/v1/feed?uid=` |
| <img src="assets/weibo.png" width="20" height="20" alt="微博"> **微博** | 实时热搜 | `/rss/hot` | `/api/v1/hot` |
| <img src="assets/sspai.png" width="20" height="20" alt="少数派"> **少数派** | 最新文章 | `/rss/sspai` | `/api/v1/sspai` |
| <img src="assets/v2ex.png" width="20" height="20" alt="V2EX"> **V2EX** | 最新主题 | `/rss/v2ex` | `/api/v1/v2ex?section=latest` |
| <img src="assets/v2ex.png" width="20" height="20" alt="V2EX"> **V2EX** | 最热主题 | `/rss/v2ex/hot` | `/api/v1/v2ex?section=hot` |
| <img src="assets/x.png" width="20" height="20" alt="X"> **X** | 账号时间线 | `/rss/x/{handle}` | `/api/v1/x?handle=` |

通用参数：`limit`、`refresh=1`（跳过读缓存、强制回源）；`.xml` 后缀与不带后缀等价。完整契约见 [API.md](API.md)。

---

## <img src="assets/weibo.png" width="28" height="28" alt="微博"> 微博

官网 <https://weibo.com> · 数据源 `m.weibo.cn`（visitor cookie，匿名）

| 内容 | RSS | JSON | 默认 | 上限 | 缓存 |
|---|---|---|---|---|---|
| 账号时间线 | `/rss/user/{uid}`、`/rss/domain/{name}` | `/api/v1/feed?uid=` | 10 | 10 | 5 分钟 |
| 实时热搜 | `/rss/hot`、`/rss/hot.xml` | `/api/v1/hot` | 20 | 50 | 5 分钟 |

### 账号时间线

- 参数：`limit` 1–10、`refresh=1`；`domain` 会先解析成 uid。
- 输出：正文 HTML、配图（大图 + 真实宽高）、转发引用块、长文全文、视频 enclosure、外链卡片。
- 上游：`container/getIndex`，单页最多 10 条；长文另走 `statuses/extend`（缓存 24 小时）。

### 实时热搜

- 规则：只保留 `realpos` 1–50 的正榜；过滤无 `realpos` 的「荐」推荐位；置顶 `hotgov` 不并入。
- 名次用 `realpos` 而不是数组下标，推荐位不会让名次错位。
- `guid` 用搜索 URL，名次变化不会让阅读器重复推送。

### 限制

- 视频直链是**签名 URL，约 1 小时过期**，且要求 `Referer: https://weibo.com/`；长期归档请用 `page_url`。
- 时间线接口一页最多 10 条，无法通过 `limit` 翻页。

---

## <img src="assets/sspai.png" width="28" height="28" alt="少数派"> 少数派

官网 <https://sspai.com> · 数据源 sspai.com JSON API（官方 RSS 只有摘要）

| 内容 | RSS | JSON | 默认 | 上限 | 缓存 |
|---|---|---|---|---|---|
| 最新文章 | `/rss/sspai`、`/rss/sspai.xml` | `/api/v1/sspai` | 10 | 30 | 列表 10 分钟 · 正文 24 小时 |

- 参数：`limit` 1–30、`full=1`（抓正文）、`refresh=1`。
- 默认输出封面 + 作者/关键词 + 摘要；`full=1` 时抓正文并缓存 24 小时。
- 上游较慢：列表 3–7 秒、正文单篇 3–20 秒，因此 `full` 默认关闭。
- 会员文章只返回摘要，标注「会员文章，仅显示摘要」，不绕过付费墙。

---

## <img src="assets/v2ex.png" width="28" height="28" alt="V2EX"> V2EX

官网 <https://www.v2ex.com> · 数据源：官方 Atom + 最热页面解析

| 内容 | RSS | JSON | 默认 | 上限 | 缓存 |
|---|---|---|---|---|---|
| 最新主题 | `/rss/v2ex` | `/api/v1/v2ex?section=latest` | 20 | 50 | 10 分钟 |
| 最热主题 | `/rss/v2ex/hot` | `/api/v1/v2ex?section=hot` | 20 | 50 | 10 分钟 |

### 最新主题

- 官方 Atom `https://www.v2ex.com/index.xml`，50 条，含完整正文、作者、发布时间。
- 参数：`limit` 1–50、`refresh=1`。

### 最热主题

- 官方**没有**最热 feed：解析 `https://www.v2ex.com/?tab=hot`，过滤「推广」节点、按回复数降序。
- 降级：页面解析不足 5 条时回退 `api/topics/hot.json`（10 条），两条都失败才报错。
- 参数：`limit` 1–50、`refresh=1`。

---

## <img src="assets/x.png" width="28" height="28" alt="X"> X（Twitter）

官网 <https://x.com> · 数据源：公共 Nitter 实例的 RSS（匿名，无账号、无 API key）

| 内容 | RSS | JSON | 默认 | 上限 | 缓存 |
|---|---|---|---|---|---|
| 账号时间线 | `/rss/x/{handle}` | `/api/v1/x?handle=` | 20 | 20 | 40 分钟 |

### 账号时间线

- 实例池从 `https://status.d420.de/api/v1/instances` 拉取（筛 `rss=true`、`points ≥ 60`、非 bad host），30 分钟内复用；目录不可用时回退内置列表（kareem.one、meowing.monster、netbub）。
- 按得分顺序逐个尝试；某个实例超时、5xx、返回 0 条都会换下一个。全部失败返回 `UPSTREAM_UNAVAILABLE`。
- 部分实例有 Anubis 人机门（`anubis_challenge` / `preact_info`）：客户端自动解 SHA-256 PoW 并缓存 cookie 后重试，不需要浏览器。
- 正文里的实例链接会改写成 `https://x.com/...`，图片保留 `pbs.twimg.com`，不暴露实际命中的镜像。
- 转发条目在标题前标 `🔁`，并去掉 Nitter 原始的 `RT by @user:` 前缀。
- 参数：`limit` 1–20、`refresh=1`。

### 限制

- **依赖第三方公益实例**：池子只有个位数健康节点，随时可能整池失效。服务本身不会挂（其余平台不受影响），但 `/rss/x/*` 会返回上游错误。
- 请按 `<ttl>40</ttl>` 低频轮询（服务端已缓存 40 分钟）；高频抓取会被实例限流。
- 只覆盖公开账号；受保护账号、仅登录可见内容无法获取。

---

## 增加一个平台

1. 新建 `internal/<platform>/client.go`，只负责 HTTP 与解析，返回中立的 `Article` 结构。
2. 在 `feed.Clients` 里加字段，实现 `feed.Get<Platform>()`，映射成 `feed.Item`（HTML、Images、CreatedAt、URL）。
3. 在 `httpapi/server.go` 注册 RSS 与 JSON 两个路由。
4. 用 `httptest` 固定响应，覆盖解析、排序/过滤、缓存命中、空响应。
5. 更新本文件、[API.md](API.md) 与 README 的平台表，并把平台图标放进 `docs/assets/`。

## 商标说明

平台图标为各自权利人的商标，仅用于标识数据来源，不代表任何隶属或背书关系。

# 支持的平台

feedmesh 把每个来源统一成 RSS 2.0 与 JSON 两种视图；新增平台只需在 `internal/` 下加一个 provider（见文末）。

| 平台 | 内容 | RSS | JSON | 默认条数 | 上限 | 缓存 |
|---|---|---|---|---|---|---|
| 微博 | 账号时间线 | `/rss/user/{uid}`、`/rss/domain/{name}` | `/api/v1/feed?uid=` | 10 | 10 | 5 分钟 |
| 微博 | 实时热搜 | `/rss/hot` | `/api/v1/hot` | 20 | 50 | 5 分钟 |
| 少数派 | 最新文章 | `/rss/sspai` | `/api/v1/sspai` | 10 | 30 | 10 分钟（正文 24 小时） |
| V2EX | 最新主题 | `/rss/v2ex` | `/api/v1/v2ex?section=latest` | 20 | 50 | 10 分钟 |
| V2EX | 最热主题 | `/rss/v2ex/hot` | `/api/v1/v2ex?section=hot` | 20 | 50 | 10 分钟 |

通用参数：`limit`、`refresh=1`（绕过读缓存、强制回源）。具体见 [API.md](API.md)。

---

## 微博 · 账号时间线

- 路由：`/rss/user/{uid}`（等于 `/rss/user/{uid}.xml`）、`/rss/domain/{name}`（个性域名先解析成 uid）
- 数据源：`m.weibo.cn` 的 `container/getIndex`（visitor cookie，匿名）
- 输出：正文 HTML、配图（`large` 大图 + 真实宽高）、转发引用块、长文全文、视频 enclosure、外链卡片
- 参数：`limit` 1–10、`refresh=1`
- 注意：列表接口单页最多 10 条；视频直链是签名 URL（约 1 小时过期）且要求 `Referer: https://weibo.com/`

## 微博 · 实时热搜

- 路由：`/rss/hot`、`/rss/hot.xml`
- 数据源：`weibo.com/ajax/side/hotSearch`（visitor cookie + weibo.com Referer）
- 规则：只保留 `realpos` 1–50 的正榜；过滤无 `realpos` 的「荐」推荐位；置顶 `hotgov` 不并入
- 参数：`limit` 1–50（默认 20）、`refresh=1`
- `guid` 用搜索 URL，名次变化不会让阅读器重复推送

## 少数派

- 路由：`/rss/sspai`、`/rss/sspai.xml`
- 数据源：sspai.com JSON API（官方 RSS 只有摘要）
- 输出：封面 + 作者/关键词 + 摘要 + 阅读全文；`full=1` 时抓正文并缓存 24 小时
- 参数：`limit` 1–30（默认 10）、`full=1`、`refresh=1`
- 注意：上游较慢（列表 3–7 秒、正文单篇 3–20 秒），`full` 默认关闭；会员文章只给摘要

## V2EX · 最新

- 路由：`/rss/v2ex`
- 数据源：官方 Atom `https://www.v2ex.com/index.xml`（50 条，含完整正文、作者、发布时间）
- 参数：`limit` 1–50（默认 20）、`refresh=1`

## V2EX · 最热

- 路由：`/rss/v2ex/hot`
- 数据源：**官方没有最热 feed**，解析 `https://www.v2ex.com/?tab=hot`，过滤「推广」节点、按回复数降序
- 降级：页面解析不足 5 条时回退 `api/topics/hot.json`（10 条）
- 参数：`limit` 1–50（默认 20）、`refresh=1`

---

## 增加一个平台

1. 新建 `internal/<platform>/client.go`，只负责 HTTP 与解析，返回中立的 `Article` 结构。
2. 在 `feed.Clients` 里加字段，实现 `feed.Get<Platform>()`，把结果映射成 `feed.Item`（HTML、Images、CreatedAt、URL）。
3. 在 `httpapi/server.go` 注册 RSS 与 JSON 两个路由。
4. 用 `httptest` 提供固定响应，覆盖解析、排序/过滤、缓存命中、空响应四条路径。
5. 更新本文件与 [API.md](API.md)。

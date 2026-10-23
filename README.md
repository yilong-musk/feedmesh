# weibo-rss-pro

把指定微博账号的时间线，以及微博实时热搜榜，转成 RSS。单二进制、零第三方依赖、对 agent 友好（stdout 只放数据、机器可读退出码、`--json` 一行 JSON）。

Go 重写自 [zgq354/weibo-rss](https://github.com/zgq354/weibo-rss)（Node/TS + Koa + LevelDB）。原版被 `leveldown` 这个原生模块锁死在 Node 20；本实现只有标准库，`go.mod` 里没有任何 `require`。

| | 原 Node 版 | 本实现 |
|---|---|---|
| 运行时 | Node 20 + 原生模块 | 无（`CGO_ENABLED=0` 静态二进制） |
| 常驻内存（实测 RSS） | ~70 MB | 稳态 ~15 MB（刚启动 8.8 MB） |
| 存储 | LevelDB | 一个 key 一个 JSON 文件 |
| 依赖 | Koa、leveldown 等 | 仅 Go 标准库 |

---

## 安装

需要 Go 1.27+（本机路径 `/home/ubuntu/.hermes/tools/go-1.27.3-linux-x64/bin/go`）。

```bash
export PATH=/home/ubuntu/.hermes/tools/go-1.27.3-linux-x64/bin:$PATH
cd /home/ubuntu/weibo-rss-pro

make install          # CGO_ENABLED=0 + -trimpath + -s -w，装到 ~/.local/bin
```

自建同样一行（`CGO_ENABLED=0` 不能省，否则会动态链接 glibc）：

```bash
CGO_ENABLED=0 go build -trimpath \
  -ldflags "-s -w -X main.version=$(git describe --tags --always 2>/dev/null || echo dev)" \
  -o weibo-rss-pro ./cmd/weibo-rss-pro
install -m 0755 weibo-rss-pro /home/ubuntu/.local/bin/weibo-rss-pro
```

验证（`file` 应显示 `statically linked`）：

```bash
file /home/ubuntu/.local/bin/weibo-rss-pro
weibo-rss-pro version            # weibo-rss-pro <git sha> (linux/amd64, go1.27.3)
weibo-rss-pro doctor             # 三项上游探活
```

---

## 输出纪律（所有子命令都遵守）

- **stdout 只放数据**：RSS XML，或 `--json` 时**一行**合法 JSON（`jq .` 可直接解析）。
- **stderr 只放日志**：每行一个 JSON 对象 `{"ts","level","msg",...}`；`--quiet` 全部关掉。
- 永不交互：不读 stdin、不弹确认、不需要 TTY、无颜色、无进度条、不分页。
- 配置优先级：**flag > 环境变量 > 默认值**。

```bash
weibo-rss-pro fetch 2803301701 --json 2>/dev/null | jq '.items[].url'
```

---

## 子命令

### 1. `serve` — 常驻 HTTP 服务

```bash
weibo-rss-pro serve --listen 127.0.0.1:3000 --cache-dir ~/.cache/weibo-rss-pro
```

启动后向 stdout 打一行并立即 flush，之后 stdout 保持静默（就绪信号，agent 阻塞读这一行即可，不用 `sleep` 猜）：

```json
{"event":"ready","addr":"127.0.0.1:3000","version":"16ae5de"}
```

收到 SIGTERM/SIGINT 时优雅关闭（10 秒超时）。

### 2. `fetch` — 抓一个账号

```bash
weibo-rss-pro fetch 2803301701                    # RSS XML 到 stdout
weibo-rss-pro fetch 2803301701 --json             # 一行 JSON
weibo-rss-pro fetch 2803301701 --json --limit 5   # 只要 5 条
weibo-rss-pro fetch rmrb                          # 个性域名（自动解析成 uid）
weibo-rss-pro fetch 2803301701 --refresh          # 跳缓存，强制回源
weibo-rss-pro fetch 2803301701 -o /tmp/rmrb.xml   # 写文件；`-o -` 等于 stdout
```

`--limit` 取值 1–10：微博 `getIndex` 单页最多 10 条，超出部分会被截到 10。

JSON 形态：

```json
{"ok":true,"uid":"2803301701","screen_name":"人民日报","description":"…",
 "fetched_at":"2026-10-23T06:55:12Z","cached":false,"count":10,
 "items":[{"id":"5357193528609583","bid":"RnZRCA7Kf","created_at":"2026-10-23T06:45:32Z",
           "text":"纯文本正文","html":"<p>HTML 正文</p>","url":"https://weibo.com/2803301701/RnZRCA7Kf",
           "images":[]}]}
```

### 3. `doctor` — 上游探活

依次探 `genvisitor`（visitor cookie）、`getIndex`（用户资料）、`statuses`（正文接口），逐项给耗时与 HTTP 状态：

```bash
weibo-rss-pro doctor --json
```

```json
{"ok":true,"steps":[
  {"step":"genvisitor","ok":true,"ms":582,"http_status":200},
  {"step":"getIndex","ok":true,"ms":802,"http_status":200},
  {"step":"statuses","ok":true,"ms":633,"http_status":200}]}
```

`--uid <uid>` 可指定探活用的账号（默认 `2803301701`，人民日报）。任一项失败时退出码按该项错误类别返回（如 4 / 3），JSON 里 `ok:false` 且该步带 `code`/`error`。

### 4. `cache` — 缓存管理

```bash
weibo-rss-pro cache stat
# dir=/home/ubuntu/.cache/weibo-rss-pro entries=5 bytes=29433

weibo-rss-pro cache purge --all --dry-run --json
# {"dry_run":true,"scope":"all","deleted":0,"entries":5,"bytes":29433}

weibo-rss-pro cache purge --uid 2803301701 --dry-run     # 只看影响面，不删
weibo-rss-pro cache purge --uid 2803301701 --json        # {"deleted":2,"entries":2,...}
weibo-rss-pro cache purge --all
```

**`--dry-run` 只统计、不落盘**——输出将删除的条目数与字节数，`deleted` 恒为 0。写操作/重活一律先给 dry-run。

`--uid` 只清该账号的 `feed` / `profile` / `timeline` 三类条目（精确匹配，不会误删 uid 前缀相同的其他账号）。`longtext` 条目按微博 id 存储、无法归属到账号，只能由 `--all` 清理（它们 24 小时后自行过期）。

### 5. `version`

```bash
weibo-rss-pro version          # weibo-rss-pro 16ae5de (linux/amd64, go1.27.3)
weibo-rss-pro version --json   # {"version":"16ae5de","go":"go1.27.3","os":"linux","arch":"amd64"}
```

### 6. `help`（等同 `-h`）

```bash
weibo-rss-pro help    # 退出码 0
```

---

## 全局 flag

| flag | 说明 | 环境变量 |
|---|---|---|
| `--json` | stdout 输出**一行** JSON（失败也走 JSON 信封） | — |
| `--quiet`, `-q` | 关掉 stderr 日志，只留数据流 | — |
| `--timeout <dur>` | 单个上游请求超时，默认 `9s`（海外机 3s 会 ETIMEDOUT） | — |
| `--cache-dir <dir>` | 缓存目录 | `WEIBO_RSS_PRO_CACHE`（旧名 `WEIBO_RSS_CACHE` 仍兼容） |
| `--listen <addr>` | `serve` 监听地址，默认 `127.0.0.1:3000` | `WEIBO_RSS_PRO_LISTEN`（旧名 `WEIBO_RSS_LISTEN` 仍兼容） |
| `-h`, `--help` | 帮助，退出码 0 | — |

flag 可以写在子命令前后任意位置。

---

## 退出码

| 码 | 含义 |
|---|---|
| `0` | 成功 |
| `2` | 用法错误（未知命令/flag、缺参数、uid 格式非法） |
| `3` | 网络错误或超时（含上游 5xx） |
| `4` | 上游封禁（HTTP 418 / 403） |
| `5` | 用户不存在，或仅登录可见 |
| `6` | 熔断中（刚被封过，冷却 10 分钟） |
| `10` | 其他（本地 IO、磁盘、未知） |

`--json` 时失败输出固定信封：

```json
{"ok":false,"error":{"code":"USER_NOT_FOUND","message":"weibo user 9999999999999999 not found: 这里还没有内容"}}
```

`code` 取值：`BAD_REQUEST` / `NETWORK` / `TIMEOUT` / `UPSTREAM_BLOCKED` / `UPSTREAM_UNAVAILABLE` / `USER_NOT_FOUND` / `LOGIN_REQUIRED` / `BREAKER_OPEN` / `INTERNAL`。

---

## HTTP 契约

全部 `GET`，仅监听 `127.0.0.1`。

### `GET /healthz`

```json
{"status":"ok","version":"16ae5de","uptime_s":8,
 "cache":{"entries":5,"bytes":29433},
 "breaker":{"open":false,"until":null}}
```

`?probe=1` 额外做一次真上游探活（结果在 30 秒内复用，避免被健康检查打爆上游）：

```bash
curl -s 'http://127.0.0.1:3000/healthz?probe=1' | jq -c '.probe'
```

> `status` 反映进程健康；依赖是否可用看 `probe` 与 `breaker`——两者分开，才不会在上游挂掉时误报「一切正常」。

### `GET /rss/user/{uid}` 与 `/rss/user/{uid}.xml`

RSS 2.0，`Content-Type: application/rss+xml; charset=utf-8`，`<ttl>15</ttl>`。两个路径等价，后者方便只认 `.xml` 后缀的阅读器。

```bash
curl -s http://127.0.0.1:3000/rss/user/2803301701 | head -8
```

### `GET /rss/domain/{name}`

个性域名（如 `rmrb`）先解析成 uid，再返回同一格式。

### `GET /rss/hot` 与 `/rss/hot.xml`

实时热搜榜，一题一条 item（`title` 带名次，`guid` 用搜索 URL，名次变化不会重复推送）。

```bash
curl -s 'http://127.0.0.1:3000/rss/hot?limit=10'
curl -s 'http://127.0.0.1:3000/rss/hot?refresh=1'   # 绕过 5 分钟缓存
```

- `limit`：1–50，默认 20。
- 每条 `description` 含「热度 148.9 万 · 标签 · 分类」与跳转链接。
- 上游缓存 5 分钟，RSS `<ttl>5</ttl>`；HTTP `Cache-Control: max-age=60`。

### `GET /api/v1/hot?limit=N`

```json
{"title":"微博热搜","description":"微博实时热搜榜","fetched_at":"2026-10-23T08:00:00Z",
 "cached":true,
 "items":[{"rank":1,"title":"1. 小城良方 剧情硬伤","text":"小城良方 剧情硬伤",
           "url":"https://s.weibo.com/weibo?q=…","html":"<p>热度 …</p>"}]}
```

### `GET /api/v1/feed?uid=<uid>&limit=N`

```json
{"uid":"2803301701","screen_name":"人民日报","description":"…",
 "fetched_at":"2026-10-23T06:55:12Z","cached":true,
 "items":[{"id":"…","bid":"…","created_at":"2026-10-23T06:45:32Z",
           "text":"…","html":"…","url":"https://weibo.com/2803301701/RnZRCA7Kf","images":[]}]}
```

字段名稳定、无缩写；时间一律 RFC3339（UTC）；`images` 恒为数组（无图时 `[]`，不是 `null`）。

### 错误

统一信封 + 对应 HTTP 状态码 + 响应头 `X-Weibo-RSS-Code`：

```json
{"error":{"code":"USER_NOT_FOUND","message":"weibo user 999 not found: 这里还没有内容"}}
```

| code | HTTP |
|---|---|
| `BAD_REQUEST` | 400（方法不对则为 405，并带 `Allow` 头） |
| `USER_NOT_FOUND` / `LOGIN_REQUIRED` | 404 |
| `UPSTREAM_BLOCKED` | 502 |
| `BREAKER_OPEN` | 503 |
| `NETWORK` / `TIMEOUT` / `UPSTREAM_UNAVAILABLE` | 504 |

---

## RSS 细节

- `title`：优先微博自己的 `status_title`，否则纯文本前 25 字（超长截断加 `…`）。
- `description`：HTML（CDATA），保留话题链接、配图（`<img>`，取 `large` 大图；带 weibo 返回的 `width`/`height` 与 `loading="lazy"`）与转发引用块 `<blockquote>`。
- `content:encoded`：与 `description` 同一份 HTML，供优先读全文命名空间的阅读器使用（Inoreader、Feedly、NetNewsWire 等）。
- 配图同时输出 Media RSS：`<media:thumbnail>`（首图）+ 每张 `<media:content medium="image">`。
- 视频卡片：解析 `page_info.media_info`，取 `stream_url_hd`（回退 `stream_url`，实测是直链 MP4），输出
  `<enclosure type="video/mp4" length="0">`、`<media:content medium="video">`，并在正文里给封面、时长与「直接播放 MP4」链接。
  两条实测限制：① 直链是**签名 URL，约 1 小时过期**（`Expires=` 参数），适合当天阅读，长期归档请用 `page_url`；② 直链要求 `Referer: https://weibo.com/`，不带该头的播放器会收到 403。
- 链接卡片：`page_info` 的 `webpage` / `article` 等类型会展开成「封面 + 标题 + 摘要 + 跳转」，不再只剩一条 `t.cn` 短链；`search_topic`（话题卡）不重复渲染。
- `link` / `guid`：`https://weibo.com/<uid>/<bid>`；缺 bid 时退到 `https://m.weibo.cn/detail/<id>`。
- `pubDate`：RFC822（`Fri, 23 Oct 2026 06:45:32 +0000`）。
- 转义覆盖 `& < > " '`；CDATA 内容里出现 `]]>` 会被安全拆分为 `]]]]><![CDATA[>`，不会提前闭合。
- channel 的 `<title>/<link>/<description>` 三者恒存在：账号简介为空时输出 `<description></description>`（RSS 2.0 要求，严格校验器不认缺失）。
- 微博的表情图 `<span class="url-icon">` 统一转成其 `alt` 文字（`[哈哈]`）。

---

## 缓存语义

- 位置：`--cache-dir` / `WEIBO_RSS_PRO_CACHE`（兼容 `WEIBO_RSS_CACHE`），默认 `~/.cache/weibo-rss-pro`。
- 布局：**一个 key 一个 JSON 文件**，文件名由 key 净化而来（`profile:2803301701` → `profile_2803301701.json`）。
- 文件格式自带过期时间：

  ```json
  {"key":"profile:2803301701","stored_at":"2026-10-23T06:55:12Z",
   "expires_at":"2026-10-23T07:55:12Z","value":{…}}
  ```

- **原子写**：先写同目录临时文件 → `fsync` → `rename`。读方永远看不到半截文件；并发写不会互相截断。
- 过期条目在读取时判定为 miss，并顺手删除。损坏的条目（非 JSON）同样视为 miss，不会让请求失败。
- 有效期：

  | key | TTL | 说明 |
  |---|---|---|
  | `timeline:<uid>` | 5 分钟 | 时间线首页 |
  | `profile:<uid>` | 1 小时 | 昵称/简介/containerid |
  | `longtext:<id>` | 24 小时 | 长文全文 |
  | `domain:<name>` | 24 小时 | 个性域名 → uid |

- `--refresh` / `?refresh=1` 跳过读缓存并覆盖写回，**包括 `domain:<name>` 域名映射**（`weibo-rss-pro fetch rmrb --refresh` 会真的回源解析域名）。

### 上游调用策略

- **全局并发 1**（信号量）：任何时刻最多一个在途请求，避免被微博风控。
- **single-flight**：同 key 的并发请求合并成一次（HTTP 与 CLI 共享该逻辑）。
- **重试**：网络错误/5xx 重试 3 次，指数退避 300ms → 600ms → 1200ms。
- **熔断**：418/403 先刷新一次 visitor cookie 重试；仍被封则熔断 10 分钟，期间所有上游调用立即返回 `BREAKER_OPEN`（退出码 6），冷却结束自动半开一次。

---

## 部署（systemd user 单元）

```bash
install -m 0644 deploy/weibo-rss-pro.service ~/.config/systemd/user/weibo-rss-pro.service
systemctl --user daemon-reload
systemctl --user enable --now weibo-rss-pro

systemctl --user is-active weibo-rss-pro     # active
systemctl --user status weibo-rss-pro        # ready 行 + 日志
journalctl --user -u weibo-rss-pro -f
```

`deploy/weibo-rss-pro.service` 只做三件事：`Restart=always`、缓存目录走 `Environment`、`NoNewPrivileges`。不碰系统级单元、不碰 Caddy。

改动后重新部署：

```bash
cd /home/ubuntu/weibo-rss-pro && make install   # 或按「安装」一节手动 build + install
systemctl --user restart weibo-rss-pro
curl -s http://127.0.0.1:3000/healthz | jq -c '{status,version,uptime_s}'
```

> 服务只监听 `127.0.0.1`，**没有暴露公网**。要对外发布请自行加反向代理并自行评估风险。

---

## 排错

**微博改接口时，只需要改 `internal/weibo`。** 其余包（`feed` / `cache` / `httpapi` / `guard`）只消费已解析好的结构体，不碰任何微博 URL、请求头或 JSON 字段名。

诊断顺序：

```bash
weibo-rss-pro doctor --json          # 1. 哪一步坏了
weibo-rss-pro fetch 2803301701 --refresh --json   # 2. 单账号端到端
journalctl --user -u weibo-rss-pro -n 50          # 3. 服务侧日志（JSON 行）
```

| 症状 | 大概率原因 | 改哪里 |
|---|---|---|
| `doctor` 第一步 `genvisitor` 失败 | visitor 接口或 cookie 名（`SUB`/`SUBP`）变了 | `internal/weibo/client.go` 的 `fetchVisitor` |
| 退出码 4 / 日志出现 `upstream blocked` | 缺 cookie、UA 被识别、或真的被限流 | `DefaultUserAgent`、`send` 里的请求头；熔断冷却见 `breakerCooloff` |
| 退出码 5 但账号确实存在 | 判断「用户不存在」的字段变了（实测：状态码在**顶层** `ok`，不在 `data.ok`） | `internal/weibo/api.go` 的 `Profile` |
| 时间线为空但资料正常 | `containerid` 取法变了（取 `tabs[tab_type=="weibo"]`） | `Profile` 里的 tab 匹配 |
| 长文只抓到半截 | `statuses/extend` 字段改名 | `LongText` / `StatusText` |
| `created_at` 变成抓取时间 | 微博换了时间格式 | `internal/feed/service.go` 的 `createdLayouts` |
| 退出码 3 且日志有 `context deadline exceeded` | 海外机到微博慢 | 调大 `--timeout`（默认 9s） |
| 退出码 6 | 熔断中，10 分钟自动恢复 | 无需操作；或重启服务清状态 |

单测覆盖（改完先跑这个）：

```bash
go test ./...         # XML 转义、缓存 TTL/原子写、uid 校验、路由、熔断状态机、--refresh 语义
go test -race ./...   # 并发面（single-flight + 缓存双检）无数据竞争
go vet ./...
```

---

## 目录结构

```
cmd/weibo-rss-pro/         CLI 入口：flag 解析、六个子命令、退出码映射
internal/weibo/        唯一与微博 HTTP 打交道的地方（改接口只改这里）
internal/feed/         微博 → 统一模型 → RSS 2.0 渲染（纯函数，无网络无磁盘）
internal/cache/        一个 key 一个文件、原子写、TTL
internal/httpapi/      HTTP 路由与错误信封
internal/guard/        熔断器 + single-flight
internal/logx/         stderr 的 JSON 行日志
deploy/                systemd user 单元
```

## 开发

```bash
export PATH=/home/ubuntu/.hermes/tools/go-1.27.3-linux-x64/bin:$PATH
go build ./... && go vet ./... && go test ./...

make check      # vet + test + gofmt 校验
bash scripts/acceptance.sh       # 全量验收（需服务已在 :3000 运行）
python3 scripts/check_rss.py f.xml   # 校验 RSS 是否 well-formed
```

约束：**只用 Go 标准库**。`go.mod` 里不允许出现 `require`——新增依赖前先想清楚是否值得放弃「单二进制零依赖」。

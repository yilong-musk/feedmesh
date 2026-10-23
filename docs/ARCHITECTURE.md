# 架构与运维

## 设计约束

- **只用 Go 标准库**：`go.mod` 不允许出现 `require`。新增依赖前先想清楚是否值得放弃「单二进制零依赖」。
- **单静态二进制**：`CGO_ENABLED=0`，约 7.7 MB，常驻 RSS 约 15 MB。
- **输出纪律**：stdout 只放数据（RSS XML 或一行 JSON），stderr 只放 JSON 行日志；永不交互。

## 数据流

```
CLI / HTTP
   ↓
feed.Service（缓存、single-flight、错误分类）
   ↓
provider 包（weibo / sspai / v2ex，只负责 HTTP 与解析）
   ↓
feed.Item（rester 无关的统一模型）
   ↓
RSS 2.0 + Media RSS / JSON
```

每个 provider 只认识自己的上游；`feed` 不知道任何 URL、请求头或字段名。

## 缓存

- 位置：`--cache-dir` / `FEEDMESH_CACHE`，默认 `~/.cache/feedmesh`。
- 布局：一个 key 一个 JSON 文件，文件名由 key 净化（`profile:2803301701` → `profile_2803301701.json`）。
- 原子写：临时文件 → `fsync` → `rename`；并发写有互斥锁，读方看不到半截文件。
- 过期条目读取时判定为 miss 并顺手删除；损坏条目（非 JSON）同样视为 miss。
- **limit 感知**：列表类缓存记录 `{items, want}`；请求条数大于已缓存宽度时回源并加宽，小 `limit` 不会把大 `limit` 的缓存截断。

| key | TTL |
|---|---|
| `timeline:<uid>` | 5 分钟 |
| `profile:<uid>` | 1 小时 |
| `longtext:<id>` | 24 小时 |
| `domain:<name>` | 24 小时 |
| `hot:top` | 5 分钟 |
| `sspai:list` | 10 分钟 |
| `sspai:detail:<id>` | 24 小时 |
| `v2ex:latest` / `v2ex:hot` | 10 分钟 |

## 上游调用策略（微博）

- **全局并发 1**：信号量保证任何时刻最多一个在途请求，避免风控。
- **single-flight**：同 key 的并发请求合并成一次，HTTP 与 CLI 共享。
- **重试**：网络错误 / 5xx 重试 3 次，指数退避 300ms → 600ms → 1200ms。
- **熔断**：418/403 先刷新 visitor cookie 重试；仍被封则熔断 10 分钟，期间上游调用立即返回 `BREAKER_OPEN`，冷却后自动半开一次。
- 少数派与 V2EX 使用各自的 HTTP 客户端：少数派详情按 3 并发抓取，V2EX 单请求完成。

## 目录结构

```
cmd/feedmesh/       CLI：flag 解析、六个子命令、退出码映射
internal/weibo/     微博 HTTP 与解析（含热搜）
internal/sspai/     少数派 JSON API
internal/v2ex/      V2EX Atom + 最热页面
internal/feed/      统一模型、RSS 渲染、缓存编排
internal/cache/     key-per-file 缓存、TTL、原子写
internal/httpapi/   HTTP 路由与错误信封
internal/guard/     熔断器 + single-flight
internal/logx/      stderr JSON 行日志
deploy/             systemd user 单元
docs/               API、平台列表、本文件
```

## 排错

**微博改接口时只需要改 `internal/weibo`。** 其余包只消费已解析结构体。

```bash
feedmesh doctor --json                        # 1. 哪一步坏了
feedmesh fetch 2803301701 --refresh --json    # 2. 单账号端到端
journalctl --user -u feedmesh -n 50           # 3. 服务侧日志
```

| 症状 | 大概率原因 | 改哪里 |
|---|---|---|
| `doctor` 第一步失败 | visitor 接口或 cookie 名（`SUB`/`SUBP`）变了 | `internal/weibo/client.go` 的 `fetchVisitor` |
| 退出码 4 / `upstream blocked` | 缺 cookie、UA 被识别、或真被限流 | `DefaultUserAgent`、请求头；熔断冷却 `breakerCooloff` |
| 退出码 5 但账号存在 | 判断「用户不存在」的字段变了（状态在顶层 `ok`） | `internal/weibo/api.go` 的 `Profile` |
| 时间线空但资料正常 | `containerid` 取法变了（`tabs[tab_type=="weibo"]`） | `Profile` 的 tab 匹配 |
| 长文只有半截 | `statuses/extend` 字段改名 | `LongText` / `StatusText` |
| `created_at` 变成抓取时间 | 时间格式变了 | `internal/feed/service.go` 的 `createdLayouts` |
| 大量 `3` 超时 | 海外机到上游慢 | 调大 `--timeout`（默认 9s） |
| 退出码 6 | 熔断中，10 分钟自动恢复 | 无需操作 |

## 开发

```bash
export PATH=/home/ubuntu/.hermes/tools/go-1.27.3-linux-x64/bin:$PATH
go build ./... && go vet ./... && go test ./...

make check                       # vet + test + gofmt 校验
bash scripts/acceptance.sh       # 全量验收（需服务已在 :3000 运行）
python3 scripts/check_rss.py f.xml   # 校验 RSS 是否 well-formed
```

CI（GitHub Actions）跑同样的 `gofmt` / `vet` / `test -race` / 静态构建检查。

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
provider 包（weibo / sspai / v2ex / xnitter，只负责 HTTP 与解析）
   ↓
feed.Item（rester 无关的统一模型）
   ↓
RSS 2.0 + Media RSS / JSON
```

每个 provider 只认识自己的上游；`feed` 不知道任何 URL、请求头或字段名。

## 缓存

- 位置：`--cache-dir` / `FEEDMESH_CACHE`，默认 `~/.cache/feedmesh`（macOS 为 `~/Library/Caches/feedmesh`）。`os.UserCacheDir()` 不可用时使用随机私有临时目录，不再回退到共享的 `/tmp/feedmesh`；该回退目录会在进程退出时删除。
- 布局：一个 key 一个 JSON 文件，文件名由 key 净化（`profile:2803301701` → `profile_2803301701.json`）。
- 权限：目录 0700，文件 0600；拒绝符号链接形式的缓存目录。
- 原子写：临时文件 → `fsync` → `rename`；并发写有互斥锁，读方看不到半截文件。
- 过期条目在启动时及服务运行期间每小时由 `GC()` 清理；读取时也会判定为 miss 并删除。损坏条目（非 JSON）或无法通过 `Entry.Key` 验证归属的文件读取时视为 miss，且不会被删除；`GC()` 同样不会仅凭文件名前缀删除无法验证的文件，并会清理超过 1 小时的 `.json.tmp*` 临时文件。`GC()` 删除前会重新读取并比对内容，避免删掉另一个进程刚写入的新条目。
- 清理范围：`cache purge --all` 只删除能验证 `Entry.Key` 与文件名对应的 feedmesh 条目，不会误删同目录下其他 `.json` 文件；`--uid` 删除 `longtext_<uid>_*`，并额外读取旧版 timeline 收集 longtext id、扫描 domain 条目（发现阶段忽略过期时间，因此过期的 domain/timeline 也会被清理）；`--handle` 删除对应 X 时间线。
- **limit 感知**：列表类缓存记录 `{items, want}`；请求条数大于已缓存宽度时回源并加宽，小 `limit` 不会把大 `limit` 的缓存截断；`refresh=1` 也会保持已有宽度并写入新数据。读取/加宽/写回按 key 串行化，避免不同 limit 的并发请求把缓存改窄。

| key | TTL |
|---|---|
| `timeline:<uid>` | 5 分钟 |
| `profile:<uid>` | 1 小时 |
| `longtext:<uid>:<id>` | 24 小时 |
| `domain:<name>` | 24 小时 |
| `hot:top` | 5 分钟 |
| `sspai:list` | 10 分钟 |
| `sspai:detail:<id>` | 24 小时 |
| `v2ex:latest` / `v2ex:hot` | 10 分钟 |
| `x:<handle>` | 40 分钟 |

## 上游调用策略（微博）

- **全局并发 1**：信号量保证任何时刻最多一个在途请求，避免风控。
- **single-flight**：同 key 的并发请求合并成一次；只在单个进程内生效，HTTP 与 CLI 通过磁盘缓存共享结果。
- **重试**：网络错误 / 5xx 重试 3 次，指数退避 300ms → 600ms → 1200ms。
- **熔断**：418/403 先刷新 visitor cookie 重试；仍被封则熔断 10 分钟，期间上游调用立即返回 `BREAKER_OPEN`；冷却结束后只放行一个探测请求，探测成功即恢复，若再次被 block 会重新熔断。
- 少数派 client 有全局 3 并发信号量（跨请求生效）；V2EX 单请求完成。`--timeout` 显式设置时覆盖所有 provider；默认微博 9 秒、少数派/V2EX 15 秒、X 20 秒（X 单次尝试最多 25 秒）。
- HTTP 层：所有请求有 3 分钟 handler deadline；`http.Server` 设置 `ReadTimeout`/`WriteTimeout`；`ErrorLog` 接 `logx`；panic 由 recovery middleware 转成 500 JSON。
- 安全：可选 Bearer token（`--token`/`FEEDMESH_TOKEN`）；按 IP 的滚动一分钟窗口限流（`--rate-limit`/`FEEDMESH_RATE_LIMIT`）；非 loopback 且未配置限额时默认 120 次/分钟/IP。启用 token 时 RSS 响应使用 `Cache-Control: private` 并带 `Vary: Authorization`，避免共享缓存绕过鉴权。上游 HTML 在 provider 边界做 allowlist 清洗，CDATA 输出前剥离 XML 非法控制字符。
- X/Nitter：实例池只接受 https、无 userinfo、非 IP 字面量/私网域名的地址，并在实际拨号时校验解析出的 IP 是公网地址；禁止跨 host 重定向；PoW difficulty 上限 12、challenge 数据上限 2 KB、nonce 上限 200 万、单页最多 20 条；实例池最多尝试 3 个节点（单次尝试默认 20 秒、最多 25 秒），全部失败后仍会尝试内置 fallback。第三方目录仍可能投毒，建议配合 egress 防火墙。

## 目录结构

```
cmd/feedmesh/       CLI：flag 解析、六个子命令、退出码映射
internal/weibo/     微博 HTTP 与解析（含热搜）
internal/sspai/     少数派 JSON API
internal/v2ex/      V2EX Atom + 最热页面
internal/xnitter/   X 时间线：Nitter 实例池 + Anubis PoW + RSS 解析
internal/feed/      统一模型、RSS 渲染、缓存编排
internal/cache/     key-per-file 缓存、TTL、原子写
internal/httpapi/   HTTP 路由与错误信封
internal/guard/     熔断器 + single-flight
internal/logx/      stderr JSON 行日志
deploy/             systemd user 单元
docs/               API、平台列表、本文件
```

## 部署（systemd user 单元）

`deploy/feedmesh.service` 可直接装到 `~/.config/systemd/user/`：`%h` / `%C` 分别展开为用户主目录和 `~/.cache`，`CacheDirectory=feedmesh` 负责建目录（`0700`），`ReadWritePaths=` 只放开缓存目录，`ProtectSystem=strict` 让文件系统其余部分只读。

```bash
install -m0644 deploy/feedmesh.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now feedmesh
```

装完逐条核对（缺一项就不算交付）：

```bash
systemctl --user is-active feedmesh            # active
systemctl --user show feedmesh -p Result -p NRestarts --value
curl -s http://127.0.0.1:3000/healthz          # status/version/uptime/cache/breaker
```

**不要给这个 user 单元加 `ProtectKernelModules=`。** 该选项除了挂载 `/usr/lib/modules`，还会把 `CAP_SYS_MODULE` 从 capability bounding set 里摘掉（`systemctl show -p CapabilityBoundingSet` 可见差异）；修改 bounding set 需要 `CAP_SETPCAP`，而 user manager 以普通用户身份运行（`CapEff`/`CapPrm` 均为 0），`prctl(PR_CAPBSET_DROP)` 返回 EPERM，systemd 直接判定失败：`status=218/CAPABILITIES`、`Failed to drop capabilities: Operation not permitted`，`ExecStart` 里的二进制根本不会被执行。`PrivateUsers=yes` 也救不回来——它要先建 user namespace，而启用 `kernel.apparmor_restrict_unprivileged_userns=1` 的主机（Ubuntu 24.04 默认）连 uid 映射都会被拒。确实需要模块加载保护时，把单元改成 system 级（system manager 有 `CAP_SETPCAP`）。

其余硬化项（`ProtectSystem` / `ProtectKernelTunables` / `ProtectControlGroups` / `RestrictAddressFamilies` / `RestrictSUIDSGID` / `LockPersonality` / `MemoryDenyWriteExecute` / `SystemCallArchitectures` / `UMask` / `NoNewPrivileges`）不修改 bounding set，user 单元下实测均可用。

## 安全与隐私

- **监听与鉴权**：默认只监听 `127.0.0.1`；要暴露到局域网或公网，加 `--token`（`FEEDMESH_TOKEN`）并放在 TLS 反向代理之后。非 loopback 启动会打印警告，未显式配置时默认 120 次/分钟/IP 的限流（实现细节见「上游调用策略」）。
- **上游内容**：在 provider 边界做 allowlist 清洗，`script`/`iframe`、`on*` 事件属性与 `javascript:` URL 会被丢弃；CDATA 输出前剥离 XML 非法控制字符。RSS 与 JSON 都不应被当作可信 HTML 渲染。
- **出站请求**：Nitter 实例池来自第三方目录，只接受 https、无 userinfo、非私网地址，并在实际拨号时校验解析出的 IP 为公网地址；禁止跨 host 重定向。第三方目录仍可能被投毒，建议在主机/容器层加 egress 防火墙，禁止访问 loopback、RFC1918 与 link-local 地址。
- **触发成本**：`refresh=1` 与 `full=1` 会触发真实上游请求，公网部署需限制访问频率。
- **第三方可见性**：微博 UID 会发送给微博，X handle 会发送给 Nitter 实例池，少数派/V2EX 会看到服务器 IP；RSS 中的图片由阅读器直连第三方图床。自托管不等于匿名。
- **本地数据**：缓存目录默认 `~/.cache/feedmesh`（macOS 为 `~/Library/Caches/feedmesh`），包含账号/域名/handle 标识与正文，目录权限 0700、文件 0600；过期条目在启动时及运行期间每小时清理；`os.UserCacheDir()` 不可用时使用随机私有临时目录并在进程退出时删除。`cache purge --all`、`--uid`、`--handle` 可做定向清理。
- **合规**：抓取微博 visitor cookie、解 Anubis、抓取 V2EX/少数派全文可能涉及平台条款与内容版权，使用场景自行确认。

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
| 大量 `3` 超时 | 海外机到上游慢 | 调大 `--timeout`（默认微博 9s；少数派/V2EX 15s；X 20s） |
| 退出码 6 | 熔断中，10 分钟自动恢复 | 无需操作 |
| `/rss/x/*` 全 504 | Nitter 实例池整体失效（status.d420.de 里 `rss=true` 的节点会轮换） | 先跑 `feedmesh` 的 X 路由带 `refresh=1`；持续失败就更新 `internal/xnitter` 的 `defaultInstances`，或换 `StatusURL` |
| X 时间线返回 0 条但 HTTP 200 | 实例改了 RSS 结构或转成 JS 壳 | `internal/xnitter/client.go` 的 `parseRSS`；对照真实 `/{handle}/rss` 输出 |
| X 抓取偶发卡在验证页 | 实例换了 Anubis 挑战格式 | `parseChallenge`（`anubis_challenge` / `preact_info`） |

## 开发

```bash
go build ./... && go vet ./... && go test ./...

make check                       # vet + test + gofmt 校验
bash scripts/acceptance.sh       # 全量验收（需服务已在 :3000 运行）
python3 scripts/check_rss.py f.xml   # 校验 RSS 是否 well-formed
```

CI（GitHub Actions）跑同样的 `gofmt` / `vet` / `test -race` / 静态构建检查。

# feedmesh

[![CI](https://github.com/yilong-musk/feedmesh/actions/workflows/ci.yml/badge.svg)](https://github.com/yilong-musk/feedmesh/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

[English](README.md) | 简体中文

把多个平台的内容汇聚成 RSS —— 自托管、单个静态二进制、零第三方依赖（`go.mod` 里没有任何 `require`），默认只监听 `127.0.0.1`。已接入的平台见下表，细节见 [docs/PLATFORMS.md](docs/PLATFORMS.md)；新平台以 provider 的形式持续增加。

## 支持的平台

| 平台 | 内容 | RSS | JSON |
|---|---|---|---|
| <img src="docs/assets/weibo.png" width="18" height="18" alt="微博"> 微博 | 账号时间线 | `/rss/user/{uid}`、`/rss/domain/{name}` | `/api/v1/feed?uid=` |
| <img src="docs/assets/weibo.png" width="18" height="18" alt="微博"> 微博 | 实时热搜 | `/rss/hot` | `/api/v1/hot` |
| <img src="docs/assets/sspai.png" width="18" height="18" alt="少数派"> 少数派 | 最新文章 | `/rss/sspai` | `/api/v1/sspai` |
| <img src="docs/assets/v2ex.png" width="18" height="18" alt="V2EX"> V2EX | 最新主题 | `/rss/v2ex` | `/api/v1/v2ex?section=latest` |
| <img src="docs/assets/v2ex.png" width="18" height="18" alt="V2EX"> V2EX | 最热主题 | `/rss/v2ex/hot` | `/api/v1/v2ex?section=hot` |
| <img src="docs/assets/x.png" width="18" height="18" alt="X"> X | 账号时间线 | `/rss/x/{handle}` | `/api/v1/x?handle=` |

参数、数据源与限制见 **[docs/PLATFORMS.md](docs/PLATFORMS.md)**。

## 特性

- **完整 RSS**：正文 HTML、配图（真实宽高 + 懒加载）、转发引用、外链卡片、长文全文；`content:encoded` 与 Media RSS 同文档输出，微博视频带 `enclosure`。
- **对上游友好**：磁盘缓存 + 按 `limit` 加宽（小请求不会截断更宽的缓存）+ single-flight；微博并发恒为 1，遇 418/403 刷新 cookie，持续失败则熔断 10 分钟。
- **两种视图**：RSS 给阅读器，JSON 给脚本与 agent（每次响应一行 JSON）。
- **单二进制**：静态链接（CGO 关闭），约 8.2 MB，常驻内存约 11 MB。

## 快速开始

需要 Go 1.27+。

```bash
git clone https://github.com/yilong-musk/feedmesh.git
cd feedmesh

make check        # go vet + go test + gofmt
make install      # 安装到 ~/.local/bin/feedmesh
feedmesh serve --listen 127.0.0.1:3000
```

试一下：

```bash
curl -s 'http://127.0.0.1:3000/rss/hot?limit=5'
curl -s 'http://127.0.0.1:3000/rss/v2ex/hot?limit=5'
curl -s 'http://127.0.0.1:3000/healthz' | jq
```

### systemd（用户级单元）

```bash
mkdir -p ~/.config/systemd/user
cp deploy/feedmesh.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now feedmesh
loginctl enable-linger "$USER"     # 未登录也常驻
```

单元使用 systemd 的 `%h` / `%C` 展开用户主目录与缓存目录，无需手工替换用户名；二进制装到别处时改 `ExecStart` 即可。

## CLI

| 命令 | 作用 |
|---|---|
| `serve --listen <addr>` | 启动 HTTP 服务；可选 `--token` / `--rate-limit` |
| `fetch <uid\|domain>` | 打印单个微博账号的时间线（RSS；`--json` 输出一行 JSON） |
| `doctor [--uid <uid>]` | 按顺序探活 visitor、getIndex、statuses |
| `cache stat` | 缓存条目数与体积 |
| `cache purge --all` | 清空所有 feedmesh 缓存条目 |
| `cache purge --uid <uid>` | 清除某个微博账号的 profile/timeline/domain/feed/长文缓存 |
| `cache purge --handle <handle>` | 清除某个 X handle 的时间线缓存 |
| `--dry-run` | 只报告，不删除 |

全局 flag：`--json`、`--quiet`、`--timeout`、`--cache-dir`、`--refresh`。`--timeout` 默认微博 9 秒、少数派/V2EX 15 秒、X 20 秒；显式设置后覆盖所有 provider，但 X 单次尝试最多 25 秒。stdout 只放数据，stderr 只放 JSON 行日志。

## 文档

英文版是唯一权威来源，本页是它的中文翻译：

- **[API](docs/API.md)**：端点、参数、JSON 结构、错误码、RSS 输出细节、缓存 TTL
- **[支持的平台](docs/PLATFORMS.md)**：每个来源的规则与限制，以及新增 provider 的步骤
- **[架构与运维](docs/ARCHITECTURE.md)**：缓存实现、上游策略、安全与隐私、目录结构、排错、开发命令
- **[English](README.md)**：英文原文，唯一权威来源（本页是它的中文翻译）

## License

[MIT](LICENSE)

# feedmesh

[![CI](https://github.com/yilong-musk/feedmesh/actions/workflows/ci.yml/badge.svg)](https://github.com/yilong-musk/feedmesh/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

[Deutsch](README.de.md) | [English](README.md) | [Español](README.es.md) | [Français](README.fr.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | [Português](README.pt.md) | [Русский](README.ru.md) | 简体中文 | [繁體中文](README.zh-TW.md)

把多个平台的内容汇聚成 RSS —— 自托管、单个静态二进制、零第三方依赖（`go.mod` 里没有任何 `require`），默认只监听 `127.0.0.1`。已接入的平台见下表，细节见 [docs/PLATFORMS.md](docs/PLATFORMS.md)；新平台以 provider 的形式持续增加。

## 支持的平台

<table>
  <thead>
    <tr><th width="136">平台</th><th>内容</th><th>RSS</th><th>JSON</th></tr>
  </thead>
  <tbody>
    <tr><td><img src="docs/assets/weibo.png" width="18" height="18" align="absmiddle" alt="微博">&nbsp;微博</td><td>账号时间线</td><td><code>/rss/user/{uid}</code>, <code>/rss/domain/{name}</code></td><td><code>/api/v1/feed?uid=</code></td></tr>
    <tr><td><img src="docs/assets/weibo.png" width="18" height="18" align="absmiddle" alt="微博">&nbsp;微博</td><td>实时热搜</td><td><code>/rss/hot</code></td><td><code>/api/v1/hot</code></td></tr>
    <tr><td><img src="docs/assets/sspai.png" width="18" height="18" align="absmiddle" alt="少数派">&nbsp;少数派</td><td>最新文章</td><td><code>/rss/sspai</code></td><td><code>/api/v1/sspai</code></td></tr>
    <tr><td><img src="docs/assets/v2ex.png" width="18" height="18" align="absmiddle" alt="V2EX">&nbsp;V2EX</td><td>最新主题</td><td><code>/rss/v2ex</code></td><td><code>/api/v1/v2ex?section=latest</code></td></tr>
    <tr><td><img src="docs/assets/v2ex.png" width="18" height="18" align="absmiddle" alt="V2EX">&nbsp;V2EX</td><td>最热主题</td><td><code>/rss/v2ex/hot</code></td><td><code>/api/v1/v2ex?section=hot</code></td></tr>
    <tr><td><img src="docs/assets/smzdm.png" width="18" height="18" align="absmiddle" alt="SMZDM">&nbsp;什么值得买</td><td>优惠精选（好价）</td><td><code>/rss/smzdm</code></td><td><code>/api/v1/smzdm</code></td></tr>
    <tr><td><img src="docs/assets/smzdm.png" width="18" height="18" align="absmiddle" alt="SMZDM">&nbsp;什么值得买</td><td>社区文章</td><td><code>/rss/smzdm/post</code></td><td><code>/api/v1/smzdm?channel=post</code></td></tr>
    <tr><td><img src="docs/assets/smzdm.png" width="18" height="18" align="absmiddle" alt="SMZDM">&nbsp;什么值得买</td><td>海淘好价</td><td><code>/rss/smzdm/haitao</code></td><td><code>/api/v1/smzdm?channel=haitao</code></td></tr>
    <tr><td><img src="docs/assets/smzdm.png" width="18" height="18" align="absmiddle" alt="SMZDM">&nbsp;什么值得买</td><td>品类好价</td><td><code>/rss/smzdm/fenlei/{slug}</code></td><td><code>/api/v1/smzdm?channel=fenlei&slug=</code></td></tr>
    <tr><td><img src="docs/assets/x.png" width="18" height="18" align="absmiddle" alt="X">&nbsp;X</td><td>账号时间线</td><td><code>/rss/x/{handle}</code></td><td><code>/api/v1/x?handle=</code></td></tr>
    <tr><td><img src="docs/assets/youtube.png" width="18" height="18" align="absmiddle" alt="YouTube">&nbsp;YouTube</td><td>频道上传</td><td><code>/rss/youtube/{handle或id}</code></td><td><code>/api/v1/youtube?ref=</code></td></tr>
    <tr><td><img src="docs/assets/douyin.png" width="18" height="18" align="absmiddle" alt="抖音">&nbsp;抖音</td><td>创作者视频</td><td><code>/rss/douyin/{sec_uid}</code></td><td><code>/api/v1/douyin?user=</code></td></tr>
    <tr><td><img src="docs/assets/tiktok.png" width="18" height="18" align="absmiddle" alt="TikTok">&nbsp;TikTok</td><td>创作者视频</td><td><code>/rss/tiktok/{handle}</code></td><td><code>/api/v1/tiktok?user=</code></td></tr>
    <tr><td><img src="docs/assets/bilibili.png" width="18" height="18" align="absmiddle" alt="哔哩哔哩">&nbsp;哔哩哔哩</td><td>UP 主视频</td><td><code>/rss/bilibili/{mid}</code></td><td><code>/api/v1/bilibili?user=</code></td></tr>
    <tr><td><img src="docs/assets/telegram.png" width="18" height="18" align="absmiddle" alt="Telegram">&nbsp;Telegram</td><td>公开频道推送</td><td><code>/rss/telegram/{channel}</code></td><td><code>/api/v1/telegram?channel=</code></td></tr>
    <tr><td><img src="docs/assets/apple.png" width="18" height="18" align="absmiddle" alt="Apple Podcasts">&nbsp;Apple Podcasts</td><td>节目单集</td><td><code>/rss/apple/{id}</code></td><td><code>/api/v1/apple?id=</code></td></tr>
    <tr><td><img src="docs/assets/apple.png" width="18" height="18" align="absmiddle" alt="Apple Podcasts">&nbsp;Apple Podcasts</td><td>热门排行</td><td><code>/rss/apple/top</code></td><td><code>/api/v1/apple?chart=top&country=</code></td></tr>
    <tr><td><img src="docs/assets/pinterest.png" width="18" height="18" align="absmiddle" alt="Pinterest">&nbsp;Pinterest</td><td>个人主页 / 板子 pin</td><td><code>/rss/pinterest/{user}</code>, <code>/rss/pinterest/{user}/{board}</code></td><td><code>/api/v1/pinterest?user=&amp;board=</code></td></tr>
    <tr><td><img src="docs/assets/polymarket.png" width="18" height="18" align="absmiddle" alt="Polymarket">&nbsp;Polymarket</td><td>新市场 / 热门事件</td><td><code>/rss/polymarket/new</code>, <code>/rss/polymarket/hot</code></td><td><code>/api/v1/polymarket?feed=new|hot</code></td></tr>
    <tr><td><img src="docs/assets/polymarket.png" width="18" height="18" align="absmiddle" alt="Polymarket">&nbsp;Polymarket</td><td>单事件赔率</td><td><code>/rss/polymarket/event/{slug}</code></td><td><code>/api/v1/polymarket?feed=event&amp;slug=</code></td></tr>
    <tr><td><img src="docs/assets/polymarket.png" width="18" height="18" align="absmiddle" alt="Polymarket">&nbsp;Polymarket</td><td>钱包成交流</td><td><code>/rss/polymarket/user/{address}</code></td><td><code>/api/v1/polymarket?feed=user&amp;address=</code></td></tr>
    <tr><td><img src="docs/assets/linuxdo.png" width="18" height="18" align="absmiddle" alt="LINUX DO">&nbsp;LINUX DO</td><td>论坛话题（最新 / 热门 / 分类 / 标签）</td><td><code>/rss/linuxdo</code>, <code>/rss/linuxdo/top</code>, <code>/rss/linuxdo/hot</code>, <code>/rss/linuxdo/c/{slug}/{id}</code>, <code>/rss/linuxdo/tag/{tag}</code></td><td><code>/api/v1/linuxdo?feed=</code></td></tr>
    <tr><td><img src="docs/assets/reddit.png" width="18" height="18" align="absmiddle" alt="Reddit">&nbsp;Reddit</td><td>版块列表</td><td><code>/rss/reddit/{sub}</code></td><td><code>/api/v1/reddit?sub=</code></td></tr>
  </tbody>
</table>

参数、数据源与限制见 **[docs/PLATFORMS.md](docs/PLATFORMS.md)**。

## 特性

- **完整 RSS**：正文 HTML、配图（真实宽高 + 懒加载）、转发引用、外链卡片、长文全文；`content:encoded` 与 Media RSS 同文档输出，微博视频带 `enclosure`。
- **对上游友好**：磁盘缓存 + 按 `limit` 加宽（小请求不会截断更宽的缓存）+ single-flight；微博并发恒为 1，遇 418/403 刷新 cookie，持续失败则熔断 10 分钟。
- **两种视图**：RSS 给阅读器，JSON 给脚本与 agent（每次响应一行 JSON）。
- **单二进制**：静态链接（CGO 关闭），约 8.3 MB，常驻内存约 11 MB。

## 安装

Linux、macOS、Windows（amd64/arm64）的静态二进制和 Debian/RPM 包随每个
[release](https://github.com/yilong-musk/feedmesh/releases) 发布，压缩包可用
`checksums.txt` 校验。已有 Go 工具链（1.27+）时，注意主包在 `cmd/feedmesh`，
安装路径要带上一级：

```bash
go install github.com/yilong-musk/feedmesh/cmd/feedmesh@latest
```

### Homebrew

```bash
brew install --cask yilong-musk/tap/feedmesh
brew upgrade --cask feedmesh
```

cask 由发布流程推送到 [yilong-musk/homebrew-tap](https://github.com/yilong-musk/homebrew-tap)，
安装的是预编译的 macOS / Linux 二进制，并在安装后去掉 quarantine 属性（归档未做 Apple 公证）。

### Docker

```bash
docker run -d --name feedmesh -p 127.0.0.1:3000:3000 \
  -v feedmesh-data:/data ghcr.io/yilong-musk/feedmesh:latest
```

每次发布都会推送 `linux/amd64` 与 `linux/arm64` 镜像，以非 root 用户运行，缓存放在
`/data` 卷里。除抖音外所有 provider 都可用 —— 抖音阅读器需要 Chromium
（`FEEDMESH_CHROME`）。

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
curl -s 'http://127.0.0.1:3000/rss/tiktok/nasa?limit=5'
curl -s 'http://127.0.0.1:3000/rss/bilibili/946974?limit=5'
curl -s 'http://127.0.0.1:3000/rss/telegram/sspai?limit=5'
curl -s 'http://127.0.0.1:3000/rss/apple/1200361736?limit=5'
curl -s 'http://127.0.0.1:3000/rss/linuxdo?limit=5'
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
| `doctor --x-handle <handle>` | 探活 X 实例池与各镜像实例 |
| `cache stat` | 缓存条目数与体积 |
| `cache purge --all` | 清空所有 feedmesh 缓存条目 |
| `cache purge --uid <uid>` | 清除某个微博账号的 profile/timeline/domain/feed/长文缓存 |
| `cache purge --handle <handle>` | 清除某个 X handle 的时间线缓存 |
| `--dry-run` | 只报告，不删除 |

全局 flag：`--json`、`--quiet`、`--timeout`、`--cache-dir`、`--refresh`。`--timeout` 默认微博 9 秒、少数派/V2EX/什么值得买/Reddit/Pinterest/Polymarket 15 秒、X/YouTube 20 秒、TikTok 30 秒、哔哩哔哩 40 秒、抖音 45 秒（冷启动浏览器约半分钟）、Telegram 15 秒、LINUX DO 20 秒、Apple Podcasts 20 秒；显式设置后覆盖所有 provider，但 X 单次尝试最多 25 秒。stdout 只放数据，stderr 只放 JSON 行日志。

## 文档

英文版是唯一权威来源，本页是它的中文翻译：

- **[API](docs/API.md)**：端点、参数、JSON 结构、错误码、RSS 输出细节、缓存 TTL
- **[支持的平台](docs/PLATFORMS.md)**：每个来源的规则与限制，以及新增 provider 的步骤
- **[架构与运维](docs/ARCHITECTURE.md)**：缓存实现、上游策略、安全与隐私、目录结构、排错、开发命令
- **[English](README.md)**：英文原文，唯一权威来源（本页是它的中文翻译）

## License

[MIT](LICENSE)

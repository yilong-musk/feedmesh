# feedmesh

[![CI](https://github.com/yilong-musk/feedmesh/actions/workflows/ci.yml/badge.svg)](https://github.com/yilong-musk/feedmesh/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

[English](README.md) | [简体中文](README.zh-CN.md) | 繁體中文 | [日本語](README.ja.md) | [한국어](README.ko.md) | [Español](README.es.md) | [Français](README.fr.md) | [Deutsch](README.de.md) | [Português](README.pt.md) | [Русский](README.ru.md)

把多個平台的內容匯聚成 RSS —— 自託管、單個靜態二進位、零第三方依賴（`go.mod` 裡沒有任何 `require`），預設只監聽 `127.0.0.1`。已接入的平台見下表，細節見 [docs/PLATFORMS.md](docs/PLATFORMS.md)；新平台以 provider 的形式持續增加。

## 支援的平台

<table>
  <thead>
    <tr><th width="136">平台</th><th>內容</th><th>RSS</th><th>JSON</th></tr>
  </thead>
  <tbody>
    <tr><td><img src="docs/assets/weibo.png" width="18" height="18" align="absmiddle" alt="微博">&nbsp;微博</td><td>帳號時間線</td><td><code>/rss/user/{uid}</code>, <code>/rss/domain/{name}</code></td><td><code>/api/v1/feed?uid=</code></td></tr>
    <tr><td><img src="docs/assets/weibo.png" width="18" height="18" align="absmiddle" alt="微博">&nbsp;微博</td><td>即時熱搜</td><td><code>/rss/hot</code></td><td><code>/api/v1/hot</code></td></tr>
    <tr><td><img src="docs/assets/sspai.png" width="18" height="18" align="absmiddle" alt="少數派">&nbsp;少數派</td><td>最新文章</td><td><code>/rss/sspai</code></td><td><code>/api/v1/sspai</code></td></tr>
    <tr><td><img src="docs/assets/v2ex.png" width="18" height="18" align="absmiddle" alt="V2EX">&nbsp;V2EX</td><td>最新主題</td><td><code>/rss/v2ex</code></td><td><code>/api/v1/v2ex?section=latest</code></td></tr>
    <tr><td><img src="docs/assets/v2ex.png" width="18" height="18" align="absmiddle" alt="V2EX">&nbsp;V2EX</td><td>最熱主題</td><td><code>/rss/v2ex/hot</code></td><td><code>/api/v1/v2ex?section=hot</code></td></tr>
    <tr><td><img src="docs/assets/smzdm.png" width="18" height="18" align="absmiddle" alt="SMZDM">&nbsp;什麼值得買</td><td>優惠精選（好價）</td><td><code>/rss/smzdm</code></td><td><code>/api/v1/smzdm</code></td></tr>
    <tr><td><img src="docs/assets/smzdm.png" width="18" height="18" align="absmiddle" alt="SMZDM">&nbsp;什麼值得買</td><td>社群文章</td><td><code>/rss/smzdm/post</code></td><td><code>/api/v1/smzdm?channel=post</code></td></tr>
    <tr><td><img src="docs/assets/smzdm.png" width="18" height="18" align="absmiddle" alt="SMZDM">&nbsp;什麼值得買</td><td>海淘好價</td><td><code>/rss/smzdm/haitao</code></td><td><code>/api/v1/smzdm?channel=haitao</code></td></tr>
    <tr><td><img src="docs/assets/smzdm.png" width="18" height="18" align="absmiddle" alt="SMZDM">&nbsp;什麼值得買</td><td>品類好價</td><td><code>/rss/smzdm/fenlei/{slug}</code></td><td><code>/api/v1/smzdm?channel=fenlei&slug=</code></td></tr>
    <tr><td><img src="docs/assets/x.png" width="18" height="18" align="absmiddle" alt="X">&nbsp;X</td><td>帳號時間線</td><td><code>/rss/x/{handle}</code></td><td><code>/api/v1/x?handle=</code></td></tr>
    <tr><td><img src="docs/assets/youtube.png" width="18" height="18" align="absmiddle" alt="YouTube">&nbsp;YouTube</td><td>頻道上傳</td><td><code>/rss/youtube/{handle或id}</code></td><td><code>/api/v1/youtube?ref=</code></td></tr>
    <tr><td><img src="docs/assets/douyin.png" width="18" height="18" align="absmiddle" alt="抖音">&nbsp;抖音</td><td>創作者影片</td><td><code>/rss/douyin/{sec_uid}</code></td><td><code>/api/v1/douyin?user=</code></td></tr>
    <tr><td><img src="docs/assets/tiktok.png" width="18" height="18" align="absmiddle" alt="TikTok">&nbsp;TikTok</td><td>創作者影片</td><td><code>/rss/tiktok/{handle}</code></td><td><code>/api/v1/tiktok?user=</code></td></tr>
    <tr><td><img src="docs/assets/bilibili.png" width="18" height="18" align="absmiddle" alt="嗶哩嗶哩">&nbsp;嗶哩嗶哩</td><td>UP 主影片</td><td><code>/rss/bilibili/{mid}</code></td><td><code>/api/v1/bilibili?user=</code></td></tr>
    <tr><td><img src="docs/assets/telegram.png" width="18" height="18" align="absmiddle" alt="Telegram">&nbsp;Telegram</td><td>公開頻道推送</td><td><code>/rss/telegram/{channel}</code></td><td><code>/api/v1/telegram?channel=</code></td></tr>
    <tr><td><img src="docs/assets/pinterest.png" width="18" height="18" align="absmiddle" alt="Pinterest">&nbsp;Pinterest</td><td>個人主頁 / 圖版 pin</td><td><code>/rss/pinterest/{user}</code>, <code>/rss/pinterest/{user}/{board}</code></td><td><code>/api/v1/pinterest?user=&amp;board=</code></td></tr>
    <tr><td><img src="docs/assets/polymarket.png" width="18" height="18" align="absmiddle" alt="Polymarket">&nbsp;Polymarket</td><td>新市場 / 熱門事件</td><td><code>/rss/polymarket/new</code>, <code>/rss/polymarket/hot</code></td><td><code>/api/v1/polymarket?feed=new|hot</code></td></tr>
    <tr><td><img src="docs/assets/polymarket.png" width="18" height="18" align="absmiddle" alt="Polymarket">&nbsp;Polymarket</td><td>單一事件賠率</td><td><code>/rss/polymarket/event/{slug}</code></td><td><code>/api/v1/polymarket?feed=event&amp;slug=</code></td></tr>
    <tr><td><img src="docs/assets/polymarket.png" width="18" height="18" align="absmiddle" alt="Polymarket">&nbsp;Polymarket</td><td>錢包成交流</td><td><code>/rss/polymarket/user/{address}</code></td><td><code>/api/v1/polymarket?feed=user&amp;address=</code></td></tr>
    <tr><td><img src="docs/assets/linuxdo.png" width="18" height="18" align="absmiddle" alt="LINUX DO">&nbsp;LINUX DO</td><td>論壇話題（最新 / 熱門 / 分類 / 標籤）</td><td><code>/rss/linuxdo</code>, <code>/rss/linuxdo/top</code>, <code>/rss/linuxdo/hot</code>, <code>/rss/linuxdo/c/{slug}/{id}</code>, <code>/rss/linuxdo/tag/{tag}</code></td><td><code>/api/v1/linuxdo?feed=</code></td></tr>
    <tr><td><img src="docs/assets/reddit.png" width="18" height="18" align="absmiddle" alt="Reddit">&nbsp;Reddit</td><td>版塊列表</td><td><code>/rss/reddit/{sub}</code></td><td><code>/api/v1/reddit?sub=</code></td></tr>
  </tbody>
</table>

參數、資料來源與限制見 **[docs/PLATFORMS.md](docs/PLATFORMS.md)**。

## 特性

- **完整 RSS**：正文 HTML、配圖（真實寬高 + 懶載入）、轉發引用、外連卡片、長文全文；`content:encoded` 與 Media RSS 同文件輸出，微博影片帶 `enclosure`。
- **對上游友善**：磁碟快取 + 按 `limit` 加寬（小請求不會截斷更寬的快取）+ single-flight；微博並行恆為 1，遇到 418/403 重新整理 cookie，持續失敗則熔斷 10 分鐘。
- **兩種檢視**：RSS 給閱讀器，JSON 給指令稿與 agent（每次回應一行 JSON）。
- **單一二進位**：靜態連結（CGO 關閉），約 8.3 MB，常駐記憶體約 11 MB。

## 安裝

Linux、macOS、Windows（amd64/arm64）的靜態二進位和 Debian/RPM 套件隨每個
[release](https://github.com/yilong-musk/feedmesh/releases) 發布，壓縮檔可用
`checksums.txt` 校驗。已有 Go 工具鏈（1.27+）時，注意主套件在 `cmd/feedmesh`，
安裝路徑要多一級：

```bash
go install github.com/yilong-musk/feedmesh/cmd/feedmesh@latest
```

### Homebrew

```bash
brew install --cask yilong-musk/tap/feedmesh
brew upgrade --cask feedmesh
```

cask 由發布流程推送到 [yilong-musk/homebrew-tap](https://github.com/yilong-musk/homebrew-tap)，
安裝的是預先編譯的 macOS / Linux 二進位，並在安裝後去掉 quarantine 屬性（封存檔未做 Apple 公證）。

### Docker

```bash
docker run -d --name feedmesh -p 127.0.0.1:3000:3000 \
  -v feedmesh-data:/data ghcr.io/yilong-musk/feedmesh:latest
```

每次發布都會推送 `linux/amd64` 與 `linux/arm64` 映像，以非 root 使用者執行，快取放在
`/data` 磁碟區裡。除抖音外所有 provider 都可用 —— 抖音閱讀器需要 Chromium
（`FEEDMESH_CHROME`）。

## 快速開始

需要 Go 1.27+。

```bash
git clone https://github.com/yilong-musk/feedmesh.git
cd feedmesh

make check        # go vet + go test + gofmt
make install      # 安裝到 ~/.local/bin/feedmesh
feedmesh serve --listen 127.0.0.1:3000
```

試一下：

```bash
curl -s 'http://127.0.0.1:3000/rss/hot?limit=5'
curl -s 'http://127.0.0.1:3000/rss/v2ex/hot?limit=5'
curl -s 'http://127.0.0.1:3000/rss/tiktok/nasa?limit=5'
curl -s 'http://127.0.0.1:3000/rss/bilibili/946974?limit=5'
curl -s 'http://127.0.0.1:3000/rss/telegram/sspai?limit=5'
curl -s 'http://127.0.0.1:3000/rss/linuxdo?limit=5'
curl -s 'http://127.0.0.1:3000/healthz' | jq
```

### systemd（使用者單元）

```bash
mkdir -p ~/.config/systemd/user
cp deploy/feedmesh.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now feedmesh
loginctl enable-linger "$USER"     # 未登入也常駐
```

單元使用 systemd 的 `%h` / `%C` 展開使用者主目錄與快取目錄，無需手動替換使用者名稱；二進位裝到別處時改 `ExecStart` 即可。

## CLI

| 命令 | 作用 |
|---|---|
| `serve --listen <addr>` | 啟動 HTTP 服務；可選 `--token` / `--rate-limit` |
| `fetch <uid\|domain>` | 印出單一微博帳號的時間線（RSS；`--json` 輸出一行 JSON） |
| `doctor [--uid <uid>]` | 依序探活 visitor、getIndex、statuses |
| `doctor --x-handle <handle>` | 探活 X 實例池與各鏡像實例 |
| `cache stat` | 快取項目數與體積 |
| `cache purge --all` | 清空所有 feedmesh 快取項目 |
| `cache purge --uid <uid>` | 清除某個微博帳號的 profile/timeline/domain/feed/長文快取 |
| `cache purge --handle <handle>` | 清除某個 X handle 的時間線快取 |
| `--dry-run` | 只報告，不刪除 |

全域 flag：`--json`、`--quiet`、`--timeout`、`--cache-dir`、`--refresh`。`--timeout` 預設微博 9 秒、sspai/V2EX/SMZDM/Reddit/Pinterest/Polymarket 15 秒、X/YouTube 20 秒、TikTok 30 秒、Bilibili 40 秒、抖音 45 秒（冷啟動瀏覽器約半分鐘）、Telegram 15 秒、LINUX DO 20 秒；顯式指定後覆蓋所有 provider，但 X 單次嘗試不超過 25 秒。stdout 只放資料，stderr 只放 JSON 行日誌。

## 文件

英文版是唯一權威來源，本頁是它的繁體中文翻譯：

- **[API](docs/API.md)**：端點、參數、JSON 結構、錯誤碼、RSS 輸出細節、快取 TTL
- **[支援的平台](docs/PLATFORMS.md)**：每個來源的規則與限制，以及新增 provider 的步驟
- **[架構與維運](docs/ARCHITECTURE.md)**：快取實作、上游策略、安全與隱私、目錄結構、排錯、開發命令
- **[English](README.md)**：英文原文，唯一權威來源（本頁是它的繁體中文翻譯）

## License

[MIT](LICENSE)

# feedmesh

[![CI](https://github.com/yilong-musk/feedmesh/actions/workflows/ci.yml/badge.svg)](https://github.com/yilong-musk/feedmesh/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

[Deutsch](README.de.md) | [English](README.md) | [Español](README.es.md) | [Français](README.fr.md) | 日本語 | [한국어](README.ko.md) | [Português](README.pt.md) | [Русский](README.ru.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md)

複数プラットフォームのコンテンツを RSS にまとめる、セルフホストの単一静的バイナリです。サードパーティ依存はありません（`go.mod` に `require` は一つもありません）。既定では `127.0.0.1` のみを待ち受けます。対応プラットフォームは下表、詳細は [docs/PLATFORMS.md](docs/PLATFORMS.md) を参照してください。新しいプラットフォームは provider として追加されます。

## 対応プラットフォーム

<table>
  <thead>
    <tr><th width="136">プラットフォーム</th><th>内容</th><th>RSS</th><th>JSON</th></tr>
  </thead>
  <tbody>
    <tr><td><img src="docs/assets/weibo.png" width="18" height="18" align="absmiddle" alt="Weibo">&nbsp;Weibo</td><td>アカウントのタイムライン</td><td><code>/rss/user/{uid}</code>, <code>/rss/domain/{name}</code></td><td><code>/api/v1/feed?uid=</code></td></tr>
    <tr><td><img src="docs/assets/weibo.png" width="18" height="18" align="absmiddle" alt="Weibo">&nbsp;Weibo</td><td>リアルタイム急上昇</td><td><code>/rss/hot</code></td><td><code>/api/v1/hot</code></td></tr>
    <tr><td><img src="docs/assets/sspai.png" width="18" height="18" align="absmiddle" alt="sspai">&nbsp;sspai</td><td>最新記事</td><td><code>/rss/sspai</code></td><td><code>/api/v1/sspai</code></td></tr>
    <tr><td><img src="docs/assets/v2ex.png" width="18" height="18" align="absmiddle" alt="V2EX">&nbsp;V2EX</td><td>最新トピック</td><td><code>/rss/v2ex</code></td><td><code>/api/v1/v2ex?section=latest</code></td></tr>
    <tr><td><img src="docs/assets/v2ex.png" width="18" height="18" align="absmiddle" alt="V2EX">&nbsp;V2EX</td><td>人気トピック</td><td><code>/rss/v2ex/hot</code></td><td><code>/api/v1/v2ex?section=hot</code></td></tr>
    <tr><td><img src="docs/assets/smzdm.png" width="18" height="18" align="absmiddle" alt="SMZDM">&nbsp;SMZDM</td><td>厳選セール</td><td><code>/rss/smzdm</code></td><td><code>/api/v1/smzdm</code></td></tr>
    <tr><td><img src="docs/assets/smzdm.png" width="18" height="18" align="absmiddle" alt="SMZDM">&nbsp;SMZDM</td><td>コミュニティ記事</td><td><code>/rss/smzdm/post</code></td><td><code>/api/v1/smzdm?channel=post</code></td></tr>
    <tr><td><img src="docs/assets/smzdm.png" width="18" height="18" align="absmiddle" alt="SMZDM">&nbsp;SMZDM</td><td>海外セール</td><td><code>/rss/smzdm/haitao</code></td><td><code>/api/v1/smzdm?channel=haitao</code></td></tr>
    <tr><td><img src="docs/assets/smzdm.png" width="18" height="18" align="absmiddle" alt="SMZDM">&nbsp;SMZDM</td><td>カテゴリ別のお買い得</td><td><code>/rss/smzdm/fenlei/{slug}</code></td><td><code>/api/v1/smzdm?channel=fenlei&amp;slug=</code></td></tr>
    <tr><td><img src="docs/assets/x.png" width="18" height="18" align="absmiddle" alt="X">&nbsp;X</td><td>アカウントのタイムライン</td><td><code>/rss/x/{handle}</code></td><td><code>/api/v1/x?handle=</code></td></tr>
    <tr><td><img src="docs/assets/youtube.png" width="18" height="18" align="absmiddle" alt="YouTube">&nbsp;YouTube</td><td>チャンネルの投稿</td><td><code>/rss/youtube/{handle|id}</code></td><td><code>/api/v1/youtube?ref=</code></td></tr>
    <tr><td><img src="docs/assets/douyin.png" width="18" height="18" align="absmiddle" alt="Douyin">&nbsp;Douyin</td><td>クリエイターの動画</td><td><code>/rss/douyin/{sec_uid}</code></td><td><code>/api/v1/douyin?user=</code></td></tr>
    <tr><td><img src="docs/assets/tiktok.png" width="18" height="18" align="absmiddle" alt="TikTok">&nbsp;TikTok</td><td>クリエイターの動画</td><td><code>/rss/tiktok/{handle}</code></td><td><code>/api/v1/tiktok?user=</code></td></tr>
    <tr><td><img src="docs/assets/bilibili.png" width="18" height="18" align="absmiddle" alt="Bilibili">&nbsp;Bilibili</td><td>投稿者の動画</td><td><code>/rss/bilibili/{mid}</code></td><td><code>/api/v1/bilibili?user=</code></td></tr>
    <tr><td><img src="docs/assets/telegram.png" width="18" height="18" align="absmiddle" alt="Telegram">&nbsp;Telegram</td><td>公開チャンネルの投稿</td><td><code>/rss/telegram/{channel}</code></td><td><code>/api/v1/telegram?channel=</code></td></tr>
    <tr><td><img src="docs/assets/pinterest.png" width="18" height="18" align="absmiddle" alt="Pinterest">&nbsp;Pinterest</td><td>プロフィール / ボードの pin</td><td><code>/rss/pinterest/{user}</code>, <code>/rss/pinterest/{user}/{board}</code></td><td><code>/api/v1/pinterest?user=&amp;board=</code></td></tr>
    <tr><td><img src="docs/assets/polymarket.png" width="18" height="18" align="absmiddle" alt="Polymarket">&nbsp;Polymarket</td><td>新着 / トレンドのイベント</td><td><code>/rss/polymarket/new</code>, <code>/rss/polymarket/hot</code></td><td><code>/api/v1/polymarket?feed=new|hot</code></td></tr>
    <tr><td><img src="docs/assets/polymarket.png" width="18" height="18" align="absmiddle" alt="Polymarket">&nbsp;Polymarket</td><td>イベントのオッズ</td><td><code>/rss/polymarket/event/{slug}</code></td><td><code>/api/v1/polymarket?feed=event&amp;slug=</code></td></tr>
    <tr><td><img src="docs/assets/polymarket.png" width="18" height="18" align="absmiddle" alt="Polymarket">&nbsp;Polymarket</td><td>ウォレットの動向</td><td><code>/rss/polymarket/user/{address}</code></td><td><code>/api/v1/polymarket?feed=user&amp;address=</code></td></tr>
    <tr><td><img src="docs/assets/linuxdo.png" width="18" height="18" align="absmiddle" alt="LINUX DO">&nbsp;LINUX DO</td><td>フォーラムのトピック（最新 / トップ / 人気 / カテゴリ / タグ）</td><td><code>/rss/linuxdo</code>, <code>/rss/linuxdo/top</code>, <code>/rss/linuxdo/hot</code>, <code>/rss/linuxdo/c/{slug}/{id}</code>, <code>/rss/linuxdo/tag/{tag}</code></td><td><code>/api/v1/linuxdo?feed=</code></td></tr>
    <tr><td><img src="docs/assets/reddit.png" width="18" height="18" align="absmiddle" alt="Reddit">&nbsp;Reddit</td><td>subreddit の一覧</td><td><code>/rss/reddit/{sub}</code></td><td><code>/api/v1/reddit?sub=</code></td></tr>
  </tbody>
</table>

パラメータ、データ源、制限は **[docs/PLATFORMS.md](docs/PLATFORMS.md)** にあります。

## 機能

- **完全な RSS**: 本文 HTML、画像（実寸と遅延読み込み）、リポスト引用、リンクカード、長文の全文。`content:encoded` と Media RSS を同一ドキュメントに出力し、Weibo の動画には `enclosure` を付けます。
- **上流に優しい**: ディスクキャッシュ、`limit` を考慮した拡張（小さいリクエストが、より広いキャッシュを切り詰めない）、single-flight。Weibo の同時実行は常に 1。418/403 で cookie を更新し、失敗が続くと 10 分間のサーキットブレーカを開きます。
- **二つの見え方**: リーダー向けの RSS と、スクリプトや agent 向けの JSON（レスポンスは 1 行）。
- **単一バイナリ**: 静的リンク（CGO 無効）、約 8.3 MB、常駐メモリはおよそ 11 MB。

## インストール

Linux、macOS、Windows（amd64/arm64）向けの静的バイナリと Debian/RPM パッケージは、毎回の
[release](https://github.com/yilong-musk/feedmesh/releases) に添付されます。アーカイブは
`checksums.txt` で検証してください。Go ツールチェーン（1.27 以上）がある場合、メインパッケージは
`cmd/feedmesh` にあるので、インストールパスにもそれを含めます。

```bash
go install github.com/yilong-musk/feedmesh/cmd/feedmesh@latest
```

### Homebrew

```bash
brew install --cask yilong-musk/tap/feedmesh
brew upgrade --cask feedmesh
```

cask はリリース手順により [yilong-musk/homebrew-tap](https://github.com/yilong-musk/homebrew-tap) へ公開されます。インストールされるのはビルド済みの macOS / Linux バイナリで、quarantine 属性は外されます（アーカイブは Apple の公証を受けていません）。

### Docker

```bash
docker run -d --name feedmesh -p 127.0.0.1:3000:3000 \
  -v feedmesh-data:/data ghcr.io/yilong-musk/feedmesh:latest
```

リリースのたびに `linux/amd64` と `linux/arm64` のイメージを公開します。非 root ユーザーで動き、キャッシュは `/data` ボリュームに置きます。Douyin 以外の provider はすべて使えます。Douyin のリーダーには Chromium（`FEEDMESH_CHROME`）が必要です。

## クイックスタート

Go 1.27 以上が必要です。

```bash
git clone https://github.com/yilong-musk/feedmesh.git
cd feedmesh

make check        # go vet + go test + gofmt
make install      # ~/.local/bin/feedmesh にインストール
feedmesh serve --listen 127.0.0.1:3000
```

試す:

```bash
curl -s 'http://127.0.0.1:3000/rss/hot?limit=5'
curl -s 'http://127.0.0.1:3000/rss/v2ex/hot?limit=5'
curl -s 'http://127.0.0.1:3000/rss/tiktok/nasa?limit=5'
curl -s 'http://127.0.0.1:3000/rss/bilibili/946974?limit=5'
curl -s 'http://127.0.0.1:3000/rss/telegram/sspai?limit=5'
curl -s 'http://127.0.0.1:3000/rss/linuxdo?limit=5'
curl -s 'http://127.0.0.1:3000/healthz' | jq
```

### systemd（ユーザーユニット）

```bash
mkdir -p ~/.config/systemd/user
cp deploy/feedmesh.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now feedmesh
loginctl enable-linger "$USER"     # ログインしていなくても動かし続ける
```

ユニットは systemd の `%h` / `%C` でホームとキャッシュディレクトリを展開するので、ユーザー名の置換は不要です。バイナリの場所が違う場合は `ExecStart` を変えてください。

## CLI

| コマンド | 動作 |
|---|---|
| `serve --listen <addr>` | HTTP サーバを起動する。任意で `--token` / `--rate-limit` |
| `fetch <uid\|domain>` | Weibo アカウントのタイムラインを 1 件表示する（RSS。`--json` は JSON を 1 行） |
| `doctor [--uid <uid>]` | visitor、getIndex、statuses を順に疎通確認する |
| `doctor --x-handle <handle>` | X のインスタンスプールと各ミラーを、指定ハンドルについて疎通確認する |
| `cache stat` | キャッシュ件数とサイズ |
| `cache purge --all` | feedmesh のキャッシュ項目をすべて削除する |
| `cache purge --uid <uid>` | 1 アカウントの profile/timeline/domain/feed/長文項目を削除する |
| `cache purge --handle <handle>` | 1 つの X ハンドルのタイムライン項目を削除する |
| `--dry-run` | 報告のみ。何も削除しない |

グローバルフラグ: `--json`、`--quiet`、`--timeout`、`--cache-dir`、`--refresh`。`--timeout` の既定は Weibo が 9 秒、sspai/V2EX/SMZDM/Reddit/Pinterest/Polymarket が 15 秒、X/YouTube が 20 秒、TikTok が 30 秒、Bilibili が 40 秒、Douyin が 45 秒（冷えたブラウザはおよそ 30 秒）、Telegram が 15 秒、LINUX DO が 20 秒です。明示した値はすべての provider を上書きしますが、X の 1 回の試行は 25 秒を超えません。stdout にはデータだけ、stderr には JSON のログ行だけが出ます。

## ドキュメント

英語版が唯一の正本です。このページはその日本語訳です。

- **[API](docs/API.md)**: エンドポイント、パラメータ、JSON の形、エラーコード、RSS 出力の詳細、キャッシュ TTL
- **[プラットフォーム](docs/PLATFORMS.md)**: ソースごとの規則と制限、provider の追加手順
- **[アーキテクチャと運用](docs/ARCHITECTURE.md)**: キャッシュ実装、上流ポリシー、セキュリティとプライバシー、構成、トラブルシュート、開発コマンド
- **[English](README.md)**: 英語の原文。唯一の正本（このページはその日本語訳）

## License

[MIT](LICENSE)

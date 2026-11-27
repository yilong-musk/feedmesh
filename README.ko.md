# feedmesh

[![CI](https://github.com/yilong-musk/feedmesh/actions/workflows/ci.yml/badge.svg)](https://github.com/yilong-musk/feedmesh/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

[Deutsch](README.de.md) | [English](README.md) | [Español](README.es.md) | [Français](README.fr.md) | [日本語](README.ja.md) | 한국어 | [Português](README.pt.md) | [Русский](README.ru.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md)

여러 플랫폼의 콘텐츠를 RSS로 모읍니다. 자체 호스팅, 정적 바이너리 하나, 서드파티 의존성 없음(`go.mod`에 `require`가 하나도 없습니다). 기본값은 `127.0.0.1`만 수신합니다. 지원 플랫폼은 아래 표와 [docs/PLATFORMS.md](docs/PLATFORMS.md)에 있으며, 새 플랫폼은 provider로 추가됩니다.

## 지원 플랫폼

<table>
  <thead>
    <tr><th width="136">플랫폼</th><th>내용</th><th>RSS</th><th>JSON</th></tr>
  </thead>
  <tbody>
    <tr><td><img src="docs/assets/weibo.png" width="18" height="18" align="absmiddle" alt="Weibo">&nbsp;Weibo</td><td>계정 타임라인</td><td><code>/rss/user/{uid}</code>, <code>/rss/domain/{name}</code></td><td><code>/api/v1/feed?uid=</code></td></tr>
    <tr><td><img src="docs/assets/weibo.png" width="18" height="18" align="absmiddle" alt="Weibo">&nbsp;Weibo</td><td>실시간 급상승 검색</td><td><code>/rss/hot</code></td><td><code>/api/v1/hot</code></td></tr>
    <tr><td><img src="docs/assets/sspai.png" width="18" height="18" align="absmiddle" alt="sspai">&nbsp;sspai</td><td>최신 글</td><td><code>/rss/sspai</code></td><td><code>/api/v1/sspai</code></td></tr>
    <tr><td><img src="docs/assets/v2ex.png" width="18" height="18" align="absmiddle" alt="V2EX">&nbsp;V2EX</td><td>최신 주제</td><td><code>/rss/v2ex</code></td><td><code>/api/v1/v2ex?section=latest</code></td></tr>
    <tr><td><img src="docs/assets/v2ex.png" width="18" height="18" align="absmiddle" alt="V2EX">&nbsp;V2EX</td><td>인기 주제</td><td><code>/rss/v2ex/hot</code></td><td><code>/api/v1/v2ex?section=hot</code></td></tr>
    <tr><td><img src="docs/assets/smzdm.png" width="18" height="18" align="absmiddle" alt="SMZDM">&nbsp;SMZDM</td><td>선정 할인</td><td><code>/rss/smzdm</code></td><td><code>/api/v1/smzdm</code></td></tr>
    <tr><td><img src="docs/assets/smzdm.png" width="18" height="18" align="absmiddle" alt="SMZDM">&nbsp;SMZDM</td><td>커뮤니티 글</td><td><code>/rss/smzdm/post</code></td><td><code>/api/v1/smzdm?channel=post</code></td></tr>
    <tr><td><img src="docs/assets/smzdm.png" width="18" height="18" align="absmiddle" alt="SMZDM">&nbsp;SMZDM</td><td>해외 할인</td><td><code>/rss/smzdm/haitao</code></td><td><code>/api/v1/smzdm?channel=haitao</code></td></tr>
    <tr><td><img src="docs/assets/smzdm.png" width="18" height="18" align="absmiddle" alt="SMZDM">&nbsp;SMZDM</td><td>카테고리별 좋은 가격</td><td><code>/rss/smzdm/fenlei/{slug}</code></td><td><code>/api/v1/smzdm?channel=fenlei&amp;slug=</code></td></tr>
    <tr><td><img src="docs/assets/x.png" width="18" height="18" align="absmiddle" alt="X">&nbsp;X</td><td>계정 타임라인</td><td><code>/rss/x/{handle}</code></td><td><code>/api/v1/x?handle=</code></td></tr>
    <tr><td><img src="docs/assets/youtube.png" width="18" height="18" align="absmiddle" alt="YouTube">&nbsp;YouTube</td><td>채널 업로드</td><td><code>/rss/youtube/{handle|id}</code></td><td><code>/api/v1/youtube?ref=</code></td></tr>
    <tr><td><img src="docs/assets/douyin.png" width="18" height="18" align="absmiddle" alt="Douyin">&nbsp;Douyin</td><td>크리에이터 동영상</td><td><code>/rss/douyin/{sec_uid}</code></td><td><code>/api/v1/douyin?user=</code></td></tr>
    <tr><td><img src="docs/assets/tiktok.png" width="18" height="18" align="absmiddle" alt="TikTok">&nbsp;TikTok</td><td>크리에이터 동영상</td><td><code>/rss/tiktok/{handle}</code></td><td><code>/api/v1/tiktok?user=</code></td></tr>
    <tr><td><img src="docs/assets/bilibili.png" width="18" height="18" align="absmiddle" alt="Bilibili">&nbsp;Bilibili</td><td>업로더 동영상</td><td><code>/rss/bilibili/{mid}</code></td><td><code>/api/v1/bilibili?user=</code></td></tr>
    <tr><td><img src="docs/assets/telegram.png" width="18" height="18" align="absmiddle" alt="Telegram">&nbsp;Telegram</td><td>공개 채널 글</td><td><code>/rss/telegram/{channel}</code></td><td><code>/api/v1/telegram?channel=</code></td></tr>
    <tr><td><img src="docs/assets/pinterest.png" width="18" height="18" align="absmiddle" alt="Pinterest">&nbsp;Pinterest</td><td>프로필 / 보드 pin</td><td><code>/rss/pinterest/{user}</code>, <code>/rss/pinterest/{user}/{board}</code></td><td><code>/api/v1/pinterest?user=&amp;board=</code></td></tr>
    <tr><td><img src="docs/assets/polymarket.png" width="18" height="18" align="absmiddle" alt="Polymarket">&nbsp;Polymarket</td><td>신규 / 인기 이벤트</td><td><code>/rss/polymarket/new</code>, <code>/rss/polymarket/hot</code></td><td><code>/api/v1/polymarket?feed=new|hot</code></td></tr>
    <tr><td><img src="docs/assets/polymarket.png" width="18" height="18" align="absmiddle" alt="Polymarket">&nbsp;Polymarket</td><td>이벤트 배당</td><td><code>/rss/polymarket/event/{slug}</code></td><td><code>/api/v1/polymarket?feed=event&amp;slug=</code></td></tr>
    <tr><td><img src="docs/assets/polymarket.png" width="18" height="18" align="absmiddle" alt="Polymarket">&nbsp;Polymarket</td><td>지갑 활동</td><td><code>/rss/polymarket/user/{address}</code></td><td><code>/api/v1/polymarket?feed=user&amp;address=</code></td></tr>
    <tr><td><img src="docs/assets/linuxdo.png" width="18" height="18" align="absmiddle" alt="LINUX DO">&nbsp;LINUX DO</td><td>포럼 주제 (최신 / 상위 / 인기 / 분류 / 태그)</td><td><code>/rss/linuxdo</code>, <code>/rss/linuxdo/top</code>, <code>/rss/linuxdo/hot</code>, <code>/rss/linuxdo/c/{slug}/{id}</code>, <code>/rss/linuxdo/tag/{tag}</code></td><td><code>/api/v1/linuxdo?feed=</code></td></tr>
    <tr><td><img src="docs/assets/reddit.png" width="18" height="18" align="absmiddle" alt="Reddit">&nbsp;Reddit</td><td>서브레딧 목록</td><td><code>/rss/reddit/{sub}</code></td><td><code>/api/v1/reddit?sub=</code></td></tr>
  </tbody>
</table>

매개변수, 데이터 소스, 제한은 **[docs/PLATFORMS.md](docs/PLATFORMS.md)** 에 있습니다.

## 기능

- **완전한 RSS**: 본문 HTML, 이미지(실제 크기와 지연 로딩), 리포스트 인용, 링크 카드, 장문 전문. `content:encoded`와 Media RSS를 같은 문서에 넣고, Weibo 동영상에는 `enclosure`를 붙입니다.
- **업스트림에 부드럽게**: 디스크 캐시, `limit`을 고려한 확장(작은 요청이 더 넓은 캐시를 자르지 않음), single-flight. Weibo 동시성은 항상 1입니다. 418/403이면 cookie를 새로 고치고, 실패가 계속되면 10분 차단기를 엽니다.
- **두 가지 보기**: 리더용 RSS, 스크립트와 agent용 JSON(응답마다 JSON 한 줄).
- **단일 바이너리**: 정적 링크(CGO 끔), 약 8.3 MB, 상주 메모리 약 11 MB.

## 설치

Linux, macOS, Windows(amd64/arm64)용 정적 바이너리와 Debian/RPM 패키지는 매
[release](https://github.com/yilong-musk/feedmesh/releases)에 첨부됩니다. 아카이브는
`checksums.txt`로 검증하세요. Go 도구 사슬(1.27+)이 있으면 메인 패키지가 `cmd/feedmesh`에
있으므로 설치 경로에도 그것을 포함합니다.

```bash
go install github.com/yilong-musk/feedmesh/cmd/feedmesh@latest
```

### Homebrew

```bash
brew install --cask yilong-musk/tap/feedmesh
brew upgrade --cask feedmesh
```

cask는 릴리스 파이프라인이 [yilong-musk/homebrew-tap](https://github.com/yilong-musk/homebrew-tap)에 올립니다. 미리 빌드된 macOS 또는 Linux 바이너리를 설치하고 quarantine 속성을 제거합니다(아카이브는 Apple 공증을 받지 않았습니다).

### Docker

```bash
docker run -d --name feedmesh -p 127.0.0.1:3000:3000 \
  -v feedmesh-data:/data ghcr.io/yilong-musk/feedmesh:latest
```

릴리스마다 `linux/amd64`와 `linux/arm64` 이미지를 게시합니다. root가 아닌 사용자로 실행하고, 캐시는 `/data` 볼륨에 둡니다. Douyin을 제외한 모든 provider를 쓸 수 있습니다. Douyin 리더는 Chromium(`FEEDMESH_CHROME`)이 필요합니다.

## 빠른 시작

Go 1.27 이상이 필요합니다.

```bash
git clone https://github.com/yilong-musk/feedmesh.git
cd feedmesh

make check        # go vet + go test + gofmt
make install      # ~/.local/bin/feedmesh 에 설치
feedmesh serve --listen 127.0.0.1:3000
```

시험:

```bash
curl -s 'http://127.0.0.1:3000/rss/hot?limit=5'
curl -s 'http://127.0.0.1:3000/rss/v2ex/hot?limit=5'
curl -s 'http://127.0.0.1:3000/rss/tiktok/nasa?limit=5'
curl -s 'http://127.0.0.1:3000/rss/bilibili/946974?limit=5'
curl -s 'http://127.0.0.1:3000/rss/telegram/sspai?limit=5'
curl -s 'http://127.0.0.1:3000/rss/linuxdo?limit=5'
curl -s 'http://127.0.0.1:3000/healthz' | jq
```

### systemd (사용자 유닛)

```bash
mkdir -p ~/.config/systemd/user
cp deploy/feedmesh.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now feedmesh
loginctl enable-linger "$USER"     # 로그인 세션 없이도 유지
```

유닛은 systemd의 `%h` / `%C`로 홈과 캐시 디렉터리를 펼치므로 사용자 이름을 바꿀 필요가 없습니다. 바이너리가 다른 곳에 있으면 `ExecStart`를 조정하세요.

## CLI

| 명령 | 하는 일 |
|---|---|
| `serve --listen <addr>` | HTTP 서버를 시작합니다. 선택: `--token` / `--rate-limit` |
| `fetch <uid\|domain>` | Weibo 계정 타임라인 하나를 출력합니다(RSS. `--json`이면 JSON 한 줄) |
| `doctor [--uid <uid>]` | visitor, getIndex, statuses를 순서대로 probe합니다 |
| `doctor --x-handle <handle>` | X 인스턴스 풀과 각 미러를 해당 핸들에 대해 probe합니다 |
| `cache stat` | 캐시 항목 수와 크기 |
| `cache purge --all` | feedmesh 캐시 항목을 모두 지웁니다 |
| `cache purge --uid <uid>` | 한 계정의 profile/timeline/domain/feed/장문 항목을 지웁니다 |
| `cache purge --handle <handle>` | X 핸들 하나의 타임라인 항목을 지웁니다 |
| `--dry-run` | 보고만 하고 지우지 않습니다 |

전역 플래그: `--json`, `--quiet`, `--timeout`, `--cache-dir`, `--refresh`. `--timeout` 기본값은 Weibo 9초, sspai/V2EX/SMZDM/Reddit/Pinterest/Polymarket 15초, X/YouTube 20초, TikTok 30초, Bilibili 40초, Douyin 45초(차가운 브라우저는 약 30초), Telegram 15초, LINUX DO 20초입니다. 값을 직접 주면 모든 provider를 덮어쓰지만, X의 한 시도는 25초를 넘지 않습니다. stdout에는 데이터만, stderr에는 JSON 로그 줄만 나갑니다.

## 문서

영어판이 유일한 기준입니다. 이 페이지는 그 한국어 번역입니다.

- **[API](docs/API.md)**: 엔드포인트, 매개변수, JSON 형태, 오류 코드, RSS 출력 세부, 캐시 TTL
- **[플랫폼](docs/PLATFORMS.md)**: 소스별 규칙과 제한, provider를 추가하는 단계
- **[아키텍처와 운영](docs/ARCHITECTURE.md)**: 캐시 구현, 업스트림 정책, 보안과 프라이버시, 배치, 문제 해결, 개발 명령
- **[English](README.md)**: 영어 원문. 유일한 기준(이 페이지는 그 한국어 번역)

## License

[MIT](LICENSE)

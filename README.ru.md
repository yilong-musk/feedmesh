# feedmesh

[![CI](https://github.com/yilong-musk/feedmesh/actions/workflows/ci.yml/badge.svg)](https://github.com/yilong-musk/feedmesh/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

[Deutsch](README.de.md) | [English](README.md) | [Español](README.es.md) | [Français](README.fr.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | [Português](README.pt.md) | Русский | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md)

Собирает материалы нескольких площадок в RSS — свой хост, один статический бинарник, без сторонних зависимостей (в `go.mod` нет ни одного `require`), по умолчанию слушает только `127.0.0.1`. Поддерживаемые площадки перечислены ниже и подробно описаны в [docs/PLATFORMS.md](docs/PLATFORMS.md); новые появляются как provider.

## Поддерживаемые площадки

| Площадка&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp; | Содержимое | RSS | JSON |
|---|---|---|---|
| <img src="docs/assets/weibo.png" width="18" height="18" align="absmiddle" alt="Weibo">&nbsp;Weibo | Лента аккаунта | `/rss/user/{uid}`, `/rss/domain/{name}` | `/api/v1/feed?uid=` |
| <img src="docs/assets/weibo.png" width="18" height="18" align="absmiddle" alt="Weibo">&nbsp;Weibo | Горячий поиск в реальном времени | `/rss/hot` | `/api/v1/hot` |
| <img src="docs/assets/sspai.png" width="18" height="18" align="absmiddle" alt="sspai">&nbsp;sspai | Свежие статьи | `/rss/sspai` | `/api/v1/sspai` |
| <img src="docs/assets/v2ex.png" width="18" height="18" align="absmiddle" alt="V2EX">&nbsp;V2EX | Свежие темы | `/rss/v2ex` | `/api/v1/v2ex?section=latest` |
| <img src="docs/assets/v2ex.png" width="18" height="18" align="absmiddle" alt="V2EX">&nbsp;V2EX | Горячие темы | `/rss/v2ex/hot` | `/api/v1/v2ex?section=hot` |
| <img src="docs/assets/smzdm.png" width="18" height="18" align="absmiddle" alt="SMZDM">&nbsp;SMZDM | Отобранные скидки | `/rss/smzdm` | `/api/v1/smzdm` |
| <img src="docs/assets/smzdm.png" width="18" height="18" align="absmiddle" alt="SMZDM">&nbsp;SMZDM | Статьи сообщества | `/rss/smzdm/post` | `/api/v1/smzdm?channel=post` |
| <img src="docs/assets/smzdm.png" width="18" height="18" align="absmiddle" alt="SMZDM">&nbsp;SMZDM | Зарубежные скидки | `/rss/smzdm/haitao` | `/api/v1/smzdm?channel=haitao` |
| <img src="docs/assets/smzdm.png" width="18" height="18" align="absmiddle" alt="SMZDM">&nbsp;SMZDM | Хорошие цены по категории | `/rss/smzdm/fenlei/{slug}` | `/api/v1/smzdm?channel=fenlei&slug=` |
| <img src="docs/assets/x.png" width="18" height="18" align="absmiddle" alt="X">&nbsp;X | Лента аккаунта | `/rss/x/{handle}` | `/api/v1/x?handle=` |
| <img src="docs/assets/youtube.png" width="18" height="18" align="absmiddle" alt="YouTube">&nbsp;YouTube | Загрузки канала | `/rss/youtube/{handle\|id}` | `/api/v1/youtube?ref=` |
| <img src="docs/assets/douyin.png" width="18" height="18" align="absmiddle" alt="Douyin">&nbsp;Douyin | Видео автора | `/rss/douyin/{sec_uid}` | `/api/v1/douyin?user=` |
| <img src="docs/assets/tiktok.png" width="18" height="18" align="absmiddle" alt="TikTok">&nbsp;TikTok | Видео автора | `/rss/tiktok/{handle}` | `/api/v1/tiktok?user=` |
| <img src="docs/assets/bilibili.png" width="18" height="18" align="absmiddle" alt="Bilibili">&nbsp;Bilibili | Видео автора | `/rss/bilibili/{mid}` | `/api/v1/bilibili?user=` |
| <img src="docs/assets/telegram.png" width="18" height="18" align="absmiddle" alt="Telegram">&nbsp;Telegram | Посты публичного канала | `/rss/telegram/{channel}` | `/api/v1/telegram?channel=` |
| <img src="docs/assets/apple.png" width="18" height="18" align="absmiddle" alt="Apple Podcasts">&nbsp;Apple Podcasts | Выпуски передачи | `/rss/apple/{id}` | `/api/v1/apple?id=` |
| <img src="docs/assets/apple.png" width="18" height="18" align="absmiddle" alt="Apple Podcasts">&nbsp;Apple Podcasts | Чарт | `/rss/apple/top` | `/api/v1/apple?chart=top&country=` |
| <img src="docs/assets/xiaoyuzhou.png" width="18" height="18" align="absmiddle" alt="Xiaoyuzhou">&nbsp;Xiaoyuzhou | Выпуски передачи | `/rss/xiaoyuzhou/{id}` | `/api/v1/xiaoyuzhou?id=` |
| <img src="docs/assets/trumpstruth.png" width="18" height="18" align="absmiddle" alt="Trump's Truth">&nbsp;Trump's Truth | Последние записи | `/rss/trumpstruth` | `/api/v1/trumpstruth` |
| <img src="docs/assets/pinterest.png" width="18" height="18" align="absmiddle" alt="Pinterest">&nbsp;Pinterest | Пины профиля или доски | `/rss/pinterest/{user}`, `/rss/pinterest/{user}/{board}` | `/api/v1/pinterest?user=&board=` |
| <img src="docs/assets/polymarket.png" width="18" height="18" align="absmiddle" alt="Polymarket">&nbsp;Polymarket | Новые или трендовые события | `/rss/polymarket/new`, `/rss/polymarket/hot` | `/api/v1/polymarket?feed=new\|hot` |
| <img src="docs/assets/polymarket.png" width="18" height="18" align="absmiddle" alt="Polymarket">&nbsp;Polymarket | Коэффициенты события | `/rss/polymarket/event/{slug}` | `/api/v1/polymarket?feed=event&slug=` |
| <img src="docs/assets/polymarket.png" width="18" height="18" align="absmiddle" alt="Polymarket">&nbsp;Polymarket | Активность кошелька | `/rss/polymarket/user/{address}` | `/api/v1/polymarket?feed=user&address=` |
| <img src="docs/assets/linuxdo.png" width="18" height="18" align="absmiddle" alt="LINUX DO">&nbsp;LINUX DO | Темы форума (новые / топ / горячие / категория / тег) | `/rss/linuxdo`, `/rss/linuxdo/top`, `/rss/linuxdo/hot`, `/rss/linuxdo/c/{slug}/{id}`, `/rss/linuxdo/tag/{tag}` | `/api/v1/linuxdo?feed=` |
| <img src="docs/assets/reddit.png" width="18" height="18" align="absmiddle" alt="Reddit">&nbsp;Reddit | Список сабреддита | `/rss/reddit/{sub}` | `/api/v1/reddit?sub=` |

Параметры, источники и ограничения — в **[docs/PLATFORMS.md](docs/PLATFORMS.md)**.

## Возможности

- **Полный RSS**: HTML тела, картинки (реальные размеры и ленивая загрузка), цитаты репостов, карточки ссылок, полный длинный текст; `content:encoded` и Media RSS в одном документе, `enclosure` для видео Weibo.
- **Бережно к источникам**: дисковый кэш с расширением по `limit` (узкий запрос не обрезает более широкий кэш) и single-flight; параллелизм Weibo всегда 1, cookie обновляются при 418/403, устойчивый сбой открывает размыкатель на 10 минут.
- **Два вида**: RSS для читалок, JSON для скриптов и agent (одна строка на ответ).
- **Один бинарник**: статическая линковка (CGO выключен), около 8,3 МБ, резидентная память около 11 МБ.

## Установка

Статические бинарники для Linux, macOS и Windows (amd64/arm64) и пакеты Debian/RPM
приложены к каждому [релизу](https://github.com/yilong-musk/feedmesh/releases); сверяйте
архивы с `checksums.txt`. Если есть цепочка Go (1.27+), основной пакет лежит в
`cmd/feedmesh`, поэтому путь установки его включает:

```bash
go install github.com/yilong-musk/feedmesh/cmd/feedmesh@latest
```

### Homebrew

```bash
brew install --cask yilong-musk/tap/feedmesh
brew upgrade --cask feedmesh
```

Cask публикует конвейер релиза в [yilong-musk/homebrew-tap](https://github.com/yilong-musk/homebrew-tap). Ставится готовый бинарник macOS или Linux, атрибут quarantine снимается (архивы не нотаризованы).

### Docker

```bash
docker run -d --name feedmesh -p 127.0.0.1:3000:3000 \
  -v feedmesh-data:/data ghcr.io/yilong-musk/feedmesh:latest
```

С каждым релизом публикуются образы `linux/amd64` и `linux/arm64`. Они работают не от root и держат кэш в томе `/data`. Работают все provider, кроме Douyin: его читателю нужен Chromium (`FEEDMESH_CHROME`).

## Быстрый старт

Нужен Go 1.27 или новее.

```bash
git clone https://github.com/yilong-musk/feedmesh.git
cd feedmesh

make check        # go vet + go test + gofmt
make install      # ставит в ~/.local/bin/feedmesh
feedmesh serve --listen 127.0.0.1:3000
```

Проверка:

```bash
curl -s 'http://127.0.0.1:3000/rss/hot?limit=5'
curl -s 'http://127.0.0.1:3000/rss/v2ex/hot?limit=5'
curl -s 'http://127.0.0.1:3000/rss/tiktok/nasa?limit=5'
curl -s 'http://127.0.0.1:3000/rss/bilibili/946974?limit=5'
curl -s 'http://127.0.0.1:3000/rss/telegram/sspai?limit=5'
curl -s 'http://127.0.0.1:3000/rss/apple/1200361736?limit=5'
curl -s 'http://127.0.0.1:3000/rss/xiaoyuzhou/6021f949a789fca4eff4492c?limit=5'
curl -s 'http://127.0.0.1:3000/rss/trumpstruth?limit=5'
curl -s 'http://127.0.0.1:3000/rss/linuxdo?limit=5'
curl -s 'http://127.0.0.1:3000/healthz' | jq
```

### systemd (пользовательский юнит)

```bash
mkdir -p ~/.config/systemd/user
cp deploy/feedmesh.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now feedmesh
loginctl enable-linger "$USER"     # работает без активного входа
```

Юнит раскрывает домашний каталог и каталог кэша через `%h` / `%C` systemd, подставлять имя пользователя не нужно. Если бинарник лежит в другом месте, поправьте `ExecStart`.

## CLI

| Команда | Что делает |
|---|---|
| `serve --listen <addr>` | Запускает HTTP-сервер; необязательно `--token` / `--rate-limit` |
| `fetch <uid\|domain>` | Печатает ленту одного аккаунта Weibo (RSS; `--json` печатает одну строку JSON) |
| `doctor [--uid <uid>]` | По очереди проверяет visitor, getIndex и statuses |
| `doctor --x-handle <handle>` | Проверяет пул инстансов X и каждое зеркало для handle |
| `cache stat` | Число и размер записей кэша |
| `cache purge --all` | Удаляет все записи кэша feedmesh |
| `cache purge --uid <uid>` | Удаляет profile/timeline/domain/feed/длинный текст одного аккаунта |
| `cache purge --handle <handle>` | Удаляет записи ленты одного handle X |
| `--dry-run` | Только отчёт, ничего не удаляет |

Глобальные флаги: `--json`, `--quiet`, `--timeout`, `--cache-dir`, `--refresh`. `--timeout` по умолчанию 9 с для Weibo, 15 с для sspai/V2EX/SMZDM/Reddit/Pinterest/Polymarket, 20 с для X/YouTube, 30 с для TikTok, 40 с для Bilibili, 45 с для Douyin (холодный браузер занимает около полуминуты), 15 с для Telegram, Xiaoyuzhou и Trump's Truth, 20 с для LINUX DO и 20 с для Apple Podcasts; явное значение перекрывает каждый provider, но одна попытка X не длится дольше 25 с. stdout несёт только данные, stderr — только строки JSON-журнала.

## Документация

Английский текст — единственный источник истины; эта страница — его русский перевод.

- **[API](docs/API.md)**: конечные точки, параметры, формы JSON, коды ошибок, детали вывода RSS, TTL кэша
- **[Площадки](docs/PLATFORMS.md)**: правила и пределы каждого источника и шаги добавления provider
- **[Архитектура и эксплуатация](docs/ARCHITECTURE.md)**: реализация кэша, политика upstream, безопасность и приватность, раскладка, диагностика, команды разработки
- **[English](README.md)**: английский оригинал, единственный источник истины (эта страница — его русский перевод)

## License

[MIT](LICENSE)

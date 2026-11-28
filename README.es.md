# feedmesh

[![CI](https://github.com/yilong-musk/feedmesh/actions/workflows/ci.yml/badge.svg)](https://github.com/yilong-musk/feedmesh/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

[Deutsch](README.de.md) | [English](README.md) | Español | [Français](README.fr.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | [Português](README.pt.md) | [Русский](README.ru.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md)

Reúne el contenido de varias plataformas en RSS: autoalojado, un solo binario estático, sin dependencias de terceros (no hay ni un `require` en `go.mod`) y, por defecto, escucha solo en `127.0.0.1`. Las plataformas admitidas están abajo y se detallan en [docs/PLATFORMS.md](docs/PLATFORMS.md); las nuevas llegan como providers.

## Plataformas admitidas

| Plataforma&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp; | Contenido | RSS | JSON |
|---|---|---|---|
| <img src="docs/assets/weibo.png" width="18" height="18" align="absmiddle" alt="Weibo">&nbsp;Weibo | Cronología de una cuenta | `/rss/user/{uid}`, `/rss/domain/{name}` | `/api/v1/feed?uid=` |
| <img src="docs/assets/weibo.png" width="18" height="18" align="absmiddle" alt="Weibo">&nbsp;Weibo | Búsquedas en tiempo real | `/rss/hot` | `/api/v1/hot` |
| <img src="docs/assets/sspai.png" width="18" height="18" align="absmiddle" alt="sspai">&nbsp;sspai | Artículos recientes | `/rss/sspai` | `/api/v1/sspai` |
| <img src="docs/assets/v2ex.png" width="18" height="18" align="absmiddle" alt="V2EX">&nbsp;V2EX | Temas recientes | `/rss/v2ex` | `/api/v1/v2ex?section=latest` |
| <img src="docs/assets/v2ex.png" width="18" height="18" align="absmiddle" alt="V2EX">&nbsp;V2EX | Temas populares | `/rss/v2ex/hot` | `/api/v1/v2ex?section=hot` |
| <img src="docs/assets/smzdm.png" width="18" height="18" align="absmiddle" alt="SMZDM">&nbsp;SMZDM | Ofertas seleccionadas | `/rss/smzdm` | `/api/v1/smzdm` |
| <img src="docs/assets/smzdm.png" width="18" height="18" align="absmiddle" alt="SMZDM">&nbsp;SMZDM | Artículos de la comunidad | `/rss/smzdm/post` | `/api/v1/smzdm?channel=post` |
| <img src="docs/assets/smzdm.png" width="18" height="18" align="absmiddle" alt="SMZDM">&nbsp;SMZDM | Ofertas del extranjero | `/rss/smzdm/haitao` | `/api/v1/smzdm?channel=haitao` |
| <img src="docs/assets/smzdm.png" width="18" height="18" align="absmiddle" alt="SMZDM">&nbsp;SMZDM | Buenos precios por categoría | `/rss/smzdm/fenlei/{slug}` | `/api/v1/smzdm?channel=fenlei&slug=` |
| <img src="docs/assets/x.png" width="18" height="18" align="absmiddle" alt="X">&nbsp;X | Cronología de una cuenta | `/rss/x/{handle}` | `/api/v1/x?handle=` |
| <img src="docs/assets/youtube.png" width="18" height="18" align="absmiddle" alt="YouTube">&nbsp;YouTube | Subidas de un canal | `/rss/youtube/{handle\|id}` | `/api/v1/youtube?ref=` |
| <img src="docs/assets/douyin.png" width="18" height="18" align="absmiddle" alt="Douyin">&nbsp;Douyin | Vídeos de un creador | `/rss/douyin/{sec_uid}` | `/api/v1/douyin?user=` |
| <img src="docs/assets/tiktok.png" width="18" height="18" align="absmiddle" alt="TikTok">&nbsp;TikTok | Vídeos de un creador | `/rss/tiktok/{handle}` | `/api/v1/tiktok?user=` |
| <img src="docs/assets/bilibili.png" width="18" height="18" align="absmiddle" alt="Bilibili">&nbsp;Bilibili | Vídeos de un creador | `/rss/bilibili/{mid}` | `/api/v1/bilibili?user=` |
| <img src="docs/assets/telegram.png" width="18" height="18" align="absmiddle" alt="Telegram">&nbsp;Telegram | Publicaciones de un canal público | `/rss/telegram/{channel}` | `/api/v1/telegram?channel=` |
| <img src="docs/assets/apple.png" width="18" height="18" align="absmiddle" alt="Apple Podcasts">&nbsp;Apple Podcasts | Episodios de un programa | `/rss/apple/{id}` | `/api/v1/apple?id=` |
| <img src="docs/assets/apple.png" width="18" height="18" align="absmiddle" alt="Apple Podcasts">&nbsp;Apple Podcasts | Los más escuchados | `/rss/apple/top` | `/api/v1/apple?chart=top&country=` |
| <img src="docs/assets/pinterest.png" width="18" height="18" align="absmiddle" alt="Pinterest">&nbsp;Pinterest | Pines de un perfil o tablero | `/rss/pinterest/{user}`, `/rss/pinterest/{user}/{board}` | `/api/v1/pinterest?user=&board=` |
| <img src="docs/assets/polymarket.png" width="18" height="18" align="absmiddle" alt="Polymarket">&nbsp;Polymarket | Eventos nuevos o en tendencia | `/rss/polymarket/new`, `/rss/polymarket/hot` | `/api/v1/polymarket?feed=new\|hot` |
| <img src="docs/assets/polymarket.png" width="18" height="18" align="absmiddle" alt="Polymarket">&nbsp;Polymarket | Cuotas de un evento | `/rss/polymarket/event/{slug}` | `/api/v1/polymarket?feed=event&slug=` |
| <img src="docs/assets/polymarket.png" width="18" height="18" align="absmiddle" alt="Polymarket">&nbsp;Polymarket | Actividad de una cartera | `/rss/polymarket/user/{address}` | `/api/v1/polymarket?feed=user&address=` |
| <img src="docs/assets/linuxdo.png" width="18" height="18" align="absmiddle" alt="LINUX DO">&nbsp;LINUX DO | Temas del foro (recientes / top / populares / categoría / etiqueta) | `/rss/linuxdo`, `/rss/linuxdo/top`, `/rss/linuxdo/hot`, `/rss/linuxdo/c/{slug}/{id}`, `/rss/linuxdo/tag/{tag}` | `/api/v1/linuxdo?feed=` |
| <img src="docs/assets/reddit.png" width="18" height="18" align="absmiddle" alt="Reddit">&nbsp;Reddit | Listado de un subreddit | `/rss/reddit/{sub}` | `/api/v1/reddit?sub=` |

Parámetros, fuentes de datos y límites están en **[docs/PLATFORMS.md](docs/PLATFORMS.md)**.

## Características

- **RSS completo**: HTML del cuerpo, imágenes (dimensiones reales y carga diferida), citas de reenvíos, tarjetas de enlace y texto largo íntegro; `content:encoded` y Media RSS en el mismo documento, con `enclosure` para los vídeos de Weibo.
- **Suave con los orígenes**: caché en disco que se ensancha según `limit` (una petición pequeña no recorta una caché más amplia) y single-flight; la concurrencia de Weibo se queda en 1, las cookies se renuevan ante 418/403 y un fallo persistente abre un cortacircuitos de 10 minutos.
- **Dos vistas**: RSS para lectores, JSON para scripts y agents (una línea por respuesta).
- **Un solo binario**: enlazado estáticamente (CGO desactivado), unos 8.3 MB y unos 11 MB de memoria residente.

## Instalación

Los binarios estáticos para Linux, macOS y Windows (amd64/arm64) y los paquetes
Debian/RPM van adjuntos a cada
[release](https://github.com/yilong-musk/feedmesh/releases); verifique los archivos
con `checksums.txt`. Con una cadena de herramientas Go (1.27 o posterior), el paquete
principal vive en `cmd/feedmesh`, así que la ruta de instalación lo incluye:

```bash
go install github.com/yilong-musk/feedmesh/cmd/feedmesh@latest
```

### Homebrew

```bash
brew install --cask yilong-musk/tap/feedmesh
brew upgrade --cask feedmesh
```

El cask lo publica el proceso de release en [yilong-musk/homebrew-tap](https://github.com/yilong-musk/homebrew-tap). Instala el binario precompilado de macOS o Linux y quita el atributo de quarantine (los archivos no están notarizados).

### Docker

```bash
docker run -d --name feedmesh -p 127.0.0.1:3000:3000 \
  -v feedmesh-data:/data ghcr.io/yilong-musk/feedmesh:latest
```

Con cada release se publican imágenes `linux/amd64` y `linux/arm64`. Se ejecutan como usuario no root y guardan la caché en el volumen `/data`. Todos los providers funcionan excepto Douyin, cuyo lector necesita Chromium (`FEEDMESH_CHROME`).

## Inicio rápido

Requiere Go 1.27 o posterior.

```bash
git clone https://github.com/yilong-musk/feedmesh.git
cd feedmesh

make check        # go vet + go test + gofmt
make install      # instala en ~/.local/bin/feedmesh
feedmesh serve --listen 127.0.0.1:3000
```

Pruébelo:

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

### systemd (unidad de usuario)

```bash
mkdir -p ~/.config/systemd/user
cp deploy/feedmesh.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now feedmesh
loginctl enable-linger "$USER"     # sigue en marcha sin una sesión activa
```

La unidad usa la expansión `%h` / `%C` de systemd para el directorio personal y el de caché, así que no hace falta sustituir el nombre de usuario; ajuste `ExecStart` si el binario está en otro sitio.

## CLI

| Comando | Qué hace |
|---|---|
| `serve --listen <addr>` | Arranca el servidor HTTP; `--token` / `--rate-limit` son opcionales |
| `fetch <uid\|domain>` | Imprime la cronología de una cuenta de Weibo (RSS; `--json` imprime una sola línea JSON) |
| `doctor [--uid <uid>]` | Prueba visitor, getIndex y statuses, en ese orden |
| `doctor --x-handle <handle>` | Prueba el grupo de instancias de X y cada espejo para un handle |
| `cache stat` | Número y tamaño de las entradas de caché |
| `cache purge --all` | Borra todas las entradas de caché de feedmesh |
| `cache purge --uid <uid>` | Borra las entradas profile/timeline/domain/feed/texto largo de una cuenta |
| `cache purge --handle <handle>` | Borra las entradas de cronología de un handle de X |
| `--dry-run` | Solo informa; no borra nada |

Indicadores globales: `--json`, `--quiet`, `--timeout`, `--cache-dir`, `--refresh`. `--timeout` vale por defecto 9 s para Weibo, 15 s para sspai/V2EX/SMZDM/Reddit/Pinterest/Polymarket, 20 s para X/YouTube, 30 s para TikTok, 40 s para Bilibili, 45 s para Douyin (un navegador en frío tarda alrededor de medio minuto), 15 s para Telegram, 20 s para LINUX DO y 20 s para Apple Podcasts; un valor explícito sustituye a todos los providers, pero un intento de X nunca supera los 25 s. stdout solo lleva datos; stderr solo lleva líneas de registro JSON.

## Documentación

El inglés es la fuente de verdad; esta página es su traducción al español.

- **[API](docs/API.md)**: endpoints, parámetros, formas JSON, códigos de error, detalles de la salida RSS, TTL de caché
- **[Plataformas](docs/PLATFORMS.md)**: reglas y límites de cada fuente, y los pasos para añadir un provider
- **[Arquitectura y operaciones](docs/ARCHITECTURE.md)**: implementación de la caché, política de orígenes, seguridad y privacidad, disposición, resolución de problemas, comandos de desarrollo
- **[English](README.md)**: el original en inglés, única fuente de verdad (esta página es su traducción al español)

## License

[MIT](LICENSE)

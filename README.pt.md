# feedmesh

[![CI](https://github.com/yilong-musk/feedmesh/actions/workflows/ci.yml/badge.svg)](https://github.com/yilong-musk/feedmesh/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

[Deutsch](README.de.md) | [English](README.md) | [Español](README.es.md) | [Français](README.fr.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | Português | [Русский](README.ru.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md)

Reúne conteúdo de várias plataformas em RSS — autoalojado, um único binário estático, sem dependências de terceiros (não há um único `require` no `go.mod`), a escutar apenas `127.0.0.1` por omissão. As plataformas suportadas estão abaixo e em pormenor em [docs/PLATFORMS.md](docs/PLATFORMS.md); as novas chegam como providers.

## Plataformas suportadas

| Plataforma&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp; | Conteúdo | RSS | JSON |
|---|---|---|---|
| <img src="docs/assets/weibo.png" width="18" height="18" align="absmiddle" alt="Weibo">&nbsp;Weibo | Cronologia de uma conta | `/rss/user/{uid}`, `/rss/domain/{name}` | `/api/v1/feed?uid=` |
| <img src="docs/assets/weibo.png" width="18" height="18" align="absmiddle" alt="Weibo">&nbsp;Weibo | Pesquisa em alta em tempo real | `/rss/hot` | `/api/v1/hot` |
| <img src="docs/assets/sspai.png" width="18" height="18" align="absmiddle" alt="sspai">&nbsp;sspai | Artigos mais recentes | `/rss/sspai` | `/api/v1/sspai` |
| <img src="docs/assets/v2ex.png" width="18" height="18" align="absmiddle" alt="V2EX">&nbsp;V2EX | Tópicos mais recentes | `/rss/v2ex` | `/api/v1/v2ex?section=latest` |
| <img src="docs/assets/v2ex.png" width="18" height="18" align="absmiddle" alt="V2EX">&nbsp;V2EX | Tópicos populares | `/rss/v2ex/hot` | `/api/v1/v2ex?section=hot` |
| <img src="docs/assets/smzdm.png" width="18" height="18" align="absmiddle" alt="SMZDM">&nbsp;SMZDM | Ofertas selecionadas | `/rss/smzdm` | `/api/v1/smzdm` |
| <img src="docs/assets/smzdm.png" width="18" height="18" align="absmiddle" alt="SMZDM">&nbsp;SMZDM | Artigos da comunidade | `/rss/smzdm/post` | `/api/v1/smzdm?channel=post` |
| <img src="docs/assets/smzdm.png" width="18" height="18" align="absmiddle" alt="SMZDM">&nbsp;SMZDM | Ofertas do estrangeiro | `/rss/smzdm/haitao` | `/api/v1/smzdm?channel=haitao` |
| <img src="docs/assets/smzdm.png" width="18" height="18" align="absmiddle" alt="SMZDM">&nbsp;SMZDM | Bons preços por categoria | `/rss/smzdm/fenlei/{slug}` | `/api/v1/smzdm?channel=fenlei&slug=` |
| <img src="docs/assets/x.png" width="18" height="18" align="absmiddle" alt="X">&nbsp;X | Cronologia de uma conta | `/rss/x/{handle}` | `/api/v1/x?handle=` |
| <img src="docs/assets/youtube.png" width="18" height="18" align="absmiddle" alt="YouTube">&nbsp;YouTube | Carregamentos de um canal | `/rss/youtube/{handle\|id}` | `/api/v1/youtube?ref=` |
| <img src="docs/assets/douyin.png" width="18" height="18" align="absmiddle" alt="Douyin">&nbsp;Douyin | Vídeos de um criador | `/rss/douyin/{sec_uid}` | `/api/v1/douyin?user=` |
| <img src="docs/assets/tiktok.png" width="18" height="18" align="absmiddle" alt="TikTok">&nbsp;TikTok | Vídeos de um criador | `/rss/tiktok/{handle}` | `/api/v1/tiktok?user=` |
| <img src="docs/assets/bilibili.png" width="18" height="18" align="absmiddle" alt="Bilibili">&nbsp;Bilibili | Vídeos de um criador | `/rss/bilibili/{mid}` | `/api/v1/bilibili?user=` |
| <img src="docs/assets/telegram.png" width="18" height="18" align="absmiddle" alt="Telegram">&nbsp;Telegram | Publicações de um canal público | `/rss/telegram/{channel}` | `/api/v1/telegram?channel=` |
| <img src="docs/assets/apple.png" width="18" height="18" align="absmiddle" alt="Apple Podcasts">&nbsp;Apple Podcasts | Episódios de um programa | `/rss/apple/{id}` | `/api/v1/apple?id=` |
| <img src="docs/assets/apple.png" width="18" height="18" align="absmiddle" alt="Apple Podcasts">&nbsp;Apple Podcasts | Mais ouvidos | `/rss/apple/top` | `/api/v1/apple?chart=top&country=` |
| <img src="docs/assets/pinterest.png" width="18" height="18" align="absmiddle" alt="Pinterest">&nbsp;Pinterest | Pins de um perfil ou quadro | `/rss/pinterest/{user}`, `/rss/pinterest/{user}/{board}` | `/api/v1/pinterest?user=&board=` |
| <img src="docs/assets/polymarket.png" width="18" height="18" align="absmiddle" alt="Polymarket">&nbsp;Polymarket | Eventos novos ou em tendência | `/rss/polymarket/new`, `/rss/polymarket/hot` | `/api/v1/polymarket?feed=new\|hot` |
| <img src="docs/assets/polymarket.png" width="18" height="18" align="absmiddle" alt="Polymarket">&nbsp;Polymarket | Odds de um evento | `/rss/polymarket/event/{slug}` | `/api/v1/polymarket?feed=event&slug=` |
| <img src="docs/assets/polymarket.png" width="18" height="18" align="absmiddle" alt="Polymarket">&nbsp;Polymarket | Atividade de uma carteira | `/rss/polymarket/user/{address}` | `/api/v1/polymarket?feed=user&address=` |
| <img src="docs/assets/linuxdo.png" width="18" height="18" align="absmiddle" alt="LINUX DO">&nbsp;LINUX DO | Tópicos do fórum (recentes / topo / populares / categoria / etiqueta) | `/rss/linuxdo`, `/rss/linuxdo/top`, `/rss/linuxdo/hot`, `/rss/linuxdo/c/{slug}/{id}`, `/rss/linuxdo/tag/{tag}` | `/api/v1/linuxdo?feed=` |
| <img src="docs/assets/reddit.png" width="18" height="18" align="absmiddle" alt="Reddit">&nbsp;Reddit | Lista de um subreddit | `/rss/reddit/{sub}` | `/api/v1/reddit?sub=` |

Parâmetros, fontes de dados e limites estão em **[docs/PLATFORMS.md](docs/PLATFORMS.md)**.

## Funcionalidades

- **RSS completo**: HTML do corpo, imagens (dimensões reais e carregamento diferido), citações de republicação, cartões de ligação e texto longo integral; `content:encoded` e Media RSS no mesmo documento, com `enclosure` para vídeos do Weibo.
- **Gentil com as origens**: cache em disco que alarga conforme o `limit` (um pedido pequeno não corta um cache mais largo) e single-flight; a concorrência do Weibo fica em 1, os cookies renovam-se em 418/403, e uma falha persistente abre um disjuntor de 10 minutos.
- **Duas vistas**: RSS para leitores, JSON para scripts e agents (uma linha por resposta).
- **Binário único**: ligado estaticamente (CGO desligado), cerca de 8,3 MB, cerca de 11 MB de memória residente.

## Instalação

Binários estáticos para Linux, macOS e Windows (amd64/arm64) e pacotes Debian/RPM
vêm anexados a cada [release](https://github.com/yilong-musk/feedmesh/releases); verifique
os arquivos com `checksums.txt`. Com uma cadeia Go (1.27+), o pacote principal está
em `cmd/feedmesh`, por isso o caminho de instalação inclui-o:

```bash
go install github.com/yilong-musk/feedmesh/cmd/feedmesh@latest
```

### Homebrew

```bash
brew install --cask yilong-musk/tap/feedmesh
brew upgrade --cask feedmesh
```

O cask é publicado em [yilong-musk/homebrew-tap](https://github.com/yilong-musk/homebrew-tap) pelo pipeline de release. Instala o binário pré-compilado de macOS ou Linux e remove o atributo quarantine (os arquivos não estão notarizados).

### Docker

```bash
docker run -d --name feedmesh -p 127.0.0.1:3000:3000 \
  -v feedmesh-data:/data ghcr.io/yilong-musk/feedmesh:latest
```

Imagens `linux/amd64` e `linux/arm64` saem com cada release, correm como utilizador não root e guardam a cache no volume `/data`. Todos os providers funcionam exceto o Douyin, cujo leitor precisa de Chromium (`FEEDMESH_CHROME`).

## Início rápido

Requer Go 1.27 ou mais recente.

```bash
git clone https://github.com/yilong-musk/feedmesh.git
cd feedmesh

make check        # go vet + go test + gofmt
make install      # instala em ~/.local/bin/feedmesh
feedmesh serve --listen 127.0.0.1:3000
```

Experimentar:

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

### systemd (unidade de utilizador)

```bash
mkdir -p ~/.config/systemd/user
cp deploy/feedmesh.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now feedmesh
loginctl enable-linger "$USER"     # continua sem uma sessão ativa
```

A unidade usa a expansão `%h` / `%C` do systemd para os diretórios home e de cache, por isso não é preciso substituir o nome de utilizador; ajuste `ExecStart` se o binário estiver noutro sítio.

## CLI

| Comando | O que faz |
|---|---|
| `serve --listen <addr>` | Arranca o servidor HTTP; `--token` / `--rate-limit` são opcionais |
| `fetch <uid\|domain>` | Imprime a cronologia de uma conta Weibo (RSS; `--json` imprime uma linha JSON) |
| `doctor [--uid <uid>]` | Sonda visitor, getIndex e statuses, por esta ordem |
| `doctor --x-handle <handle>` | Sonda o conjunto de instâncias X e cada espelho para um handle |
| `cache stat` | Número e tamanho das entradas de cache |
| `cache purge --all` | Remove todas as entradas de cache do feedmesh |
| `cache purge --uid <uid>` | Remove as entradas profile/timeline/domain/feed/texto longo de uma conta |
| `cache purge --handle <handle>` | Remove as entradas de cronologia de um handle X |
| `--dry-run` | Só reporta, não apaga nada |

Flags globais: `--json`, `--quiet`, `--timeout`, `--cache-dir`, `--refresh`. `--timeout` é por omissão 9 s para o Weibo, 15 s para sspai/V2EX/SMZDM/Reddit/Pinterest/Polymarket, 20 s para X/YouTube, 30 s para o TikTok, 40 s para o Bilibili, 45 s para o Douyin (um browser frio demora cerca de meio minuto), 15 s para o Telegram, 20 s para o LINUX DO e 20 s para o Apple Podcasts; um valor explícito substitui todos os providers, mas uma tentativa X nunca passa de 25 s. O stdout leva só dados; o stderr leva só linhas de registo JSON.

## Documentação

O inglês é a fonte de verdade; esta página é a tradução em português.

- **[API](docs/API.md)**: endpoints, parâmetros, formas JSON, códigos de erro, detalhes da saída RSS, TTL de cache
- **[Plataformas](docs/PLATFORMS.md)**: regras e limites por fonte, e os passos para acrescentar um provider
- **[Arquitetura e operações](docs/ARCHITECTURE.md)**: implementação da cache, política de upstream, segurança e privacidade, disposição, resolução de problemas, comandos de desenvolvimento
- **[English](README.md)**: o original em inglês, única fonte de verdade (esta página é a tradução em português)

## License

[MIT](LICENSE)

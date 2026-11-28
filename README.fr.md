# feedmesh

[![CI](https://github.com/yilong-musk/feedmesh/actions/workflows/ci.yml/badge.svg)](https://github.com/yilong-musk/feedmesh/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

[Deutsch](README.de.md) | [English](README.md) | [Español](README.es.md) | Français | [日本語](README.ja.md) | [한국어](README.ko.md) | [Português](README.pt.md) | [Русский](README.ru.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md)

Rassemble le contenu de plusieurs plateformes en RSS : auto-hébergé, un seul binaire statique, aucune dépendance tierce (il n'y a pas un seul `require` dans `go.mod`), à l'écoute de `127.0.0.1` seulement par défaut. Les plateformes prises en charge sont listées ci-dessous et détaillées dans [docs/PLATFORMS.md](docs/PLATFORMS.md) ; les nouvelles arrivent sous forme de providers.

## Plateformes prises en charge

| Plateforme&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp; | Contenu | RSS | JSON |
|---|---|---|---|
| <img src="docs/assets/weibo.png" width="18" height="18" align="absmiddle" alt="Weibo">&nbsp;Weibo | Fil d'un compte | `/rss/user/{uid}`, `/rss/domain/{name}` | `/api/v1/feed?uid=` |
| <img src="docs/assets/weibo.png" width="18" height="18" align="absmiddle" alt="Weibo">&nbsp;Weibo | Recherches tendances en temps réel | `/rss/hot` | `/api/v1/hot` |
| <img src="docs/assets/sspai.png" width="18" height="18" align="absmiddle" alt="sspai">&nbsp;sspai | Derniers articles | `/rss/sspai` | `/api/v1/sspai` |
| <img src="docs/assets/v2ex.png" width="18" height="18" align="absmiddle" alt="V2EX">&nbsp;V2EX | Derniers sujets | `/rss/v2ex` | `/api/v1/v2ex?section=latest` |
| <img src="docs/assets/v2ex.png" width="18" height="18" align="absmiddle" alt="V2EX">&nbsp;V2EX | Sujets populaires | `/rss/v2ex/hot` | `/api/v1/v2ex?section=hot` |
| <img src="docs/assets/smzdm.png" width="18" height="18" align="absmiddle" alt="SMZDM">&nbsp;SMZDM | Bons plans sélectionnés | `/rss/smzdm` | `/api/v1/smzdm` |
| <img src="docs/assets/smzdm.png" width="18" height="18" align="absmiddle" alt="SMZDM">&nbsp;SMZDM | Articles de la communauté | `/rss/smzdm/post` | `/api/v1/smzdm?channel=post` |
| <img src="docs/assets/smzdm.png" width="18" height="18" align="absmiddle" alt="SMZDM">&nbsp;SMZDM | Bons plans importés | `/rss/smzdm/haitao` | `/api/v1/smzdm?channel=haitao` |
| <img src="docs/assets/smzdm.png" width="18" height="18" align="absmiddle" alt="SMZDM">&nbsp;SMZDM | Bons prix par catégorie | `/rss/smzdm/fenlei/{slug}` | `/api/v1/smzdm?channel=fenlei&slug=` |
| <img src="docs/assets/x.png" width="18" height="18" align="absmiddle" alt="X">&nbsp;X | Fil d'un compte | `/rss/x/{handle}` | `/api/v1/x?handle=` |
| <img src="docs/assets/youtube.png" width="18" height="18" align="absmiddle" alt="YouTube">&nbsp;YouTube | Mises en ligne d'une chaîne | `/rss/youtube/{handle\|id}` | `/api/v1/youtube?ref=` |
| <img src="docs/assets/douyin.png" width="18" height="18" align="absmiddle" alt="Douyin">&nbsp;Douyin | Vidéos d'un créateur | `/rss/douyin/{sec_uid}` | `/api/v1/douyin?user=` |
| <img src="docs/assets/tiktok.png" width="18" height="18" align="absmiddle" alt="TikTok">&nbsp;TikTok | Vidéos d'un créateur | `/rss/tiktok/{handle}` | `/api/v1/tiktok?user=` |
| <img src="docs/assets/bilibili.png" width="18" height="18" align="absmiddle" alt="Bilibili">&nbsp;Bilibili | Vidéos d'un créateur | `/rss/bilibili/{mid}` | `/api/v1/bilibili?user=` |
| <img src="docs/assets/telegram.png" width="18" height="18" align="absmiddle" alt="Telegram">&nbsp;Telegram | Messages d'un canal public | `/rss/telegram/{channel}` | `/api/v1/telegram?channel=` |
| <img src="docs/assets/apple.png" width="18" height="18" align="absmiddle" alt="Apple Podcasts">&nbsp;Apple Podcasts | Épisodes d'une émission | `/rss/apple/{id}` | `/api/v1/apple?id=` |
| <img src="docs/assets/apple.png" width="18" height="18" align="absmiddle" alt="Apple Podcasts">&nbsp;Apple Podcasts | Classement | `/rss/apple/top` | `/api/v1/apple?chart=top&country=` |
| <img src="docs/assets/pinterest.png" width="18" height="18" align="absmiddle" alt="Pinterest">&nbsp;Pinterest | Épingles d'un profil ou d'un tableau | `/rss/pinterest/{user}`, `/rss/pinterest/{user}/{board}` | `/api/v1/pinterest?user=&board=` |
| <img src="docs/assets/polymarket.png" width="18" height="18" align="absmiddle" alt="Polymarket">&nbsp;Polymarket | Événements nouveaux ou tendance | `/rss/polymarket/new`, `/rss/polymarket/hot` | `/api/v1/polymarket?feed=new\|hot` |
| <img src="docs/assets/polymarket.png" width="18" height="18" align="absmiddle" alt="Polymarket">&nbsp;Polymarket | Cotes d'un événement | `/rss/polymarket/event/{slug}` | `/api/v1/polymarket?feed=event&slug=` |
| <img src="docs/assets/polymarket.png" width="18" height="18" align="absmiddle" alt="Polymarket">&nbsp;Polymarket | Activité d'un portefeuille | `/rss/polymarket/user/{address}` | `/api/v1/polymarket?feed=user&address=` |
| <img src="docs/assets/linuxdo.png" width="18" height="18" align="absmiddle" alt="LINUX DO">&nbsp;LINUX DO | Sujets du forum (récents / top / populaires / catégorie / tag) | `/rss/linuxdo`, `/rss/linuxdo/top`, `/rss/linuxdo/hot`, `/rss/linuxdo/c/{slug}/{id}`, `/rss/linuxdo/tag/{tag}` | `/api/v1/linuxdo?feed=` |
| <img src="docs/assets/reddit.png" width="18" height="18" align="absmiddle" alt="Reddit">&nbsp;Reddit | Liste d'un subreddit | `/rss/reddit/{sub}` | `/api/v1/reddit?sub=` |

Les paramètres, les sources et les limites sont dans **[docs/PLATFORMS.md](docs/PLATFORMS.md)**.

## Fonctions

- **RSS complet** : HTML du corps, images (dimensions réelles et chargement différé), citations de republication, cartes de lien, texte long intégral ; `content:encoded` et Media RSS dans le même document, avec `enclosure` pour les vidéos Weibo.
- **Doux avec les sources** : cache disque élargi selon `limit` (une petite requête ne tronque pas un cache plus large) et single-flight ; la concurrence Weibo reste à 1, les cookies sont renouvelés sur 418/403, et un échec persistant ouvre un disjoncteur de 10 minutes.
- **Deux vues** : RSS pour les lecteurs, JSON pour les scripts et les agents (une ligne par réponse).
- **Binaire unique** : lié statiquement (CGO désactivé), environ 8,3 Mo, environ 11 Mo de mémoire résidente.

## Installation

Les binaires statiques pour Linux, macOS et Windows (amd64/arm64) et les paquets
Debian/RPM sont joints à chaque
[release](https://github.com/yilong-musk/feedmesh/releases) ; vérifiez les archives
avec `checksums.txt`. Avec une chaîne Go (1.27+), le paquet principal se trouve
dans `cmd/feedmesh`, donc le chemin d'installation l'inclut :

```bash
go install github.com/yilong-musk/feedmesh/cmd/feedmesh@latest
```

### Homebrew

```bash
brew install --cask yilong-musk/tap/feedmesh
brew upgrade --cask feedmesh
```

Le cask est publié sur [yilong-musk/homebrew-tap](https://github.com/yilong-musk/homebrew-tap) par le pipeline de release. Il installe le binaire macOS ou Linux précompilé et retire l'attribut quarantine (les archives ne sont pas notarisées).

### Docker

```bash
docker run -d --name feedmesh -p 127.0.0.1:3000:3000 \
  -v feedmesh-data:/data ghcr.io/yilong-musk/feedmesh:latest
```

Des images `linux/amd64` et `linux/arm64` sont publiées à chaque release. Elles s'exécutent en utilisateur non root et gardent le cache dans le volume `/data`. Tous les providers fonctionnent sauf Douyin, dont le lecteur a besoin de Chromium (`FEEDMESH_CHROME`).

## Démarrage rapide

Go 1.27 ou plus récent est requis.

```bash
git clone https://github.com/yilong-musk/feedmesh.git
cd feedmesh

make check        # go vet + go test + gofmt
make install      # installe dans ~/.local/bin/feedmesh
feedmesh serve --listen 127.0.0.1:3000
```

Essayer :

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

### systemd (unité utilisateur)

```bash
mkdir -p ~/.config/systemd/user
cp deploy/feedmesh.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now feedmesh
loginctl enable-linger "$USER"     # continue sans session active
```

L'unité utilise l'expansion `%h` / `%C` de systemd pour les répertoires personnel et de cache : aucune substitution de nom d'utilisateur n'est nécessaire. Ajustez `ExecStart` si le binaire est ailleurs.

## CLI

| Commande | Rôle |
|---|---|
| `serve --listen <addr>` | Démarre le serveur HTTP ; `--token` / `--rate-limit` sont facultatifs |
| `fetch <uid\|domain>` | Affiche le fil d'un compte Weibo (RSS ; `--json` imprime une seule ligne JSON) |
| `doctor [--uid <uid>]` | Sonde visitor, getIndex et statuses, dans cet ordre |
| `doctor --x-handle <handle>` | Sonde le pool d'instances X et chaque miroir pour un handle |
| `cache stat` | Nombre et taille des entrées de cache |
| `cache purge --all` | Supprime toutes les entrées de cache feedmesh |
| `cache purge --uid <uid>` | Supprime les entrées profile/timeline/domain/feed/texte long d'un compte |
| `cache purge --handle <handle>` | Supprime les entrées de fil d'un handle X |
| `--dry-run` | Rapport seulement, ne supprime rien |

Indicateurs globaux : `--json`, `--quiet`, `--timeout`, `--cache-dir`, `--refresh`. `--timeout` vaut par défaut 9 s pour Weibo, 15 s pour sspai/V2EX/SMZDM/Reddit/Pinterest/Polymarket, 20 s pour X/YouTube, 30 s pour TikTok, 40 s pour Bilibili, 45 s pour Douyin (un navigateur à froid prend environ une demi-minute), 15 s pour Telegram, 20 s pour LINUX DO et 20 s pour Apple Podcasts ; une valeur explicite remplace chaque provider, mais une tentative X ne dépasse jamais 25 s. stdout ne porte que les données ; stderr ne porte que des lignes de journal JSON.

## Documentation

L'anglais est la source de vérité ; cette page en est la traduction française.

- **[API](docs/API.md)** : endpoints, paramètres, formes JSON, codes d'erreur, détails de la sortie RSS, TTL de cache
- **[Plateformes](docs/PLATFORMS.md)** : règles et limites par source, et les étapes pour ajouter un provider
- **[Architecture et exploitation](docs/ARCHITECTURE.md)** : implémentation du cache, politique amont, sécurité et vie privée, disposition, dépannage, commandes de développement
- **[English](README.md)** : l'original anglais, seule source de vérité (cette page en est la traduction française)

## License

[MIT](LICENSE)

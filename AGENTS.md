# AGENTS.md — repository constraints for coding agents

These rules apply to every change in this repository, whether it is made by a
human or by an automated agent. A pull request that breaks a rule is rejected,
not patched up during review.

## 1. English only

All text authored as part of this project must be English. "Authored text"
means everything a maintainer writes:

- identifiers, comments, log lines, error messages, test names and assertions
- CLI help, flags, exit-code descriptions, config keys and their defaults
- README, `docs/`, code comments, `deploy/` unit comments, `Makefile`,
  shell scripts and CI workflow files
- commit messages, branch names, pull-request titles and bodies, issue text

No other language may appear in those places, not even a single word or a
parenthetical translation. Mixed-language files are a violation.

### 1.1 What the rule does not cover

- **Upstream payloads.** Content fetched from Weibo, sspai, V2EX or X is
  Chinese/Japanese/Korean by nature. Test fixtures that reproduce upstream
  responses must stay byte-faithful and are treated as data, not as prose.
- **Runtime output.** Anything a feed returns to a reader is data derived from
  upstream or from the string table in §1.2 — never an inline literal.
- **Third-party files.** `LICENSE` text and vendored assets keep their
  original wording.

### 1.2 User-facing strings

- The default output language is English.
- Every user-facing string (channel titles, descriptions, labels, error text
  shown to readers) lives in one string table with stable keys. Business
  logic must not branch on a language name.
- A translation is a locale pack, not an edit of the English table.

### 1.3 Localized versions

A non-English version is allowed only as a separate, clearly named artifact:

| Kind | Path |
|---|---|
| README | `README.<tag>.md` (e.g. `README.zh-CN.md`) |
| Docs | `docs/<tag>/…` (e.g. `docs/zh-CN/API.md`) |
| Runtime locale pack | `internal/i18n/<tag>.json` (e.g. `internal/i18n/zh-CN.json`) |

`<tag>` is a BCP 47 language tag. Rules for localized artifacts:

1. English is the source of truth; it must exist before a translation.
2. A localized file is self-contained and never mixed into an English file.
3. When the English text changes meaning, its translations are updated or
   deleted in the same pull request.
4. Localized files are listed in the docs index; they never replace the
   English entry point.

## 2. Everything else stays as it is

This file only adds the language rule. Existing conventions still apply:
standard-library-only Go, a single static binary (`CGO_ENABLED=0`), the
committed CI gates, and the deployment notes in `docs/ARCHITECTURE.md`.

## 3. Enforcement

`make lang` (also wired into `make check` and the CI job) runs
`scripts/check_language.py`, which fails when a tracked file contains
Cyrillic, Hebrew, Arabic, Devanagari, Thai, Kana, CJK or Hangul characters in
authored text.

| Exemption | Reason |
|---|---|
| `*_test.go` | fixtures reproduce upstream payloads byte for byte |
| a line containing `i18n-ignore` | upstream identifiers that must keep their original form, e.g. a node name |
| `docs/<tag>/`, `README.<tag>.md`, `internal/i18n/<tag>.json` | localized versions, allowed by §1.3 |
| `docs/assets/`, image and PDF files, `LICENSE` | binaries and third-party text |

Anything outside those exemptions is a build failure, not a review comment.
When an upstream identifier must stay as-is, add the marker and a one-line
reason next to it:

```go
const promotedNode = "<upstream node title>" // i18n-ignore: upstream node title
```


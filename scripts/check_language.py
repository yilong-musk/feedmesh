#!/usr/bin/env python3
"""Fail when non-English scripts show up in authored text.

AGENTS.md §1 requires every authored string in this repository to be English.
This check scans tracked files for characters outside the Latin script and
reports them, apart from:

  * test files (`*_test.go`), which reproduce upstream payloads byte for byte
  * lines carrying an explicit `i18n-ignore` marker (upstream identifiers that
    must keep their original form, e.g. a node name or a vendor icon label)
  * binary assets and the license text

Run it directly or through `make lang`, which CI also executes.
"""

from __future__ import annotations

import re
import subprocess
import sys

# Writing systems that must not appear in authored text.
RANGES = [
    (0x0400, 0x04FF, "Cyrillic"),
    (0x0590, 0x05FF, "Hebrew"),
    (0x0600, 0x06FF, "Arabic"),
    (0x0900, 0x097F, "Devanagari"),
    (0x0E00, 0x0E7F, "Thai"),
    (0x3040, 0x30FF, "Kana"),
    (0x3400, 0x4DBF, "CJK"),
    (0x4E00, 0x9FFF, "CJK"),
    (0xAC00, 0xD7AF, "Hangul"),
    (0xF900, 0xFAFF, "CJK"),
]

SKIP_SUFFIXES = (".png", ".jpg", ".jpeg", ".gif", ".webp", ".ico", ".pdf")
SKIP_PATHS = ("docs/assets/",)
SKIP_FILES = {"LICENSE"}
MARKER = "i18n-ignore"

# Localized artifacts are allowed by AGENTS.md §1.3; they are separate files,
# never edits of the English originals.
TAG = r"[a-z]{2,3}(?:-[A-Za-z0-9]{2,8})*"
LOCALIZED = (
    re.compile(rf"^docs/{TAG}/"),
    re.compile(rf"^README\.{TAG}\.md$"),
    re.compile(rf"^internal/i18n/{TAG}\.json$"),
)


def tracked_files() -> list[str]:
    out = subprocess.run(
        ["git", "ls-files", "-z"], check=True, capture_output=True
    ).stdout
    return [p for p in out.decode("utf-8", "replace").split("\0") if p]


def skipped(path: str) -> bool:
    if path in SKIP_FILES or path.startswith(SKIP_PATHS):
        return True
    if path.endswith(SKIP_SUFFIXES):
        return True
    if any(p.match(path) for p in LOCALIZED):
        return True
    # Upstream fixtures live inline in tests; they are data, not prose.
    return path.endswith("_test.go")


def offending(text: str) -> list[str]:
    names = set()
    for ch in text:
        cp = ord(ch)
        for lo, hi, name in RANGES:
            if lo <= cp <= hi:
                names.add(name)
                break
    return sorted(names)


def main() -> int:
    hits = 0
    for path in tracked_files():
        if skipped(path):
            continue
        try:
            with open(path, encoding="utf-8") as fh:
                lines = fh.read().splitlines()
        except (OSError, UnicodeDecodeError):
            continue
        for number, line in enumerate(lines, 1):
            if MARKER in line:
                continue
            scripts = offending(line)
            if not scripts:
                continue
            hits += 1
            print(f"{path}:{number}: {'/'.join(scripts)} text: {line.strip()[:120]}")
    if hits:
        print(
            f"\n{hits} line(s) contain non-English scripts. See AGENTS.md §1: "
            "authored text stays English, translations live in separate "
            "localized files, and upstream data uses an i18n-ignore marker.",
            file=sys.stderr,
        )
        return 1
    print("language check: no non-English scripts in authored text")
    return 0


if __name__ == "__main__":
    sys.exit(main())

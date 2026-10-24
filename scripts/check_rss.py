#!/usr/bin/env python3
"""Validate the rendered RSS: well-formedness + field sanity."""
import sys
import xml.etree.ElementTree as ET

if len(sys.argv) < 2:
    sys.exit("usage: check_rss.py <feed.xml>")
path = sys.argv[1]
tree = ET.parse(path)
root = tree.getroot()
print("root tag:", root.tag, "version:", root.get("version"))

ch = root.find("channel")
print("channel title:", ch.findtext("title"))
print("ttl:", ch.findtext("ttl"))
print("link:", ch.findtext("link"))
items = ch.findall("item")
print("items:", len(items))

bad = []
required = ["title", "link", "guid", "description", "pubDate"]
for i, it in enumerate(items):
    for r in required:
        if it.find(r) is None:
            bad.append(f"item {i}: missing {r}")
    desc = it.findtext("description") or ""
    if "]]>" in desc:
        bad.append(f"item {i}: description contains a bare ]]>")
    guid = it.find("guid")
    if guid is not None and guid.get("isPermaLink") != "true":
        bad.append(f"item {i}: guid isPermaLink missing")

first = items[0]
print("\nfirst item:")
print("  title:", (first.findtext("title") or "")[:60])
print("  link :", first.findtext("link"))
print("  date :", first.findtext("pubDate"))
print("  desc :", (first.findtext("description") or "")[:120].replace("\n", " "))

# Every description must round-trip as text (CDATA content is plain char data).
empties = sum(1 for it in items if not (it.findtext("description") or "").strip())
print("\nitems with empty description:", empties)
print("problems:", bad if bad else "none")
sys.exit(1 if bad else 0)

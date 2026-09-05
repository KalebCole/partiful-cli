#!/usr/bin/env python3
"""PROTOTYPE: locate current Partiful Explore callable names in public JS."""

from __future__ import annotations

import re
import sys
import urllib.parse
import urllib.request

BASE = "https://partiful.com"
NEEDLES = (
    "getDiscoverFeed",
    "getDiscoverSections",
    "getDiscoverSection",
    "getDiscoverEventItemDecorators",
    "allowedFeedPresentationStyles",
    "allowedSectionPresentationStyles",
    "DISCOVER_HOME",
    "Seattle",
    '"SEA"',
    "SEATTLE",
)


def fetch(url: str) -> str:
    request = urllib.request.Request(url, headers={"User-Agent": "partiful-cli-prototype/1"})
    with urllib.request.urlopen(request, timeout=30) as response:
        return response.read().decode("utf-8", errors="replace")


def main() -> int:
    html = fetch(f"{BASE}/explore")
    sources = sorted(set(re.findall(r'<script[^>]+src="([^"]+)"', html)))
    found: set[str] = set()
    discover_callables: set[str] = set()
    for source in sources:
        url = urllib.parse.urljoin(BASE, source)
        try:
            body = fetch(url)
        except Exception as exc:  # prototype: keep scanning
            print(f"WARN {url}: {exc}", file=sys.stderr)
            continue
        discover_callables.update(re.findall(r'["\'](getDiscover[A-Za-z0-9]+)["\']', body))
        for needle in NEEDLES:
            start = body.find(needle)
            if start < 0:
                continue
            found.add(needle)
            left = max(0, start - 700)
            right = min(len(body), start + len(needle) + 1200)
            print(f"\n=== {needle} in {url} ===\n{body[left:right]}\n")
    missing = sorted(set(NEEDLES) - found)
    print("\n=== DISCOVER CALLABLES ===")
    for callable_name in sorted(discover_callables):
        print(callable_name)
    if missing:
        print(f"MISSING: {', '.join(missing)}", file=sys.stderr)
    return 0 if found else 1


if __name__ == "__main__":
    raise SystemExit(main())

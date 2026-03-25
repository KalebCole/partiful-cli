---
name: partiful-posters
description: Browse and search the Partiful poster catalog for event imagery
---

# Partiful CLI — Posters

Browse the built-in poster catalog. No auth required — the catalog is public.

## Commands

### List Posters
```bash
partiful posters list
partiful posters list --category "Dinner Party" --limit 5
partiful posters list --type gif --limit 10
```

| Flag | Description |
|------|-------------|
| `--category <name>` | Filter by category (e.g., "Birthday", "Dinner Party") |
| `--type <ext>` | Filter by file type: `png`, `gif`, `jpeg` |
| `--limit <n>` | Max results (default: 20) |

### Search Posters
```bash
partiful posters search "birthday"
partiful posters search "disco" --limit 5
```
Fuzzy search across poster names, tags, and categories.

### Get Poster Details
```bash
partiful posters get <posterId>
```
Returns full metadata: name, categories, tags, preview URL.

## Usage with Events

Once you find a poster ID, pass it to `events create` or `events update`:
```bash
partiful events create --title "Party" --date "2026-04-05T19:00" --poster "piscesairbrush.png"
```

Or skip the manual search — `--poster-search` does it in one step:
```bash
partiful events create --title "Party" --date "2026-04-05T19:00" --poster-search "disco"
```

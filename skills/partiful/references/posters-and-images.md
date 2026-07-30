# Posters and Images

The built-in poster catalog is public and does not require Partiful authentication.

## Browse and Select

```bash
partiful posters list --limit 20
partiful posters list --category "Birthday" --type gif --limit 10
partiful posters search "disco" --limit 5
partiful posters get <poster-id>
```

Search covers names, tags, and categories. Apply a selected poster with either an exact ID or a fuzzy query:

```bash
partiful events create --title "Party" --date "2026-08-01T19:00" --poster <poster-id> --dry-run
partiful events update <event-id> --poster-search "disco" --dry-run
```

## Custom Images

```bash
partiful events create --title "Party" --date "2026-08-01T19:00" --image ./flyer.png --dry-run
partiful events update <event-id> --image "https://example.com/poster.jpg" --dry-run
```

Custom image upload requires authentication and may fail with a `404` even when other operations work. If it does, use a built-in poster through `--poster` or `--poster-search` rather than repeatedly retrying upload.

Only choose one of `--poster`, `--poster-search`, or `--image` per command. Verify the event afterward with `partiful events get <event-id>`.

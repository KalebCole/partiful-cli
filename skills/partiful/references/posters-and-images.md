# Posters and Images

The CLI exposes the built-in poster catalog.

## Browse posters

```bash
partiful posters list
partiful posters search --query "disco"
```

List or search the catalog, choose a `posterId`, then apply it on an event
write.

## Apply a poster ID

```bash
partiful events create   --title "Party"   --start 2026-08-01T19:00:00-07:00   --timezone America/Los_Angeles   --poster-id <poster-id>
```

Add `--dry-run` to `events create` or `events update` to preview the resolved
poster request without sending a write.

## Limits

Custom image upload is unavailable through the CLI. If the user supplies an
image, explain that limitation instead of substituting an unrelated poster.

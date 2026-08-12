# Posters and Images

The approved Go CLI exposes the built-in poster catalog.

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

`events create` and `events update` return a plan until you add
`--apply --plan <token>`.

## Limits

Custom image upload is not part of the approved public Go command catalog.
If the user needs a custom upload path, stop and explain that the current native
CLI does not ship it as a reviewed public command.

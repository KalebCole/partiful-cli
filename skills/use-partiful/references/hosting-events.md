# Hosting events

When creating, save one complete JSON payload and pass `--input` so the schedule, visibility, and description are reviewable before the write. Default `visibility` to `private` unless the user asked for public discovery. A fixed end is a calendar boundary; if the user describes a variable end ("until the game ends"), put that in the description instead of claiming a dynamic end.

See the create/update schema for `posterId`. There is no custom image upload. If the user supplied a cover image, do not substitute an unrelated built-in poster. Create the event, then upload the image in the web editor at `https://partiful.com/events/<eventId>/edit`, and confirm it on `https://partiful.com/e/<eventId>`. Poster search is keyword-picky; try several synonyms before concluding the library has nothing.

After a create, including when the write returns `contract.protocol_changed`, list upcoming with `--all --max-items`, match the exact title and normalized start, require a single record, then `events get` for the authoritative readback. Events publish immediately; a missing page is not a propagation delay. After changing `start` or `end`, confirm the public event page shows a human local date and time, not raw UTC.

Do not use em dashes in titles or descriptions. Match the voice of the user's existing hosted events (list upcoming and past, then `events get` on theirs) rather than generic party copy. Keep transient updates such as weather out of the durable description; send a later blast instead (see `references/guests-and-blasts.md`).

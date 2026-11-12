# Guests and blasts

`guests list` requires host access. As an attendee it fails with `HOST_PERMISSION_REQUIRED` ("This command requires host access to the event."); do not use it to look up your own RSVP. As a host, the current CLI can also fail with `GUESTS_PROTOCOL_CHANGED` ("The guests no longer match the reviewed remote contract."). Report that rather than retrying or scraping. See the protocol-changed rule in `SKILL.md`.

Do not use em dashes in invite messages or blasts. Match the voice of the user's existing hosted events rather than generic party copy. Follow the approval rule in `SKILL.md`. Text blasts go to guests as SMS, so keep them short. Transient updates such as weather belong in a blast, not the durable event description.

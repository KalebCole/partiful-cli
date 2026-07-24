<!-- wayfinder:task -->
# DOCS: update partiful skill + README for explore

Labels: wayfinder:task
Blocked by: IMPLEMENT
Assignee: (unclaimed)
Status: open

## Question

Document the shipped `explore` command so it's discoverable and correct.

Work:
- Update the `partiful` skill SKILL.md (~/.hermes/skills/social-media/partiful/):
  add an "Explore / discovery" section — endpoints, build-id caveat, RSVP-to-
  discovered-event flow, region slugs, tag-filter behavior. Correct the current
  skill's flat claim that there's "no browse public events" command.
- Update repo README + AGENTS.md with the new command group.
- Note any pitfalls found during IMPLEMENT (rotating build id, tag filtering
  quirks) in the skill's Pitfalls section.

Output: skill + README updated, linked commit. This is the last ticket; when it
closes the destination is reached.

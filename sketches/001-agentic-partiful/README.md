## Prototype: Agentic Partiful landing page

One live HTML artifact for the selected **Prompt → Party** direction. A natural-language brief becomes an agent conversation with explicit approval gates; the phone is a synchronized, display-only event preview.

### Interaction

- Run the synthetic agent sequence from the hero prompt. Research, publish approval, group-invite approval, RSVP monitoring, and reminder approval all happen in chat.
- The phone never becomes a workflow surface. It only reflects draft, live, and guest states.
- The conversation stays inside a bounded rail so the phone remains visible as the workflow progresses.

### Product boundary

This is an integration concept for Partiful CLI running inside a capable agent runtime. Partiful CLI provides deterministic event, guest, RSVP, and blast operations. External research, durable scheduling, and future execution come from the surrounding runtime. The CLI itself does not expose a native scheduling command.

### Open locally

```bash
open sketches/001-agentic-partiful/index.html
```

All event data is synthetic. No command executes and no message sends.

# Partiful CLI Web Design System

## Product story

The site presents Partiful CLI as the deterministic tool an agent can use to manage a Partiful event. The story separates two parts:

1. **Partiful CLI** performs event, guest, RSVP, blast, poster, schema, and diagnostic operations.
2. **Bundled skill** gives supported agents command guidance, privacy boundaries, and confirmation rules.

Never imply that an accepted invite or blast request proves delivery. Describe `+watch` as bounded polling, not durable monitoring.

## Visual direction

Friendly editorial minimalism with oversized display typography, high-contrast surfaces, and flat color fields. The page should feel social and optimistic without copying Partiful product chrome.

### Tokens

- Ink: `#17151a`
- Paper: `#f7f3ec`
- White: `#ffffff`
- Purple: `#6f54ee`
- Dark purple: `#4932bb`
- Lime: `#c8f05b`
- Yellow: `#ffd83d`
- Blue: `#8dd8ff`
- Pink: `#ff7caf`
- Orange: `#ff9b61`
- Border: `rgba(23, 21, 26, .13)`

Display typeface: **Bricolage Grotesque**. Body typeface: **Figtree**. Both load from Google Fonts with sans-serif fallbacks. Monospace labels and commands use SFMono-Regular, Menlo, Consolas, or equivalent.

## Composition

- Content width: `1180px` with `24px` desktop gutters and `14px` compact-mobile gutters.
- Hero: centered statement, readable supporting copy, editable synthetic prompt, then revealable workflow.
- Workflow: conversation first, passive phone preview second. On mobile, keep this DOM order and stack vertically.
- Major sections use full-width color fields to make the long page easy to scan.
- Cards use rounded corners only when they represent a contained artifact, command surface, or approval state.

## Core components

### Navigation

Brand at left; section anchors and primary install CTA at right. Collapse links on narrow screens while keeping brand visible.

### Synthetic prompt

Editable textarea with a visible `Synthetic demo · nothing gets sent` label and one action. The interaction must never call a real API.

### Agent conversation

The only workflow control surface. Every consequential operation appears as an approval card with explicit copy and one button. Announce state updates through live regions and move focus to the next approval.

### Phone preview

Display-only artifact. It may show draft, published, invitation-request, RSVP-watch, and reminder states, but must contain no workflow buttons or fake host actions.

### Install tabs

Roving-tabindex tablist for npm, npx, and agent-skill paths. Each panel provides one copyable command, a short prerequisite or follow-up note, and a visible copy confirmation.

### Capability proof

Human-readable capability names pair with exact command evidence. Use concrete terms such as `partiful schema events.create`, `partiful doctor`, `--dry-run`, `--format json`, and `partiful +watch <eventId>`.

## Responsive behavior

- Validate widths at 320, 390, 428, and 1440 CSS pixels.
- No horizontal overflow.
- Hero headline scales with `clamp()` and remains centered.
- Two-column sections become single-column below 760px.
- Conversation precedes phone on mobile.
- Command rows may wrap, but copy controls remain at least 44px square.

## Accessibility

- One `h1`; sequential section headings.
- Skip link to `#main-content`.
- Every button has `type="button"` and an accessible name.
- Tabs expose `tablist`, `tab`, `tabpanel`, `aria-selected`, and roving keyboard focus.
- State updates use `aria-live` where appropriate.
- Visible focus rings use a three-pixel ink outline with a four-pixel offset.
- Respect `prefers-reduced-motion`; reveal content immediately and remove nonessential transitions.
- Decorative SVGs are hidden from assistive technology.

## Motion

Motion explains state changes, never decorates idle surfaces. The initial workflow reveal may use a short stagger. Approval milestones update the phone and move focus. Reduced-motion mode skips delays and scroll animation.

# BUILD-PROMPT: partiful-cli JS → TypeScript Port

Port partiful-cli to TypeScript so the `src/lib/` API layer's endpoint interfaces + Zod response
schemas ARE the living API spec (a byproduct of typing the code, never a separate file that drifts).

## Drive off the map

`.wayfinder/ts-port/map.md` + `.wayfinder/ts-port/tickets/T*.md` are the source of truth for scope,
sequencing, conventions, and per-ticket detail. Work tickets T1→T6 in order (do not reorder), mark
each closed in the map's "Decisions so far" as you finish, and don't invent scope outside the map.
Complete every ticket — a partial port is not done.

## Start

Cut `feat/typescript-port` from `main` at commit `ad7be30` (baseline: `npm test` = 195 green).
Each ticket's gate is `tsc --noEmit` clean AND `npm test` green; keep it green file-by-file.

## Before opening the PR

Run an adversarial review pass (`adversarial-code-review` skill), fix everything blocking, re-review
until the verdict is clean.

## After opening the PR — bot-poll loop

External review bots (CodeRabbit, etc.) run async (`pr-review-bots` skill):
1. Wait, re-check the PR for new bot reviews (`gh pr view`, `gh pr checks`).
2. For each actionable comment: verify against current code, fix valid ones (test first), reply-skip
   invalid ones with a one-line rationale.
3. Push fixes (re-triggers bots), repeat.

**Stop when ANY holds:** (a) latest bot review has zero actionable comments AND checks pass; (b) two
consecutive cycles yield only reasoned-declined comments; (c) 6 cycles elapsed — then summarize the
remaining open threads. Never loop forever.

## Done

Port complete, all gates green, adversarial review clean, bot-poll loop terminated, PR green and
mergeable. Write the final report (before/after test counts, convention deviations with reasons), stop.

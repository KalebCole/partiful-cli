# T1 — TS toolchain setup

**Type:** task (AFK) · **Blocks on:** T0 · **Status:** OPEN

## Question

Stand up the TypeScript toolchain so the rest of the port has a compile target and run path,
without breaking the working CLI.

## Scope

- Add `typescript` + `tsx` (or equivalent) to devDependencies; `@types/node`.
- `tsconfig.json`: `strict: true` (Kaleb's call — full strict from day one, no loose-then-tighten),
  `module`/`moduleResolution` for ESM ("NodeNext"), `noEmit` for the check step, `allowJs: true`
  DURING migration so JS + TS coexist file-by-file.
- Run path: `tsx src/cli.js` works; `bin` still resolves. No dist build required if running via tsx,
  but decide + document build-vs-tsx here.
- Add `npm run typecheck` = `tsc --noEmit` and `npm run build` if a dist is chosen.
- Verify `npm test` still green with the toolchain added (nothing ported yet).

## Done when

`tsconfig.json` exists with strict on, `allowJs` true, `npm run typecheck` passes on the still-JS
tree (or reports only expected allowJs-permitted state), CLI still runs, tests green.

## Answer

<!-- record tsconfig decisions + run/build choice on close -->

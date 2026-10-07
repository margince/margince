# frontend/: the Margince web app

React 19 + Vite + TypeScript (strict) + Tailwind 4 + Biome + Vitest +
Playwright. Margince has its own design system. The catalog is
[src/design-system/README.md](src/design-system/README.md), and visual
direction is in [DESIGN.md](../DESIGN.md).

## Commands

```sh
pnpm install
pnpm dev          # Vite dev server; proxies /v1 to http://localhost:8080 (BACKEND_PORT overrides)
pnpm check        # the frontend gate: Biome + unit tests + tsc + build
pnpm e2e          # build + the Playwright screen-acceptance harness
pnpm gen:api      # regenerate src/api/schema.d.ts from ../backend/api/crm.yaml
```

From the repo root: `make frontend-check`, `make frontend-e2e`, and `make dev`
(the full running stack: api + this SPA). `make check` runs this lane as well as
the Go one. It needs Node 24, the version CI pins, and pnpm.

### UI-preview switches (`VITE_UI_PREVIEW_*`)

Presentation scaffolding for design review, off unless the var is set. They
are not feature flags: they cannot make a flow work, only draw one. They live
in `src/app/ui-preview.ts`; the prefix tells a reader they are previews.

| Var | What it draws |
|---|---|
| `VITE_UI_PREVIEW_OIDC=1` | The federated sign-in buttons on the login screen, with the second provider marked *not yet available*. |
| `VITE_UI_PREVIEW_RESET=1` | The "Forgot password?" link and the request card it opens. |

```sh
pnpm dev:preview                    # every switch on — the demo entry point
pnpm build:preview                  # the same, built
VITE_UI_PREVIEW_OIDC=1 pnpm dev     # login screen, with the SSO block drawn
```

Never ship a preview build: it draws controls the installation cannot honour.
`dev` and `build` never set these switches.

`VITE_UI_PREVIEW_OIDC=1` substitutes two providers in `AuthScreen` when the
server offers none, at the render boundary after the query. Reviewers can then
see the sign-in buttons without configuring a provider. The wire is untouched
and the query cache keeps the server's real answer. `startFederatedSignIn` stays
inert only for a button the preview invented, so an installation that serves
real providers keeps working buttons under the switch. The preview labels are
not translated, because provider labels are server-owned copy. A preview build
logs a one-time `console.warn` saying so.

The same switch marks the second preview provider *not yet available*.
`previewedUnavailableProviders()` returns a set of keys that `ProviderButtons`
renders as a native `disabled` button with an `.is-unavailable` class. The label
is untouched, because the marker must not splice our words onto a label the
installation wrote. A screen reader therefore hears that the control is
unavailable, but not why. Only the preview can produce a marked provider:
`oidc_providers[]` items are `{ key, label }` with no availability field, so in
the product `ProviderButtons` always receives an empty set. The product never
shows a dead provider control (Google, Microsoft, SSO); a marked button is
allowed here for the same reason a Storybook story is.

`VITE_UI_PREVIEW_RESET=1` draws the "Forgot password?" link. The flow is
finished on both sides: `POST /auth/forgot-password` and
`POST /auth/reset-password` in the contract, handlers in
`backend/internal/modules/identity/reset.go`, and all four views on this screen.
`password_reset` is computed live as `h.resetMailer != nil`, so it reports
`false` on the shipped `config/margince.yaml` only because that file has no
`email:` block. The switch draws the link and wires no mailer. The request form
behind it is the real one, so submitting it against a mailer-less installation
gets a `501 not_implemented` back and shows it as the form's failure note. The
confirmation, the deep-link form and the spent-token refusal are asserted in
`src/screens/auth.test.tsx` instead of being reachable here.

Unset, each switch reads `undefined` and nothing changes. Both positions of both
are pinned by `src/app/ui-preview.test.ts` and the `federated sign-in` cases in
`src/screens/auth.test.tsx`. The e2e lane builds without any of them, so
`offers no identity provider that does not work` still measures the real
default.

## Layout

- **Read [`src/design-system/README.md`](src/design-system/README.md) before
  building a control.** It is the catalog. Every interactive control comes from
  that directory; a native `<select>` or a hand-rolled dropdown is a defect, and
  `src/design-system/native-controls.test.ts` (a TypeScript-AST vitest gate, run
  on its own by `make native-controls`) refuses one. `pnpm storybook` shows them
  all.
- `src/design-system/`: tokens, primitives and their stories. Read
  [its README](src/design-system/README.md) before building a control.
- `src/app/`: the shell, the top bar, `nav.ts` and `theme.ts`. The shell is a
  labeled sidebar that collapses to the canonical rail, its preference
  persisted; at phone width the same markup is a bottom bar with a More
  overflow. `nav.ts` holds the canonical nav items in groups. A label is
  presentation and never a route id: `deals` presents as Pipeline, `inbox` as
  Approvals, `ai` as Ask Margince. `theme.ts` resolves and applies light/dark
  before React mounts, so an unauthenticated screen can be dark at all. Also the hash router, the ⌘K palette, and the
  agent section at the foot of the rail (`agentrail.tsx`, the one AI affordance
  in the chrome). See
  [docs/explanation/frontend-architecture.md](../docs/explanation/frontend-architecture.md).
- `src/screens/`: one file per surface, or one directory when a surface is a
  state machine (`onboarding-conversation/`); unbuilt routes render a pending
  state.
- `src/i18n/`: `en.ts`, `de.ts` and `vi.ts`. Key parity is enforced at compile
  time and at runtime.
- `src/format/`: the presentation edge: money/date/duration formatting,
  IANA-only zones, FX lineage display (consumes the IR base_value
  verbatim, never multiplies).
- `src/api/`: `schema.d.ts` is generated (never hand-edit), and `client.ts`
  is the seam every typed `/v1` call goes through. The LinkedIn CSV upload is
  the one `/v1` route that bypasses it, because the generated client cannot
  serialize multipart. (The OAuth discovery read in `connected-agents.tsx` is a
  raw `fetch` too, but it is not a `/v1` route.) Also here: the session cookie
  and the `/v1` mount, with no tenant header. One installation serves one
  company and the server binds that singleton itself, so two tests assert the
  absence of any workspace header.
- `e2e/`: the Playwright harness. It holds AC-named acceptance tests, the 390px
  no-horizontal-scroll sweep, axe WCAG 2.2 AA on every core screen, and the
  held-read claim for a record open. The <300 ms perceived budget is measured by
  `make bench-mobile`, which samples a p95 instead of gating one wall-clock
  reading on a shared runner. Runs over a network-edge seed mock by default;
  `BASE_URL=…` points the same suite at a live backend.

## The gates (all run by `pnpm check` / `pnpm e2e`)

1. Token canon: every Ledger Green token value pinned to the design canon.
2. Three type families only: Outfit (display), Geist (everything read,
   figures included, aligned with `tabular-nums`), Geist Mono (code only:
   `pre`, `code`, `samp`, `.code-block`).
3. Literal colours live only in `tokens.css`.
4. No hard-coded user-facing copy: JSX text and user-facing attributes
   must come from the i18n catalogs (TS AST walk).
5. No emoji glyphs in source strings: Lucide only; the 🟢/🟡 autonomy
   semantics render through the `.dot` token component.
6. One service worker ships, and only `src/app/pwa.ts` registers it. It
   answers nothing from Cache Storage but the offline page, for a navigation
   the network could not make, and that page's own script
   ([pwa.md](../docs/explanation/pwa.md)).
7. WCAG 2.2 AA (axe) in the e2e lane. The perceived-perf budget is not
   here: `make bench-mobile` samples it, because one wall-clock reading on a
   shared runner measures the runner.
8. The unauthenticated surface at 390px / 320px / 200% zoom:
   - no horizontal scroll
   - the primary action inside the viewport with a 44px target (rounded)
   - the identity region whole at every width (every row present, visible and
     taller than zero), and above the task on screen while the task stays first
     in the DOM
   - one h1, which is the greeting
   - the Core out of the a11y tree
   - axe

   The rest of the width sweep walks authenticated routes only.

## Working agreements

- Copy reaches components via `t()`/props; atoms never hard-code words.
- Anything that renders money/time goes through `src/format/`. Locale
  changes rendering only, never a stored value; no FX math, no fixed
  offsets, no calendar diffs.
- Staged / real / human-typed are three distinguishable styles, always.
  Confidence is never hidden. Absent data is omitted, never guessed.
- Packaging: the app is a standalone static `dist/` build (`pnpm build`),
  served separately from the API binary, which embeds no SPA (and serves more
  than `/v1`: probes, `/setup/*`, the public edge, webhooks, and `/mcp` with its
  OAuth routes when enabled). How `dist/` is hosted (a static server, a CDN,
  or a reverse proxy in front of the API) is a deployment choice, not baked
  into the build.

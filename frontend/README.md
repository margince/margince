<!-- prose:plain -->
# frontend/: the Margince web app

React 19 + Vite + TypeScript (strict) + Tailwind 4 + Biome + Vitest + Playwright. Margince has its
own design system. The catalog is [src/design-system/README.md](src/design-system/README.md), and the
visual direction is in [DESIGN.md](../DESIGN.md).

## Commands

```sh
pnpm install
pnpm dev          # Vite dev server; proxies /v1 to http://localhost:8080 (BACKEND_PORT overrides)
pnpm check        # the frontend gate: Biome + unit tests + tsc + build
pnpm e2e          # build + the Playwright screen-acceptance harness
pnpm gen:api      # regenerate src/api/schema.d.ts from ../backend/api/crm.yaml
```

From the root of the repository: `make frontend-check`, `make frontend-e2e`, and `make dev` (the
full running stack: the API and this SPA). `make check` runs this lane as well as the Go one. It
needs Node 24, the version CI pins, and pnpm.

### Switches for previews of the app (`VITE_UI_PREVIEW_*`)

Scaffolding for design review, off unless the variable is set. They can draw a flow but never make it work,
which sets them apart from feature flags. They are in `src/app/ui-preview.ts`; the prefix tells a reader
they are previews.

| Var | What it draws |
|---|---|
| `VITE_UI_PREVIEW_OIDC=1` | The federated sign-in buttons on the login screen, with the second provider marked *not yet available*. |
| `VITE_UI_PREVIEW_RESET=1` | The "Forgot password?" link and the request card it opens. |

```sh
pnpm dev:preview                    # every switch on — the demo entry point
pnpm build:preview                  # the same, built
VITE_UI_PREVIEW_OIDC=1 pnpm dev     # login screen, with the SSO block drawn
```

Never ship a preview build: it draws controls the installation cannot honour. `dev` and `build`
never set these switches.

`VITE_UI_PREVIEW_OIDC=1` puts two providers in `AuthScreen` when the server offers none, at the
render boundary after the query. Reviewers can then see the sign-in buttons without setting up a
provider. The wire is not touched, and the query cache keeps the real answer of the server.

`startFederatedSignIn` does nothing only for a button the preview made up. So an installation that
serves real providers keeps working buttons under the switch. The preview labels are not translated,
because provider labels are copy the server owns. A preview build logs a one-time `console.warn`
that says so.

The same switch marks the second preview provider *not yet available*.
`previewedUnavailableProviders()` returns a set of keys that `ProviderButtons` renders as a native
`disabled` button with an `.is-unavailable` class. The label is not touched, because the marker must
not add our words to a label the installation wrote. So a screen reader hears that the control is not
available, but not why. Only the preview can make a marked provider. `oidc_providers[]` items are
`{ key, label }` with no field for availability, so in the product `ProviderButtons` always receives
an empty set.

The product never shows a provider control that does not work (Google, Microsoft, SSO). A marked
button is allowed here for the same reason a Storybook story is.

`VITE_UI_PREVIEW_RESET=1` draws the "Forgot password?" link. The flow is finished on both sides:
`POST /auth/forgot-password` and `POST /auth/reset-password` in the contract, handlers in
`backend/internal/modules/identity/reset.go`, and all four views on this screen. `password_reset` is
computed live as `h.resetMailer != nil`. So it reports `false` on the shipped `config/margince.yaml`
only because that file has no `email:` block. The switch draws the link and wires no mailer.

The request form behind the link is the real one. So to send it to an installation with no mailer
gets a `501 not_implemented` back, and shows it as the failure note of the form. The confirmation,
the deep-link form and the refusal of a spent token are checked in `src/screens/auth.test.tsx`
instead, because no one can reach them here.

When a switch is not set, it reads `undefined` and nothing changes. `src/app/ui-preview.test.ts` and
the `federated sign-in` cases in `src/screens/auth.test.tsx` pin both positions of both switches.
The `e2e` lane builds without any of them, so `offers no identity provider that does not work` still
measures the real default.

## Layout

- **Read [`src/design-system/README.md`](src/design-system/README.md) before you build a control.**
  It is the catalog. Every control a user can click or type into comes from that folder. An HTML
  `<select>` or a dropdown built by hand is a bug, and `src/design-system/native-controls.test.ts`
  refuses one. That test is a vitest gate over the TypeScript AST, and `make native-controls` runs
  it on its own. `pnpm storybook` shows every control.
- `src/design-system/`: tokens, building blocks and their stories. Read
  [its README](src/design-system/README.md) before you build a control.
- `src/app/`: the shell, the top bar, `nav.ts` and `theme.ts`. The shell is a labeled sidebar that
  folds down to the standard rail, and it keeps that choice. At phone width the same markup is a
  bottom bar with a More menu for the rest.

  `nav.ts` holds the standard nav items in groups. A label
  is for display and never a route ID: `deals` shows as Pipeline, `inbox` as Approvals, `ai` as Ask
  Margince. `theme.ts` resolves and applies `light` or `dark` before React mounts, so a screen before
  sign-in can be dark at all. Also here: the hash router and the `⌘K` palette. The agent
  section at the foot of the rail is here too (`agentrail.tsx`): the one AI control in
  the frame of the app. See
  [docs/explanation/frontend-architecture.md](../docs/explanation/frontend-architecture.md).
- `src/screens/`: one file per surface, or one folder when a surface is a state machine
  (`onboarding-conversation/`). Routes that are not built yet render a pending state.
- `src/i18n/`: `en.ts`, `de.ts` and `vi.ts`. Key parity is checked at compile time and at run time.
- `src/format/`: the display edge. Money, date and duration formatting, IANA zones only, and the FX
  lineage display (it uses the IR `base_value` as it is, and never multiplies).
- `src/api/`: `schema.d.ts` is generated (never edit it by hand), and `client.ts` is the seam every
  typed `/v1` call goes through. The LinkedIn CSV upload is the one `/v1` route that skips it,
  because the generated client cannot send a multipart body. (The OAuth discovery read in
  `connected-agents.tsx` is a raw `fetch` too, but it is not a `/v1` route.) Also here: the session
  cookie and the `/v1` mount, with no tenant header. One installation serves one company, and the
  server binds that single company itself. So two tests check that no workspace header is sent.
- `e2e/`: the Playwright harness. It holds acceptance tests named for their AC, and the `390px` sweep that
  checks there is no sideways scroll. It also holds axe WCAG 2.2 AA on every core screen, and the
  held-read claim for opening a record. `make bench-mobile` measures the budget of `<300 ms` as a user feels it. It
  samples a `p95` instead of gating one clock reading on a shared runner. By default the tests run over
  a seed mock at the network edge; `BASE_URL=…` points the same suite at a live backend.

## The gates (all run by `pnpm check` / `pnpm e2e`)

1. Token canon: every Ledger Green token value is pinned to the design canon.
2. Three type families only. Outfit is for display. Geist is for everything a user reads, numbers
   included, aligned with `tabular-nums`. Geist Mono is for code only: `pre`, `code`, `samp`,
   `.code-block`.
3. Literal colours live only in `tokens.css`.
4. No copy typed into code. JSX text and attributes a user sees come from the `i18n` catalogs
   (checked by a walk of the TS AST).
5. No emoji in source strings: Lucide only. The 🟢/🟡 autonomy meanings render through the `.dot`
   token component.
6. One service worker ships, and only `src/app/pwa.ts` registers it. From Cache Storage it answers only
   two things. One is the offline page, for a page load the network could not make. The other is that
   page's own script ([pwa.md](../docs/explanation/pwa.md)).
7. WCAG 2.2 AA (axe) in the `e2e` lane. The budget for how fast it feels is not here.
   `make bench-mobile` samples it, because one clock reading on a shared runner measures the runner.
8. The surface before sign-in at `390px` / `320px` / `200%` zoom:
   - no sideways scroll
   - the main action inside the viewport, with a `44px` target (rounded)
   - the identity region whole at every width: every row there, visible and taller than zero
   - the identity region above the task on screen, while the task stays first in the DOM
   - one `h1`, which is the greeting
   - the Core out of the `a11y` tree
   - axe

   The rest of the width sweep walks routes after sign-in only.

## Working agreements

- Copy reaches components through `t()` and props; the smallest components never type in words.
- All that renders money or time goes through `src/format/`. The locale changes only how a value
  renders, never a stored value. No FX math, no fixed offsets, no math on calendar dates.
- Staged, real and typed by a human are three styles a user can tell apart, always. Confidence is
  never hidden. Absent data is left out, never guessed.
- Packaging: the app is a standalone static `dist/` build (`pnpm build`), served apart from the API
  binary. The API embeds no SPA. It serves more than `/v1`: probes, `/setup/*`, the public edge,
  webhooks, and `/mcp` with its OAuth routes when it is on. You choose how to host `dist/` when you
  deploy: a static server, a CDN, or a reverse proxy in front of the API. The build does not fix it.

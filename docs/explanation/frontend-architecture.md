# Frontend architecture: how the web app is put together

Read this before you change anything under `frontend/`. Most of the app's
conventions are enforced, but many are visible only in source; this page
states them once. The command sheet (what to run, which switches exist) is
[`frontend/README.md`](../../frontend/README.md); this page gives the reasons
behind the structure, the role [architecture.md](architecture.md) plays for the
Go tree.

## What the app is

A standalone static Vite/React build. `pnpm build` produces a `dist/` served
**separately** from the API binary, which serves `/v1` only and embeds no SPA.
How `dist/` is hosted (static server, CDN, reverse proxy) is a deployment
choice; the build does not fix it.

It is a **plain client of the same `/v1` contract** every other client uses.
There is no privileged path, no backdoor and no frontend-only endpoint; the
agent surface follows the same rule. Three consequences follow:

- **One API seam for the `/v1` contract.** `src/api/client.ts` is where a typed
  JSON request is constructed: same-origin `location.origin + "/v1"`,
  `credentials: "include"` for the session cookie, and the global `fetch`
  resolved per call so test stubs can intercept. Two raw `fetch` calls exist,
  and neither weakens the rule. The LinkedIn `Connections.csv` upload
  (`screens/linkedin-import.tsx`) is a **`/v1` call the typed client cannot
  express**, because the client cannot serialize a multipart body. The file
  says so in place, and a new exception of that kind needs the same note. The
  other is `screens/connected-agents.tsx` reading
  `/.well-known/oauth-protected-resource`, which is **not a `/v1` route**, so
  the typed client does not carry it.
- **No tenant selector on the wire.** One installation serves one company, and
  the server resolves it itself. The client sends the session cookie and
  nothing else. `auth.test.tsx` and `preferences.test.tsx` assert the absence
  of a workspace header, so re-introducing one fails the build.
- **Generated types, gated.** `src/api/schema.d.ts` and
  `src/api/public-events.ts` are generated from `backend/api/crm.yaml` and
  `backend/api/public-events.yaml` (`pnpm gen:api`). Never hand-edit them.
  `make frontend-check` regenerates and diffs, so a contract change that
  skipped regeneration fails the build instead of leaving stale frontend types.

Routing is **hash-based** (`#/deals/01J9ZK` → `{ screen: "deals", id: "01J9ZK" }`)
so any static host serves `index.html` for every entry point with no
server-side SPA fallback. A hash may carry a query of its own; the parser strips
it, so a `?utm=…` never leaks into a screen name.

## The layers

```text
 src/design-system/     tokens.css → brand.css → base.css
                        → atoms → trust → the Core primitive → composed
 src/app/               shell = sidebar + top bar + page title, hash router,
                        ⌘K palette, agent dock, theme, capability, banners
 src/screens/           one file per surface (a directory when the surface
                        is a state machine)
 src/i18n/  src/format/ the presentation edge: copy, money, dates, zones
 src/api/               the one seam + the generated contract types
```

- **`src/design-system/`**: the shared vocabulary, in dependency order.
  `tokens.css` is the canonical Ledger-Green palette, mirrored verbatim from
  [DESIGN.md](../../DESIGN.md) and pinned value by value by `tokens.test.ts`.
  `brand.css` is the derived layer: every value there is a `color-mix()` of a
  canonical token, never a new hex. Then come `atoms.tsx` (Button, Badge,
  Avatar, Card, Modal, …), `trust.tsx` (the trust vocabulary: `AutonomyDot`,
  `EvidenceChip`, `ConfidenceMeter`, `ProvenanceTag`, `StagingCard`,
  `FieldDiff`), the Margince Core (`margince-core*`), and `composed.tsx`,
  which builds on both (`RecordView`, `PipelineBoard`, `GroupedTimelineList`,
  …). `motion.ts` holds the reduced-motion rule: reduced motion jumps to the
  end state, never to nothing. `conformance.test.ts` is the drift gate over the
  whole tree.
- **`src/app/`**: the application chrome and what is true on every screen.
  `shell.tsx` is the frame, the sidebar and the page's own heading.
  `topbar.tsx` is the session strip: collapse control, breadcrumb, search,
  account. `pagemeta.ts` holds what both of them know about a page before it
  renders. Then `nav.ts` (the canonical destination list), `router.tsx` (hash
  routing), `palette.tsx` (⌘K), `agentrail.tsx` (the agent, at the foot of the
  rail), `theme.ts`, `capability.ts`, and the shell-level advisories
  (`economybanner.tsx`, `embedreindexbanner.tsx`).
- **`src/screens/`**: one file per surface. A surface gets a *directory*
  only when it is a state machine instead of a page. One has:
  `screens/onboarding-conversation/`, where the conversation machine, its acts,
  its scenes and its restore logic each need their own file. Everything else,
  including surfaces as large as `companies.tsx` and `deals.tsx`, stays a file
  with co-located `*.test.tsx` and `*.stories.tsx`. An address no screen
  answers renders a notice saying so (`shell.unknownPage`), never a blank page.
- **`src/i18n/`**: three catalogs (`en`, `de`, `vi`), `en` the default. Key
  parity is enforced twice. `MessageKey` is `keyof typeof en` and the other
  catalogs are `satisfies`-checked against it, so a missing key fails `tsc`.
  `i18n.test.ts` re-checks at runtime and proves `LOCALES` matches the
  registered catalogs, so a build that skipped typechecking still fails.
  A new string therefore lands in all three catalogs in one change: one catalog
  alone is a red build, never a missing translation on a reader's screen. A
  count takes its wording from the reader's own `Intl.PluralRules` category
  (`format/plural.ts`), and `one-plural-rule.test.ts` refuses a `count === 1`
  choosing a message key anywhere outside this directory.
  Resolution order is the explicit choice → the browser's languages → `en`
  (unconfigured English is `en-GB`, never `en-US`). Locale is presentation
  only: it never participates in storage or math.
- **`src/format/`**: the presentation edge. Money arrives as integer minor
  units + ISO currency and is only *scaled* for display; zones are IANA names,
  and a fixed offset is rejected at the edge with an error. No FX math, no
  calendar arithmetic, no locale flowing back into storage.

## The shell

`src/app/nav.ts` holds the canonical navigation and `rail.test.tsx` pins its
order. It is eight items: Home standing alone, then three labelled groups.

| Group | Route ids |
|---|---|
| *(ungrouped)* | `home` |
| Records | `contacts`, `companies`, `leads`, `deals` |
| Work | `projects`, `filters` |
| Intelligence | `analytics` |

The groups are the **expanded sidebar's** own structure. Collapsed, each group
heading keeps its box and draws a hairline in the same space, so the 56px rail
is the flat list [DESIGN.md](../../DESIGN.md#6-the-shell) describes; the
expanded state adds to it. Collapse is a persisted preference
(`margince.sidebarCollapsed` in `localStorage`, read once at mount). The column
animates between 256px and 56px on `--dur-move`.

At **≤700px** the same `<nav>` element becomes a fixed bottom bar of five equal
cells. `MOBILE_PRIMARY` (`home`, `contacts`, `deals`) rides the bar; everything
else lives behind **More**, which expands the same element into a sheet. One
nav element means one navigation landmark and no second item list to keep in
sync. The hidden routes' own rows are `display:none` at this width, so
**More** carries the claim the hidden row would: `aria-current="page"` on the
destination itself. On a page below a destination it hides (a company, lead or
project record, a focused Filters and views page, one list) the top bar's trail
claims the page, so **More** says `"true"`. A contact or deal record leaves
**More** without the attribute, because that destination's own bar cell is
visible and carries the claim. **More** drops the attribute once the sheet is
open, so two elements never both claim the current page.

The **middle** cell is the agent, which is not a destination: it reports
instead of navigating, and it belongs to the whole session, not to one screen.
`NavLevelView` takes it as a `centre` slot and renders it into the row stream,
after the bar row that leaves as many cells to its left as to its right. The
bar's tab order is then the order a thumb reads it in; a cell placed by a grid
column alone would be third on screen and last to the keyboard. It rises clear
of the bar's top edge by `--phoneAgentRise`, which `--phoneNavClearance` adds
to the bar's own height so a sticky element never lands behind it. Above the
breakpoint the same block is the sidebar's foot (`.railagent`). It moves
instead of being drawn twice, because two of them would be two Cores reporting
one session.

`RAIL_LESS_SCREENS` is the documented layout exception: `onboarding`, `book`,
`client`, `preferences`, `unsubscribe`, `confirm`, `room`, `oauth-consent`.
These render full-bleed with their own chrome. A human lending an agent their
authority reads that screen apart from the app, outside its frame. The
pre-session surfaces (login, availability, the splash) use the same rail-less
frame.

### A nav label is presentation and never a route id

This is the convention the next developer adding a destination is most likely to
get wrong, so it is stated in `nav.ts`, again in `palette.tsx`, and here.
`NavItem.screen` is the **route id**: the stable English name in the hash, in
`App.tsx`'s switch, and in every `href`. `NavItem.labelKey` is a **catalog key**
whose rendered text is free to differ, and one of the eight does:

| Route id | Rendered label |
|---|---|
| `filters` | Filters and views |

The command palette relies on the split. Every screen command carries its route
id, plus any `aliases` the nav item declares, as hidden `keywords`. Someone
typing "filters", "today" or "pipeline" still finds the destination, in any
locale, without a hand-kept synonym list. **Never rename a `screen` to match a
label**: that breaks every existing hash URL and every `SCREEN_ENTITY` /
`OFF_RAIL_TITLE_KEYS` lookup keyed on it.

Off-rail destinations (reached from Settings, from a record or from another
screen, never from the rail) resolve their page title through
`OFF_RAIL_TITLE_KEYS` in `app/pagemeta.ts`: `worklist`, `settings`, `offers`,
`partners`, `share`, `search`, `scheduled`, `tags`, `lists`. A raw screen slug
is never shown as a title.

### The badge policy

`BADGE_SCREENS` is empty: no rail destination shows a count. A badge would
count only what wants a human's attention, never an ambient total. The list
endpoints are keyset-paginated and return no total, so a decorative count has
nothing true to show. Approvals and tasks are lanes inside Home, which reports
its numbers on the page. The set stays because
`badgeIds` is how a nav level declares badgeable rows, and deeper levels use the
same mechanism.

## Colour

The app chrome is **glass over the lit page ground**, as
[DESIGN.md](../../DESIGN.md#6-the-shell) and the top of `src/app/shell.css`
state. It does not use the deep ink-green field, so rail icons read as ordinary
theme tokens instead of white-alpha on a dark ground.

The dark-rail family in `tokens.css` (`--bgRail`, `--railTop`,
`--railBottom`, `--railIcon`, `--railIconHover`, `--railIconActive`,
`--railHover`, `--railActive`, `--overlayScrim`) is for the ink-green field
only. Its comment says it is unthemed, white-alpha in both themes, which is
correct for that field: the collapsed rail's tooltips, the client-surface bar,
and the website and deck surfaces. A new app panel styled from those tokens is
styled as the marketing field by mistake.

Both appearances ship, and **the theme resolves before React mounts**.
`main.tsx` calls `startTheme()` above `createRoot(...).render(...)`. The
resolution lives in `src/app/theme.ts` and nowhere else: an explicit choice
(`margince.theme` in `localStorage`, `light` or `dark`) wins, otherwise the OS
`prefers-color-scheme`.

A third value, `system`, is a standing instruction to keep following the OS.
An install that has never chosen resolves as `system`, and so does any value
the build cannot name. While it is the choice, `theme.ts` holds a
`prefers-color-scheme` subscription and repaints an open tab when the OS
setting changes; an explicit choice drops it. `startTheme()` arms that at boot
instead of in the first mounted control, because the account menu, which owns
the three-way chooser, renders its control only while it is open.

Resolving the theme at boot means unauthenticated pages follow the stored
theme, signing out does not leave a stale theme on screen, and reload does not
flash light before dark.

**Literal colours live only in `tokens.css`.** Everything else (`brand.css`,
every component sheet, every `.tsx`) reads `var(--token)`, and a derived value
is a `color-mix()` of a canonical token instead of a new hex. So the dark
theme's accent lift carries through with nothing to re-declare per theme. The
rule is enforced twice (see below), and the exemptions are **named files, never
patterns**:

| Exempt | Why |
|---|---|
| `design-system/tokens.css` | the literals are its job; `tokens.test.ts` pins each one |
| `index.html` | `<meta name="theme-color">` cannot read a CSS custom property |
| `design-system/provider-mark.tsx` | it carries Google's and Microsoft's own sign-in marks. Another company's colours are not ours to tokenise, and a provider mark in Ledger Green is a *wrong* mark |

`--overlayLight` / `--overlayDark` (pure white and pure black, unthemed) live in
`tokens.css` for the same mechanical reason: they are material effects, not
brand colour, and a literal anywhere else fails the gate.

## Provenance

**`EvidenceMark` is the provenance affordance.** A value that came from
somewhere other than a user typing it carries a dotted underline. Opening the
mark says where it came from, how sure the system was, the text it was read
from and when, and offers a way through to that field's full history.

It replaced a **stack of three chips** under every value (`ProvenanceTag` +
`ConfidenceMeter` + `EvidenceChip`, three widgets per field). Three chips under
a value read as clutter, and the value they describe gets lost among them. The
mark keeps the record readable and puts the evidence one interaction away.

The older primitives remain in two places, both by design:

- **Inside the mark.** `EvidenceMark`'s panel renders `ProvenanceTag` itself,
  and states confidence as a word instead of a meter.
- **On the staging surfaces**, whose job is to compare a proposal against what
  is held. These are the approvals lane of the worklist (`screens/worklist.tsx`,
  `screens/approvalrow.tsx`), the onboarding confirm card
  (`screens/onboarding-conversation/confirm-card.tsx`,
  `screens/onboarding-company-form.tsx`), the Company-context settings screen,
  and the record surfaces that show a single provenance line
  (`contacts.tsx`, `leads.tsx`, `consent.tsx`, `history.tsx`).
  `StagingCard`/`FieldDiff` are the composed forms of the same vocabulary.

**One open at a time, for pointer *and* keyboard.** A module-level
`closeOpenMark` holds the single open panel; opening one closes the last.
Pointer dismissal would give a mouse user that behaviour anyway. The explicit
registry gives it to a keyboard user tabbing down a column of marked values,
who leaves a trail of one panel instead of a stack of overlapping regions.
Escape closes the panel and returns focus to the trigger. The panel is a named
`<section>`, not a dialog: it is a disclosure beside the value, the page behind
it stays usable, and nothing traps focus. A mark with **no source renders as
plain text**, because an underline that opens an empty popover teaches the
reader to stop opening them.

## The core primitive

`MarginceCoreScene` (`design-system/margince-core.tsx`) is the product's one
piece of AI identity, shown by the unauthenticated surface, the session splash,
onboarding and the agent rail. Four rules apply to it:

- **One implementation.** A caller passes `state` and never restyles. Sizing
  through the documented `--coreSize` / `--coreGlass` custom properties is
  configuration; anything beyond that is a caller restyling a shared primitive.
- **The state list is closed**: `idle`, `ingest`, `working`, `warning`,
  `error`. Callers use the Core as a *status channel* (a sign-in in flight, a
  server that cannot be reached). A status channel with an open vocabulary
  cannot be tested, and a second caller cannot reuse it. Red means not
  connected and nothing else; amber is the fault that can wait. `progress` is
  optional and draws the ring only when passed.
- **Rendering is a fallback ladder.** The WebGL2 shader is preferred
  (`margince-core-shader.ts`), and a host without it gets a static CSS dress
  carrying the same `data-core-state`. Nothing reading the Core's state off the
  DOM can tell the two apart.
- **It is `aria-hidden`.** The surface around it also states every state it
  shows in text, which is what makes it safe to be this decorative.

The agent section's orb **is** the Core (`MarginceCoreScene`). There is one orb
in the product; a CSS approximation in permanent chrome would be a second one
for a reader to tell apart from the real thing. For the same reason it is the
only one on screen at a time, so the panel it opens carries none.
`agentrail.css` says the same next to the rule that sizes it.

## The gates

`make check-fe` → `make frontend-check` is the merge lane, and it runs in this
order. Four fail-closed shell greps come first, so they hold the same
discipline even if the test tree regresses.

| Gate | Where it lives | What fails it |
|---|---|---|
| Token purity | `frontend/scripts/check-ds-purity.sh` | a hex/`rgb()`/`rgba()`/`hsl()`/`hsla()`/`oklch()` literal in hand-written shipped `.ts`/`.tsx`/`.css` under `frontend/src` or `extensions/*/frontend`. It skips four names: `tokens.css` (the literals are its job), `provider-mark.tsx` (third-party brand marks), `*.test.*` (fixtures) and the generated `schema.d.ts`. `index.html` is also exempt, for a different reason: the script walks `frontend/src` and `extensions/*/frontend`, and `index.html` sits above both. Fails closed if it scans zero files |
| Font lock | `frontend/scripts/check-font-lock.sh` | a `font-family` outside Outfit (display) / Geist (body, figures included) / Geist Mono (code only) + the named generic fallbacks; and mono anywhere but `code`, `pre`, `samp` and `.code-block`, which `design-system/mono.test.ts` refuses too |
| Icon glyphs | `frontend/scripts/check-icon-glyph.sh` | an emoji in rendered code. Comments are stripped, since the 🟢/🟡 tier notation is house style and renders through `AutonomyDot` |
| Spacing | `frontend/scripts/check-ds-spacing.sh` | a **newly added** inline `margin`/`padding`/`gap` px literal; diff-scoped vs `origin/main`, waived in-line with `// ds:ignore <reason>` |
| Spacing roles | `frontend/scripts/check-ds-spacing-roles.sh` | a screen rule that re-spaces a design-system primitive the design system spaces, or re-sizes one it sizes: `font-size`, `line-height`, `letter-spacing` (both corpora are derived from `design-system/*.css` on every run). Also a rule that spells a rung where a role exists: `*-actions` gap, `*-cards` gap, `*-card`/`*-panel` padding. **Whole-tree**: promoting a class into the design system turns untouched screen rules into findings that no diff contains. A variant with no role to name it is waived in-line with `/* ds:ignore <reason> */`. `check-ds-spacing-roles.test.sh` tests its verdict against fixture trees |
| Type source | `design-system/type-source.test.ts` | type declared by value anywhere but `tokens.css`. In every `.css` under `frontend/src` and every non-test style object, `font-size`, `line-height`, `letter-spacing` and `text-transform` may say only `inherit`. `font` takes a `--font*` token and `font-weight` a `--fontWeight*` one. Two layout facts are excepted, only inside the two UA resets. It refuses capitals by every route: `font-variant-caps`, the `font-variant` shorthand, the small-caps features, and a `.toUpperCase()` whose result is rendered. Fails closed: a corpus missing `app.css`, under 100 stylesheets or under 100 components fails instead of passing empty |
| Action rows | `design-system/actionrow.test.ts` | a container whose element children are two or more buttons and nothing else, that does not get `gap: var(--gapActions)` from a class it names or from its own inline style. That includes a class **no stylesheet defines**, which a CSS-only gate cannot see. Waived in line with `{/* ds:ignore <reason> */}` |
| Contract type drift | `make frontend-check` | `pnpm gen:api` produces a diff in `src/api/schema.d.ts` / `public-events.ts` |
| Lint | `pnpm lint` (Biome) | formatting and lint findings over `src` + `index.html` |
| Conformance suite | `design-system/conformance.test.ts` | the AST-accurate arm of the same rules, plus hard-coded user-facing copy outside the i18n catalogs and an invalid web-app manifest |
| Service worker | `frontend/scripts/vite-pwa.test.ts`, `frontend/src/app/serviceworker-registrar.test.ts` | the SPA build not emitting `/sw.js`. The emitted worker answering from Cache Storage anything but a failed navigation and the offline page's own script. The worker intercepting a path the api owns, keeping a cache that is not its own, or keeping its cache name when its contents changed. Any shipped module but `src/app/pwa.ts`, in any script dialect, reaching for `navigator.serviceWorker` ([pwa.md](pwa.md)) |
| Stylesheet namespaces | `design-system/stylesheetnamespace.test.ts` | a screen's class namespace declared in a stylesheet other than its home sheet, across every `.css` under `frontend/src` and each extension's frontend layer |
| Timeline rows | `design-system/timelinerows.test.ts` | a rule that reaches a `.timeline` list's `li` by anything but `>` from the `.timeline` compound, which also styles every list nested in a row; a sibling (`+`, `~`) of such a row passes. Across every `.css` under `frontend/src` and each extension's frontend layer. Fails closed when its parsed row selectors fall short of a plain-text count |
| Token canon | `design-system/tokens.test.ts` | a Ledger-Green value drifting from the design canon |
| Typecheck + build | `pnpm build` (`tsc -b && vite build`) | any type error |
| Unit tests | `pnpm test` (Vitest) | co-located `*.test.tsx` |
| Render UAT | `make fe-uat` → `frontend/scripts/fe-uat.mjs` | a changed component with no co-located story, a changed story the build does not register, or an unclean headless render. **Not** in `make check`: it is the frontend-only UAT lane, with its artifact at `.tmp/fe-uat/manifest.json` |
| Screen acceptance | `make frontend-e2e` → `frontend/e2e/` | AC-named Playwright cases, axe WCAG 2.2 AA, the 390px no-horizontal-scroll sweep. The perceived-perf budget is `make bench-mobile`'s: a sampled p95, because one wall-clock reading in a shared lane measures the runner |

The backend's `craft static` pre-push hook does **not** cover `frontend/`. The
frontend lane is separate from the Go merge gate and needs node + pnpm. Run
`make check-fe` (or `make frontend-check`) before pushing a frontend change.

## Where to look first

| If you are changing… | Start at |
|---|---|
| a destination, a nav label, a badge | `src/app/nav.ts`, then `src/app/shell.tsx` and `shell.test.tsx` |
| what a route renders | `src/App.tsx` (`ScreenView`), then the screen file |
| a colour, a radius, a spacing rung | `src/design-system/tokens.css` (and `tokens.test.ts`), never a call site |
| a derived colour role | `src/design-system/brand.css`: a `color-mix()`, never a new hex |
| light/dark behaviour | `src/app/theme.ts` + `tokens.css`, which carries all three states: the light palette on bare `:root`, the `prefers-color-scheme` arm for a surface whose host states nothing, and the `[data-theme]` arms an explicit choice stamps |
| how a derived value shows its evidence | `src/design-system/evidencemark.tsx` |
| a staging/approval surface | `src/design-system/trust.tsx` + `src/screens/worklist.tsx` |
| copy | `src/i18n/en.ts`, `de.ts` **and** `vi.ts`; key parity is compile-time |
| money, dates, durations, zones | `src/format/format.ts`, except which calendar day an instant falls on and the instant a picked day ends, which are `src/format/calendarday.ts` |
| an API call | `src/api/client.ts` is the seam; regenerate types with `pnpm gen:api` |
| the Core's appearance or states | `src/design-system/margince-core.tsx` + `margince-core-shader.ts` + `margince-core-motion.ts` |

## Where the code lives

| | |
|---|---|
| The API seam + generated contract types | `frontend/src/api/{client.ts,schema.d.ts,public-events.ts}` |
| Boot: theme, install offer, service worker, query client, 403 handling | `frontend/src/main.tsx` |
| Service worker registration, install state / the worker and its offline page | `frontend/src/app/pwa.ts` / `frontend/src/offline/`, built by `frontend/scripts/vite-pwa.ts` |
| Route → screen, the auth gate, the onboarding gate | `frontend/src/App.tsx` |
| Shell frame, sidebar, page heading | `frontend/src/app/{shell.tsx,shell.css}` |
| Top bar: breadcrumb, search, account | `frontend/src/app/{topbar.tsx,topbar.css,account.tsx}` |
| The canonical nav, badges, mobile set, rail-less set | `frontend/src/app/nav.ts` |
| Hash router | `frontend/src/app/router.tsx` |
| ⌘K palette, the agent in the rail | `frontend/src/app/{palette.tsx,agentrail.tsx}` |
| Theme resolution and persistence | `frontend/src/app/theme.ts` |
| Tokens (canonical) / derived roles / base controls | `frontend/src/design-system/{tokens.css,brand.css,base.css}` |
| Atoms, trust vocabulary, composed surfaces | `frontend/src/design-system/{atoms,trust,composed}.tsx` |
| The provenance mark | `frontend/src/design-system/evidencemark.tsx` |
| The Core primitive + its renderers | `frontend/src/design-system/margince-core.tsx`, `margince-core-{engine,gl,shader,motion}.ts` |
| The AI runtime chip | `frontend/src/design-system/airuntimechip.tsx` |
| Design gates (tests) | `frontend/src/design-system/{conformance,tokens}.test.ts` |
| Design gates (fail-closed greps) | `frontend/scripts/check-*.sh` |
| Change-scoped render UAT | `frontend/scripts/fe-uat.mjs` |
| Screens | `frontend/src/screens/` (one file per surface; `onboarding-conversation/` is the one state-machine directory) |
| Catalogs / presentation edge | `frontend/src/i18n/`, `frontend/src/format/` |

## Where to go next

- [company-record-page.md](company-record-page.md): the biggest screen this
  structure carries.
- [pwa.md](pwa.md): installing the app, the service worker and the offline page.
- [company-context.md](company-context.md): the onboarding wizard and the
  company-profile screens.
- [architecture.md](architecture.md): the Go side of the same contract.
- [../reference/make-targets.md](../reference/make-targets.md): every target
  named above.
- [../../frontend/README.md](../../frontend/README.md): commands, the
  UI-preview switches, working agreements.

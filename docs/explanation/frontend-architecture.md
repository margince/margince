<!-- prose:plain -->
# The frontend: how the web app is put together

Read this before you change anything under `frontend/`. Most of the rules of
the app are enforced, but many show only in the source, so they are written
down here once. The list of commands (what to run, which switches exist) is
[`frontend/README.md`](../../frontend/README.md). Here are the reasons behind
the shape, as [architecture.md](architecture.md) gives them for the Go tree.

## What the app is

A Vite and React build of plain files that stands alone. `pnpm build` writes a
`dist/` folder that is served **apart** from the API binary. The binary serves
`/v1` only and holds no copy of the web app. How `dist/` is hosted is a
choice made at deploy time: a file server, a CDN, or another server that passes
requests on. The build does not fix it.

It is a **plain client of the same `/v1` contract** that every other client
uses. There is no path with more rights, no secret way in and no endpoint only
for the frontend; the agent surface follows the same rule. Three things follow:

- **One API seam for the `/v1` contract.** `src/api/client.ts` is where a typed
  JSON request is built. It goes to the same host (`location.origin + "/v1"`),
  with `credentials: "include"` for the session cookie. It looks up `fetch`
  again on each call, so test stubs can stand in for it. Two raw `fetch` calls
  exist, and neither breaks the rule; they come after this list.
- **The wire names no tenant.** One installation serves one company, and the
  server resolves it itself. The client sends the session cookie and nothing
  else. `auth.test.tsx` and `preferences.test.tsx` check that no workspace
  header is sent, so adding one back fails the build.
- **Generated types, gated.** `src/api/schema.d.ts` and
  `src/api/public-events.ts` are generated from `backend/api/crm.yaml` and
  `backend/api/public-events.yaml` (`pnpm gen:api`). Never edit them by hand.
  `make frontend-check` generates them again and fails on any change. So a
  contract change that skipped that step fails the build, and old types never
  stay in the frontend.

The first raw call is the LinkedIn `Connections.csv` file send. It is a
**`/v1` call the typed client cannot make**: the client cannot build a
`multipart` body. Its file (`screens/linkedin-import.tsx`) says so in place, and a new exception of that kind
needs the same note. The other raw call is `screens/connected-agents.tsx`
reading `/.well-known/oauth-protected-resource`. That is **not a `/v1` route**,
so the typed client does not carry it.

Routing uses the **URL hash** (`#/deals/01J9ZK` → `{ screen: "deals", id: "01J9ZK" }`).
So any plain file host serves `index.html` for every entry point, and the
server needs no fallback for the web app. A hash may carry a query of its own.
The router removes it, so a `?utm=…` never ends up in a screen name.

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

- **`src/design-system/`**: the shared parts, in the order they depend on each
  other. `tokens.css` is the Ledger-Green colour set that decides. It is copied
  as is from [DESIGN.md](../../DESIGN.md) and pinned value by value by
  `tokens.test.ts`. `brand.css` is the derived layer: every value there is a
  `color-mix()` of a source token, never a new hex value.

  Then come `atoms.tsx` (`Button`, `Badge`, `Avatar`, `Card`, `Modal`, …) and
  `trust.tsx`. The trust parts are `AutonomyDot`, `EvidenceChip`,
  `ConfidenceMeter`, `ProvenanceTag`, `StagingCard` and `FieldDiff`. Next come
  the Margince Core (`margince-core*`) and `composed.tsx`, which builds on both
  (`RecordView`, `PipelineBoard`, `GroupedTimelineList`, …). `motion.ts` holds
  the `prefers-reduced-motion` rule: with it on, a change shows its end state at
  once, never nothing. `conformance.test.ts` is the drift gate over the whole
  tree.
- **`src/app/`**: the shell of the app and what holds on every screen.
  `shell.tsx` is the shell, the sidebar and the title of each page.
  `topbar.tsx` is the top bar of the session: the control that makes the
  sidebar small, the breadcrumb, search, and the account. `pagemeta.ts` holds
  what both of them know about a page before it renders.

  Then come `nav.ts`
  (the list of places that decides), `router.tsx` (hash routing) and
  `palette.tsx` (`⌘K`). After them are `agentrail.tsx` (the agent, at the end
  of the rail), `theme.ts` and `capability.ts`. Last are the advisories the
  shell itself shows (`economybanner.tsx`, `embedreindexbanner.tsx`).
- **`src/screens/`**: one file per surface. A surface gets a *folder* only
  when it is a state machine and not a page. One has one:
  `screens/onboarding-conversation/`. There the conversation machine, each
  `*-act.tsx` and `*-scene.tsx` file, and the code that loads the conversation
  again each need their own file. Everything else, even surfaces as
  large as `companies.tsx` and `deals.tsx`, stays one file, with its
  `*.test.tsx` and `*.stories.tsx` beside it. An address that no screen answers
  shows a note that says so (`shell.unknownPage`), never an empty page.
- **`src/i18n/`**: three catalogs (`en`, `de`, `vi`), with `en` the default.
  Two checks make every catalog hold the same keys. `MessageKey` is
  `keyof typeof en`, and the other catalogs are checked against it with
  `satisfies`, so a missing key fails `tsc`. `i18n.test.ts` checks again at
  runtime and proves `LOCALES` matches the registered catalogs. So a build that
  skipped the type check still fails.

  So a new string lands in all three catalogs in one change. A string in one
  catalog alone is a red build, never missing text on the screen of a reader.
  A count takes its text from the `Intl.PluralRules` kind of the language the
  reader uses (`format/plural.ts`). `one-plural-rule.test.ts` refuses a
  `count === 1` that chooses a message key in any file outside this folder.

  The order of choice is: the user's own choice, then the languages the browser
  asks for, then the default. English with no setting is `en-GB`, never
  `en-US`. The locale only changes how things show: it never changes what is
  stored, and it never changes a calculation.
- **`src/format/`**: the edge where values turn into text. Money comes in as a
  whole number of `minor` units plus its ISO 4217 code. It is only moved to the
  right size to show it. Each time zone is an IANA name, and a
  fixed value such as `+02:00` is rejected at the edge with an error. No turning
  one kind of money into another, no counting on calendar dates, and no locale
  going back into what is stored.

## The shell

`src/app/nav.ts` holds the list of places that decides, and `rail.test.tsx`
pins its order. It has 8 entries: Home on its own, then three groups with
labels.

| Group | Route ids |
|---|---|
| *(no group)* | `home` |
| Records | `contacts`, `companies`, `leads`, `deals` |
| Work | `projects`, `filters` |
| Intelligence | `analytics` |

The groups belong to the **open sidebar** alone. When the sidebar is small,
each group title keeps its place and shows a line there. So the
`56px` rail is the plain list that [DESIGN.md](../../DESIGN.md#6-the-shell)
shows; the open state adds to it. Whether the sidebar is small is a stored
setting (`margince.sidebarCollapsed` in `localStorage`, read once when it first
renders). The column moves between `256px` and `56px` over `--dur-move`.

At `≤700px` the same `<nav>` element becomes a bar of 5 places of the same
size, fixed below the page. `MOBILE_PRIMARY` (`home`, `contacts`, `deals`) is on
the bar; everything else lives behind **More**, which opens the same element
over the screen. One `<nav>` element means screen readers find one `navigation`
role, and there is no second list to keep the same. At this size the rows of
the routes not on the bar are `display:none`, so **More** carries
`aria-current="page"` for them.

That value sits on the place a hidden row stands for. Some pages sit below such
a place: a company, lead or project record, a Filters and views page, or one
list. There the top bar's trail claims the page, so **More** says `"true"`. A contact
or deal record leaves **More** without the value, because the bar shows that
place and claims the page. **More** drops the value once it is open, so two
elements never both claim the current page.

The third of the 5 places is the agent, which is not a place to go to. It
reports and does not move you, and it belongs to the whole session, not to one
screen. `NavLevelView` takes it as its `centre` and renders it into the
stream of rows. It comes after the row that leaves as many places before it as
after it. So the tab order of the bar is the order the user sees it in. A place put
there by CSS alone would be third on screen and last in tab order.

The agent stands above the top edge of the bar by `--phoneAgentRise`.
`--phoneNavClearance` adds that to the size of the bar, so an element that
stays in place as the page moves never lands behind it. Above `700px` the same
block is the last part of the sidebar (`.railagent`). It moves and is never
rendered twice, because two of them would be two Cores reporting one session.

`RAIL_LESS_SCREENS` is the stated exception to the shell: `onboarding`, `book`,
`client`, `preferences`, `unsubscribe`, `confirm`, `room`, `oauth-consent`.
These fill the whole screen with their own shell. A human who gives an agent
their authority reads that screen apart from the app, outside its shell. The
surfaces before a session (sign-in, the `availability` page, the loading
screen) use the same shell with no rail.

### A rail label is what shows, never a route id

This is the rule the next developer who adds a place is most likely to get
wrong. So it is stated in `nav.ts`, again in `palette.tsx`, and here.
`NavItem.screen` is the **route id**: the English name that never changes, in
the hash, in the `switch` of `App.tsx`, and in every `href`.
`NavItem.labelKey` is a **catalog key** whose rendered text may be different,
and one of the 8 is:

| Route id | Rendered label |
|---|---|
| `filters` | `Filters and views` |

The `⌘K` palette depends on the split. Every screen command carries its route
id, plus any `aliases` the rail entry declares, as `keywords` the user never
sees. Someone who types `filters`, `today` or `pipeline` still finds the place,
in any locale, with no second list of names kept by hand. **Never rename a
`screen` to match a label**: that breaks every existing hash URL and every
`SCREEN_ENTITY` / `OFF_RAIL_TITLE_KEYS` lookup keyed on it.

Some places are reached from Settings, from a record or from another screen,
never from the rail. Such a place resolves its page title through
`OFF_RAIL_TITLE_KEYS` in `app/pagemeta.ts`: `worklist`, `settings`, `offers`,
`partners`, `share`, `search`, `scheduled`, `tags`, `lists`. A raw screen id
never shows as a title.

### The badge policy

`BADGE_SCREENS` is empty: no place on the rail shows a count. A badge would
count only what needs a human to look, never a plain total. The
list endpoints page by key and return no total, so a count only for looks has
no real number to show. Approvals and tasks are lanes inside Home, which
reports its numbers on the page. The set stays because `badgeIds` is how a
rail level declares rows that may carry a badge. The levels below it use it
too.

## Colour

The app shell is **glass over the light page**, as
[DESIGN.md](../../DESIGN.md#6-the-shell) and the top of `src/app/shell.css`
state. It does not use the dark green field, so the marks on the rail use
ordinary theme tokens, not `white-alpha` on a dark field.

The dark rail group in `tokens.css` (`--bgRail`, `--railIconActive`,
`--overlayScrim`) is for the dark green field
only. Its comment says it has no theme, `white-alpha` in both themes, which is
correct for that field. That field is the tooltips the small rail shows, the bar
of the client surface, and the surfaces for the web site and the deck. A new
app panel styled from those tokens is styled, in error, as the public web site.

Both themes ship, and **the theme resolves before React starts**.
`main.tsx` calls `startTheme()` above `createRoot(...).render(...)`. That choice
lives in `src/app/theme.ts` and in no other file. A stated choice
(`margince.theme` in `localStorage`, `light` or `dark`) wins; otherwise the OS
`prefers-color-scheme` decides.

A third value, `system`, says to keep following the OS. An install that has
never made a choice resolves as `system`, and so does any value the build does not
know. While it is the choice, `theme.ts` watches `prefers-color-scheme` and
renders an open tab again when the OS setting changes. A stated choice drops
that watch. `startTheme()` sets it up at boot and not in the first control on
screen. The account panel owns the three-way choice, and it renders its control
only while it is open.

Because the theme resolves at boot, pages before sign-in follow the stored
theme. Signing out does not leave an old theme on screen, and loading the page
again does not show light for a moment before dark.

**Raw colour values live only in `tokens.css`.** Everything else (`brand.css`,
the CSS file of every component, every `.tsx`) reads `var(--token)`. A derived
value is a `color-mix()` of a source token, not a new hex value. So when the
dark theme makes its key colour lighter, that carries through, with nothing to
declare again per theme. Two checks enforce the rule (see below), and the files
it skips are **each named by path, never matched by shape**:

| Skipped | Why |
|---|---|
| `design-system/tokens.css` | the raw values are its job; `tokens.test.ts` pins each one |
| `index.html` | `<meta name="theme-color">` cannot read a CSS custom value |
| `design-system/provider-mark.tsx` | it carries the own sign-in marks of Google and Microsoft. The colours of another company are not ours to turn into tokens, and a provider mark in Ledger Green is a *wrong* mark |

`--overlayLight` and `--overlayDark` (`#ffffff` and `#000000`, with no theme)
live in `tokens.css` for the same reason. They are layers that cover the page,
not the colour of the product, and a raw value in any other file fails the gate.

## Where a value comes from

**`EvidenceMark` is how a value shows its source.** A value that did not come
from a user typing it carries a line of small points under it. Opening the mark
says where the value comes from, how much the system trusts it, the text it was
read from and when. It also offers a way through to the full history of that
field.

It takes the place of a **stack of three tags** under every value
(`ProvenanceTag` + `ConfidenceMeter` + `EvidenceChip`, three parts per field).
Three tags under a value are noise, and the reader loses the value they are
about. The mark keeps the record clear to read and puts the evidence behind one
click.

The old parts stay in two places, both by design:

- **Inside the mark.** The `EvidenceMark` panel renders `ProvenanceTag` itself,
  and states its trust as text, not as a bar.
- **On the staging surfaces**, whose job is to set a proposal beside what is
  stored. The first is the approvals lane of the Worklist (`screens/worklist.tsx`,
  `screens/approvalrow.tsx`). Then the confirm step of onboarding
  (`screens/onboarding-conversation/confirm-card.tsx`,
  `screens/onboarding-company-form.tsx`) and the Company-context settings
  screen. Last are the record surfaces that show a single line about where a
  value comes from (`contacts.tsx`, `leads.tsx`, `consent.tsx`, `history.tsx`).
  `StagingCard` and `FieldDiff` are the composed forms of the same parts.

**One open at a time, by click *and* by key.** `closeOpenMark`, at module
level, holds the one open panel; opening a new one removes the last one from the
screen. A click outside the panel already removes it, so a user who clicks gets
this already. The registry gives it to a user who moves down a column of marked
values with the tab key. That user leaves one panel behind, not a stack of
panels on top of each other. The Escape key removes the panel and returns focus
to the mark that opened it.

The panel is a named `<section>`, not a `dialog`. It opens beside the value, the
page behind it still works, and nothing holds focus in place. A mark with **no
source renders as plain text**. A line under a value that opens an empty panel
makes the reader stop opening them.

## The Core

`MarginceCoreScene` (`design-system/margince-core.tsx`) is the one mark of AI
identity in the product. It shows on the surface before sign-in, the session
loading screen, onboarding and the agent rail. Four rules apply to it:

- **One copy.** A caller passes `state` and never changes its style. Setting its
  size through the stated `--coreSize` and `--coreGlass` custom values is
  config. Anything beyond that is a caller changing the style of a shared part.
- **The state list is closed**: `idle`, `ingest`, `working`, `warning`,
  `error`. Callers use the Core as a *status channel* (a sign-in on its way, a
  server that cannot be reached). A status channel with an open list of states
  cannot be tested, and a second caller cannot use it again. Red means not
  connected and nothing else; `warning` is the fault that can wait. `progress`
  is not required, and the mark for it shows only when it is passed.
- **Rendering is a fallback ladder.** The WebGL2 shader comes first
  (`margince-core-shader.ts`). A host without it gets a plain CSS version that
  carries the same `data-core-state`. Nothing that reads the state of the Core
  off the DOM can tell the two apart.
- **It is `aria-hidden`.** The surface around it also states in text every
  state it shows. That is what makes it safe for the Core to be only for looks.

The orb in the agent section **is** the Core (`MarginceCoreScene`). There is one
orb in the product. A CSS copy that never leaves the shell would be a second
one, which a reader must tell apart from the real thing. For the same reason it
is the only one on screen at a time, so the panel it opens carries none.
`agentrail.css` says the same beside the rule that sets its size.

## The gates

`make check-fe` → `make frontend-check` is the merge lane, and it runs in this
order. Four shell script checks that fail closed come first, so the rules hold
even if the test tree breaks.

| Gate | Where it lives | What fails it |
|---|---|---|
| Tokens only | `frontend/scripts/check-ds-purity.sh` | a raw hex, `rgb()`, `rgba()`, `hsl()`, `hsla()` or `oklch()` value in a shipped `.ts`, `.tsx` or `.css` file written by hand under `frontend/src` or `extensions/*/frontend`. It skips four names: `tokens.css` (the raw values are its job), `provider-mark.tsx` (the marks of other companies), `*.test.*` (test data) and the generated `schema.d.ts`. `index.html` is skipped too, for a different reason: the script walks `frontend/src` and `extensions/*/frontend`, and `index.html` is above both. Fails closed if it scans zero files |
| Type list | `frontend/scripts/check-font-lock.sh` | a `font-family` other than Outfit (titles), Geist (body text, numbers too) or Geist Mono (code only), plus the named fallbacks; and Mono in any place but `code`, `pre`, `samp` and `.code-block`, which `design-system/mono.test.ts` refuses too |
| No emoji | `frontend/scripts/check-icon-glyph.sh` | an emoji in rendered code. Comments are removed first, since the 🟢/🟡 tier marks are our style and render through `AutonomyDot` |
| Spacing | `frontend/scripts/check-ds-spacing.sh` | a **new** `margin`, `padding` or `gap` in `px`, written in place. It reads only lines changed against `origin/main`, and skips a line marked `// ds:ignore <reason>` |
| Spacing roles | `frontend/scripts/check-ds-spacing-roles.sh` | a screen rule that sets the space of a design-system part the design system already spaces, or sets the size of one it already sizes: `font-size`, `line-height`, `letter-spacing`. Both lists are derived from `design-system/*.css` on every run. Also a rule that writes a fixed step where a role exists: the `*-actions` gap, the `*-cards` gap, the `*-card` or `*-panel` `padding`. **Whole tree**: moving a CSS class into the design system turns screen rules nobody touched into findings in lines no change holds. A case with no role to name it is skipped with `/* ds:ignore <reason> */` on its line. `check-ds-spacing-roles.test.sh` tests its verdict against test trees |
| Type source | `design-system/type-source.test.ts` | text type set by value in any file but `tokens.css`. In every `.css` under `frontend/src` and every style object outside tests, `font-size`, `line-height`, `letter-spacing` and `text-transform` may say only `inherit`. `font` takes a `--font*` token and `font-weight` a `--fontWeight*` one. Two values that place things are accepted, only inside the two UA style blocks. It refuses `uppercase` text by every route: `font-variant-caps`, the `font-variant` short form, the `small-caps` features, and a `.toUpperCase()` whose result is rendered. Fails closed: a set with no `app.css`, under 100 CSS files or under 100 component files fails and does not pass empty |
| Action rows | `design-system/actionrow.test.ts` | an element whose direct parts are two or more buttons and nothing else, and that does not get `gap: var(--gapActions)` from a class it names or from its own `style` value. That includes a class that **no CSS file declares**, which a gate that reads only CSS cannot see. A line is skipped with `{/* ds:ignore <reason> */}` |
| Contract type drift | `make frontend-check` | `pnpm gen:api` changes `src/api/schema.d.ts` or `public-events.ts` |
| Lint | `pnpm lint` (Biome) | style and lint findings over `src` and `index.html` |
| Design test | `design-system/conformance.test.ts` | the same rules read from the AST, not from text, plus copy users see that is written in the code outside the `i18n` catalogs, and a wrong web app manifest |
| Service worker | `frontend/scripts/vite-pwa.test.ts`, `frontend/src/app/serviceworker-registrar.test.ts` | the web app build not writing `/sw.js`. The worker answering from `Cache Storage` anything but a failed page load and the offline page with its script. The worker catching a path the API owns, keeping a cache that is not its own, or keeping its cache name when what it holds changed. Any shipped module but `src/app/pwa.ts`, in any kind of script, reaching for `navigator.serviceWorker` ([pwa.md](pwa.md)) |
| CSS file owners | `design-system/stylesheetnamespace.test.ts` | the class names of a screen declared in a CSS file other than the screen's own, in every `.css` under `frontend/src` and in the frontend layer of each extension |
| Timeline rows | `design-system/timelinerows.test.ts` | a rule that reaches the `li` of a `.timeline` list by anything but `>` from the `.timeline` part, which also reaches `li` items inside a row; a sibling (`+`, `~`) of such a row passes. It reads every `.css` under `frontend/src` and the frontend of each extension. It fails closed when the selectors it parses and a plain count of the text disagree |
| Token values | `design-system/tokens.test.ts` | a Ledger-Green value that drifts from the design source |
| Type check and build | `pnpm build` (`tsc -b && vite build`) | any type error |
| Unit tests | `pnpm test` (Vitest) | the `*.test.tsx` files beside the code |
| Render UAT | `make fe-uat` → `frontend/scripts/fe-uat.mjs` | a changed component with no story beside it, a changed story the build does not register, or a render that fails when run with no screen. It is **outside** `make check`: it is the UAT lane for the frontend alone, and it writes its result to `.tmp/fe-uat/manifest.json` |
| Screen checks | `make frontend-e2e` → `frontend/e2e/` | Playwright cases named for their AC, axe at WCAG 2.2 AA, and the `390px` `no-horizontal-scroll` sweep. The speed budget is the one in `make bench-mobile`: a `p95` over many runs, because one clock reading in a shared lane measures the machine it runs on |

The `craft static` hook reads only the comments of a `frontend/` file. The
frontend lane is separate from the Go merge gate and needs Node and pnpm. Run `make check-fe` (or `make frontend-check`) before pushing a
frontend change.

## Where to look first

| If you are changing… | Start at |
|---|---|
| a place on the rail, a rail label, a badge | `src/app/nav.ts`, then `src/app/shell.tsx` and `shell.test.tsx` |
| what a route renders | `src/App.tsx` (`ScreenView`), then the screen file |
| a colour, a `border-radius`, a spacing step | `src/design-system/tokens.css` (and `tokens.test.ts`), never a call site |
| a derived colour role | `src/design-system/brand.css`: a `color-mix()`, never a new hex value |
| light and dark themes | `src/app/theme.ts` and `tokens.css`, which carries all three states: the light colour set on `:root` alone, the `prefers-color-scheme` block for a surface whose host states nothing, and the `[data-theme]` blocks a stated choice sets |
| how a derived value shows its evidence | `src/design-system/evidencemark.tsx` |
| a staging or approval surface | `src/design-system/trust.tsx` and `src/screens/worklist.tsx` |
| copy | `src/i18n/en.ts`, `de.ts` **and** `vi.ts`; the type check holds their keys the same |
| money, dates and time | `src/format/format.ts`, apart from which calendar day a point in time falls on, and the point in time where the day the user set ends: those are in `src/format/calendarday.ts` |
| an API call | `src/api/client.ts` is the seam; generate the types again with `pnpm gen:api` |
| how the Core looks, or its states | `src/design-system/margince-core.tsx`, `margince-core-shader.ts` and `margince-core-motion.ts` |

## Where the code lives

| | |
|---|---|
| The API seam and the generated contract types | `frontend/src/api/{client.ts,schema.d.ts,public-events.ts}` |
| Boot: theme, install offer, service worker, query client, 403 handling | `frontend/src/main.tsx` |
| Registering the service worker, install state / the worker and its offline page | `frontend/src/app/pwa.ts` / `frontend/src/offline/`, built by `frontend/scripts/vite-pwa.ts` |
| Route → screen, the sign-in gate, the onboarding gate | `frontend/src/App.tsx` |
| The shell, sidebar, page title | `frontend/src/app/{shell.tsx,shell.css}` |
| Top bar: breadcrumb, search, account | `frontend/src/app/{topbar.tsx,topbar.css,account.tsx}` |
| The rail list, the badge set, the phone set, the set with no rail | `frontend/src/app/nav.ts` |
| The hash router | `frontend/src/app/router.tsx` |
| The `⌘K` palette, the agent in the rail | `frontend/src/app/{palette.tsx,agentrail.tsx}` |
| How the theme resolves and is stored | `frontend/src/app/theme.ts` |
| Tokens (the source) / derived roles / plain controls | `frontend/src/design-system/{tokens.css,brand.css,base.css}` |
| Small parts, trust parts, composed surfaces | `frontend/src/design-system/{atoms,trust,composed}.tsx` |
| The evidence mark | `frontend/src/design-system/evidencemark.tsx` |
| The Core and the code that renders it | `frontend/src/design-system/margince-core.tsx`, `margince-core-{engine,gl,shader,motion}.ts` |
| The AI runtime tag | `frontend/src/design-system/airuntimechip.tsx` |
| Design gates (tests) | `frontend/src/design-system/{conformance,tokens}.test.ts` |
| Design gates (shell script checks that fail closed) | `frontend/scripts/check-*.sh` |
| Render UAT scoped to the change | `frontend/scripts/fe-uat.mjs` |
| Screens | `frontend/src/screens/` (one file per surface; `onboarding-conversation/` is the one state-machine folder) |
| Catalogs / the edge where values turn into text | `frontend/src/i18n/`, `frontend/src/format/` |

## Where to go next

- [company-record-page.md](company-record-page.md): the largest screen in this
  shape.
- [pwa.md](pwa.md): installing the app, the service worker and the offline page.
- [company-context.md](company-context.md): the onboarding steps and the
  company profile screens.
- [architecture.md](architecture.md): the Go part of the same contract.
- [../reference/make-targets.md](../reference/make-targets.md): every target
  named above.
- [../../frontend/README.md](../../frontend/README.md): commands, the
  `VITE_UI_PREVIEW_*` switches, working agreements.

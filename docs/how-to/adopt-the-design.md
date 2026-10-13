<!-- prose:plain -->
# Adopt the design

The plan, step by step, for landing [`DESIGN.md`](../../DESIGN.md) in the product. The design system comes
first. Then come the five record pages (company, contact, deal, lead, project), with every state they already
handle. Then the sub pages and the rest. Each numbered step is one pull request.

The plan builds the mock in the
Aurora artifact linked from `DESIGN.md`. Where the mock and the running product disagree on a fact, the product
is right, and the mock is corrected.

Two rules bind every step:

- **Nothing the pages say today goes away.** Every state, refusal and feature in sections 3–7 has a place in the
  new layout before the old one goes. A new style that drops a `"hidden from you"` sentence has failed, even if it
  looks right.
- **The head shows base verbs**, never the task of the day. `Write email · Log activity · Add task · more` on
  every company. The move the call names lives inside *What needs you*, with its reason. Ask is a pane of
  questions it has ready, not a free text field.

## 1. What the gates need from a new style

Read this before you touch a stylesheet. Each gate checks one clear rule.

| Gate | What it holds | What the new style must do |
|---|---|---|
| `tokens.test.ts` | Pins about 40 token values (`canonical`), the surface light ladders in both themes, AA contrast of five ink tokens on five ground tokens, chip colours, `--accent` = `#0b7a53`, `--bgRail` = `#13231d` and missing from dark, the dark `@media` block byte-equal to `[data-theme="dark"]`, and `brand.css` as derived values only | Change `tokens.css` and the `canonical` table in one commit. Keep both ladders in order. Check the contrast again on the new ground tokens. Keep the two dark blocks the same. Keep `--bgRail` declared (the rail stops using it, the pin stays) |
| `check-ds-purity.sh` + `conformance.test.ts` | No colour value outside `tokens.css` (only `provider-mark.tsx` may have one) | Every new colour is a token, the glow and each pane included |
| `check-font-lock.sh` + `conformance.test.ts` + `mono.test.ts` | Three font families only: Outfit, Geist, Geist Mono. Outfit stays by decision, after the display font of the mock was tried in Step 1. Geist Mono only on `pre`, `code`, `samp` and `.code-block`; `frontend/scripts/check-font-lock.sh` and `frontend/src/design-system/mono.test.ts` refuse it in any other place | Change the family in **four** places in one pull request: the strip list of the script, `allowedFamilies` in `conformance.test.ts`, `--f-*` in `tokens.css` (pinned), and the Google Fonts link in `index.html` |
| `check-ds-spacing.sh` | No new `px` value in `padding`, `margin` or `gap` under `screens/` and `app/` | Screen CSS uses `--space-*`; design-system CSS may keep a `px` value that looks right |
| `check-ds-spacing-roles.sh` | No screen rule changes the spacing of a design-system part that the design system spaces. No screen rule changes the size of one it sizes (`font-size`, `line-height`, `letter-spacing`). The three named places take their role token: the gap of `*-actions` → `--gapActions`, the gap of `*-cards` → `--gapCards`, the padding of `*-card` or `*-panel` → `--padCard` or `--padPanel`. Whole tree | Change a role in `tokens.css`, where every screen moves with it. A screen that needs its own space or size sets it on its own element. Or it changes the part with a role token, or turns the check off in line with a reason |
| `check-space-tokens.sh` | Every `var(--x)` is declared in some place | Rename a token only with all the places that use it |
| `actionrow.test.ts` | Two or more buttons side by side sit in a box that gets `gap: var(--gapActions)` | Give a row of verbs its own class with that gap, or use `.form-actions`, `.actions` or `.card-actions`. A class that no stylesheet has is the failure this gate exists for |
| `onecard.test.ts` | No second rule declares the full look of `.card` | When `Panel` becomes the pane, `.card` must not end up the same as it |
| `type-source.test.ts` | No size, `line-height`, `letter-spacing` or `font-weight` declared by value outside `tokens.css`. `font-size`, `line-height`, `letter-spacing` and `text-transform` may say only `inherit`; `font` reads a `--font*` token and `font-weight` a `--fontWeight*` one. Text in capital letters is refused in every form, including `.toUpperCase()` in code | Delete the declaration and let the element take it from the element that holds it, or name the token the role needs. A heading takes `Heading size=`, a caption `.t-caption`, and a control's name `.t-label` |
| `catalog.test.ts` | Every part the design system gives, named in the README table, with a story | A new or renamed part ships with its row and story |
| `native-controls.test.ts` | No `<select>` | Keep `Select` |
| `table-scroll-coverage.test.ts` | Every table inside `TableScroll` | Keep it |
| `conformance.test.ts` motion, focus, pending | `prefers-reduced-motion` answers come after their rule. Every outline is `--focus-ring`. One `ds-pulse` user. Three pinned outline rules that are only for looks | Add any new dashed outline to the allow list. The glow is a background, not a motion |
| `e2e` `ac.spec.ts` | WCAG 2.2 AA with axe on every core screen; no scroll to the side at 390px; the 10 rail entries in order | Run it in both themes after the token pull request |
| `e2e` `company-record.spec.ts` | `"KPI strip above the tab strip"`, `"one Company 360 card"`, `"left rail carries the account's context as one panel of named sections"`, `"lifecycle control is a control, not a tag"`, `"logo not favicon"` | These checks are about the old shape. Write them again **in the company pull request**, in the same commit as the layout, to the new shape. That is readings under the tab strip, the 360 as the first pane, and the details panel on the right |
| `e2e` `perf-mobile.spec.ts` | The record's `<h1>` draws from the router before any record read returns, `p95` under 300 ms on `Fast 3G` | The head keeps drawing from route state. Nothing in the head may wait on the 360 |
| `history.spec.ts` | A record's tab is an address | Keep tabs in the URL; the deal's tabs move there (section 5) |
| File limit of 500 lines | `companies.tsx`, `company360.tsx`, `companyheader.tsx`, `deals.tsx`, `leads.tsx` are already over | New code goes in new files; do not make these longer |
| `i18n.test.ts` | `en.ts`, `de.ts`, `vi.ts` carry the same keys | Every new key lands in all three |

## 2. The design system

The order is part of the plan. Tokens come first, because every later pull request is measured against them.
The shell comes last in this group: it is the part users see most, and the safest once the tokens hold.

### Step 1: Tokens and type (`tokens.css`, `tokens.test.ts`, `base.css`, `app.css`, `index.html`)

1. **The ground tokens.** Move the light ladder to the new ground.
   `--bgPage` → `#f1f5f2`, a light green, and `--bgElevated` → the full colour behind a pane.
   `--bgInset` and `--bgHover` sit one step apart.
   Add `--pane`, `--paneEdge`, `--glowA` and `--glowB`.
   The last two are the glow in each corner, at `.06`/`.10` light and `.10`/`.20` dark.

   Check the light ladder and the contrast again. The test checks the order of the ladder, so pick
   values that keep it, not values that look right alone.
2. **The ink tokens.** `--textPrimary/--textContent/--textTertiary/--textMuted/--textMeta` take the `--ink…--ink4`
   values from `DESIGN.md` section 3.
   All five must reach 4.5:1 on all five ground tokens in both themes.
3. **The accent and the agent.** `--accent` stays `#0b7a53` (pinned). The `--ai` family stays.
   Add `--aiBg` (the row colour) and `--aiLine` if `--aiLight` and `--aiMed` do not match the mock.
   Or map the mock to them, and update the mock.
4. **Rail tokens.** The design has no dark rail.
   `--bgRail` and the `--rail*` family stay declared (pinned), and `shell.css` stops using them.
   Note in `tokens.css` that they are no longer used.
5. **Fonts.** Outfit (display, 600), Geist (body, 400/500/600, numbers included), Geist Mono (code only,
   400/500).
   A number lines up through `font-variant-numeric: tabular-nums` (`.t-num`), not a mono font.

   Mono made money look like machine output, and was too much in a full row. `tabular-nums` gives the column
   line-up that mono was first picked for. Geist Mono reaches the page through one `base.css` rule on `pre`,
   `code` and `samp`, plus `.code-block`. A `<kbd>` is body type. The display font of the mock was tried in
   Step 1, and Outfit stayed by decision. A family change is four places in one pull request.

   Keep `--fontFamilyHeading`, `--fontFamilyBody` and `--fontFamilyMono` as the names.
6. **Type sizes.** Make the `.t-*` classes of `base.css` read `--fs-*`, not their own `px` (today they do not
   match).
   Set `--fs-body` 13.5px, `--lh-normal` 1.55, and `--fs-display` 32px with `--tracking-display` `-0.03em`.
   `body` in `app.css` reads the tokens, not `14px/1.5`.
7. **Corner radius and shadow.** The radius goes by role, and the ladder in `tokens.css` is the contract.
   `--r-lg` is 20px for a pane, the details panel and a reading card.
   `--r-md` is 16px for a board card and the agent's row.
   Then `--r-control` 12px, `--r-sm` 8px for a chip, and `--r-xs` 4px for a key.
   `--r-full` is for a pill or a monogram (`DESIGN.md` section 5).

   Where a browser has `corner-shape: squircle`, every step but `--r-full` is twice its size. A superellipse of
   radius `R` looks about as round as a round corner of `R/2`. One `@supports` block in `tokens.css` does this, and no
   call site touches it.

   There are three shadow tokens. `--shadow-rest` is one 1px/2px layer, used by every surface at rest and every
   filled control. `--shadow-well` is the same layer turned `inset`, used by every field, because a field has a
   floor instead of a top. `--shadow-pop` is for each popover and the drawer. A control drops its layer on hover,
   on click and on focus.
8. **Dark.** Add the new tokens to both dark blocks, with the dark values from `DESIGN.md` section 3.
   The two blocks must stay byte for byte the same as each other, not as the light one.
   The gate holds that.

The step is finished when `make fe-unit` is green with the new `canonical` table, and `make fe-ds-gates` is
green. `ac.spec.ts` is green in both themes. Storybook shows every story on the new ground, and no reader would
say any of them looks wrong.

### Step 2: Small parts (`atoms.tsx`, `atoms.css`, `base.css`)

| Part | Change |
|---|---|
| `Button` | Flat. `primary`: `--accent` fill, `--textOnAccent`, 36px, 10px radius, `font-weight` 500. `ghost`: `--pane` fill, `--line2` outline. Small: 32px. Icon only: 36×36 (the more button). The agent's fill for Accept on a staged row is the `variant="ai"` that exists. No gradient, no `3D`. |
| `Badge` | One part, one size (20px: an 18px line inside a 1px edge), no capital letters, no dot by default. `soft` is the default: the colour of the tone with its Text ink and a hairline in the tone, for a status beside text and down a column. `primary` is the full fill with a clear edge, for a count, and for the one status that must not be missed. Six tone values (`default`, `accent`, `success`, `warn`, `danger`, `ai`). `ai` always draws Sparkles. Others may take an icon left of the label, or `live` for a dot that pulses in the same place. |
| `Card` | Keep it as it is (`onecard.test.ts`); the pane is `Panel`, not `Card`. |
| `StatCard` | The reading card. The eyebrow is its label, with a 26px number in the display font, tabular (down to 20px where five share a narrow row). Then a 12.5px basis, `--pane` ground, 18px radius and `min-height` 138px. `numeric` for numbers. |
| `Skeleton`, `PendingBody`, `EmptyState` | Set the colours with tokens. `EmptyState` sits left in its pane, with one sentence and one verb. |
| `SegmentedControl`, `TextInput`, `Select`, `ComboBox`, `Kbd`, `Modal`, `Callout`, `Switch` | Token and radius pass only. The compose drawer becomes `Modal intent="drawer-reading"` (880px, `--bgElevated`). |
| `.t-eyebrow` | The one small type in capital letters: 10.5px, `.08em`. Move the eyebrow baseline down where the new style removes words said twice. |

Stories: update every changed story. The catalog test needs no new rows unless a part is added
(`variant="agent"` is a prop, not a part).

### Step 3: Composed and record parts

| Part | Change |
|---|---|
| `Panel` / `PanelPlate` / `PanelBody` / `PanelRow` (`panel.css`) | The **zone pane**: `--pane` with `--paneEdge`, 18px radius, `backdrop-filter: blur(12px)`, padding `--space-5 --space-6`. A 17px title in the display font, with its count (`small`) and verb (`a`) on one line. Rows at `--space-3`, each with a hairline. `PanelPlate` becomes a row on `--bg3`. |
| `StatStrip` | Becomes the readings row: five `StatCard` parts in a grid, 16px gap, no plate. A slot that cannot be read stays absent or says so (the rule the strip has today). |
| `RecordView` (`composed.tsx`, `composed.css`) | The head: the 56px mark and 32px name **set on one line**. The facts line is 13px, wraps in rows, and carries the live dot and the way in. Verbs sit right, and wrap under when the page is narrow, with the `more` icon button last. Remove `PageAsideToggle` from `actions`, and draw it at the right end of the tab strip. The `controls` slot (deal only) is dropped: the deal's value, stage and owner move to the facts line. `nameBadge` stays on the name line. |
| `RecordTabs` | Plain: no line under the strip, 13px, a 2px `--accent` line under the open tab, counts at 11px `--ink4`. A `trailing` slot for the Details control. The strip runs the full width above the columns, so the details pane opens under it. |
| `PageAside` | The details pane: the aside slot of `RecordView`, 300px, under the tab row beside the work. One pane, **closed by default**, that keeps its state for each reader. A screen claims it with `usePageAside`, and hands `RecordView` its content only while it is open. The parts inside the pane decide their own layout (section 3.4: five subjects or one story). |
| `PageZones` | `.page-zones-aside` becomes `minmax(0,1fr) 300px`. The `both` and `rail` shapes stay for pages that use them. |
| `GroupedTimelineList` / `TimelineRow` (`composed.css .timeline`) | Already the rail. Style only: a 76px date column in tabular numbers, and marks by kind. A full dot; an empty ring for `change`; indigo for an agent change; an indigo ring of dashes for staged; a sign in a ring for a thread group. Then a kind eyebrow, words for in or out, a 13.5/600 title, text that stops at three lines, and a meta line. A thread group is a card on the body side. |
| `record360/spine.css` | Keep the shape (it belongs to the product). Take the sizes of the mock: the gap day count at 26px display amber, the today bar 2×15px, a grey line of dots for the days to come. Nothing about how it is built changes. |
| `record360/verdict.tsx` + `brieftitle.tsx` + `citations.tsx` | The 360 pane. `BriefTitle` becomes the indigo tile, `"{name} · 360"`, `"read this record {when}"` and `"Write it again"`. `VerdictHead` puts the standing word at 34px display, left of the sentence that says why. `Citations` draw inside the sentence as source chips. |
| `EvidenceMark` | Add a preview on hover and focus: the popover (`--bgElevated`, `--shadow-pop`, 300px). It holds the source text in an indigo block with a line on its left, and the source line. A click still opens the full proof (`EvidenceModal`). |
| `trust.tsx` (`StagingCard`, `FieldDiff`) | The agent's row: `--aiBg` ground, 14px radius, eyebrow in `--aiText`, a 16px display title, a sentence, `Rests on` chips and verbs. Staged: an `--aiLine` edge of dashes, and Accept in the agent fill. |
| `DecisionCard`, `BriefItemCard`, `Callout` | Token pass. `Callout` is a row with a tone dot, never a filled box. |
| `RelationshipMap` (`relationshipmap.css`) | Colours only. Each node box on `--pane`, and the gap node in amber dashes. Edges by band: 2.5px accent for a close link, dashes for one that is new, dots for one with no contact for a long time. The node a user picks lights in ink and the rest drop to 35%, and the panel is a pane. The shape does not change (`relationshipmap.layout.ts` only works out the layout, and is tested). |
| `ListTable` / `DataTable` | Headers 11.5px `--ink3`, 44px rows, a hairline under each, numbers right and tabular (`.t-num`), the picked row on `--accentBg`. |

### Step 4: The shell (`shell.css`, `shell.tsx`, `topbar.css`, `agentrail.css`, `agent-edge.css`, `navlevel.tsx`)

1. **The ground and glow.** `.app` draws `--bgPage` with a round glow at the top left and the top right, as a
   background.
   That is `--glowA` and `--glowB`; they are not an element, and not a motion.
2. **Rail.** `glass` (`--pane` and `blur`) over the glow, a hairline on the right, no dark ground.
   Closed, it stays **64px**. The 44px touch targets and the tooltip rule in `shell.css` depend on that width.
   Breaking them for the 52px of the mock costs too much. Open, it is 224px, not 252.

   Rows are 34px, group labels are `.t-eyebrow` with only the first letter capital, and the orb sits
   last. `rail.test.tsx` and `ac.spec.ts` pin the 10 rows and their order, and those do not change.
3. **Top bar.** It stays 50px, `glass`, with a hairline under it.
   The breadcrumb sits left, the account menu right, and the `⌘K` field between them.
   **Nothing from a record sits in it.**
4. **Settings second level.** On a settings route, the sidebar itself becomes the 210px column that carries the
   level (`SettingsRail`/`navlevel.tsx`).
   It uses the same `glass`.

   It still shows one level at a time. The rail's places step aside for the section's entries, with Back above
   them. `rail.test.tsx` and `ac.spec.ts` hold one level in the sidebar. The two columns side by side in the mock
   would change how users move through the app. So they are out of scope for a new style.
5. **Agent chrome.** `agentrail` and `agent-edge` take the tokens; the orb stays where it is.

The step is finished when `ac.spec.ts`, `rail.test.tsx`, `shell.test.tsx` and `shell.stories` pass in both
themes, with the 390px sweep.

## 3–9. The pages

The record pages, each with every state it handles today, are in
[adopt-the-design-records.md](adopt-the-design-records.md). Section 3 is the company, as the reference; then
section 4 contact, section 5 deal, section 6 lead and section 7 project. The rest is in
[adopt-the-design-surfaces.md](adopt-the-design-surfaces.md) (section 8, sections `8a–c`, section 9). That
covers the sub pages and maps, both sides of the Deal Room, and the width ladder. It also covers empty and full
records, and the other screens. The section numbers below point to those pages.

## 10. Order and pull request list

| Pull request | Scope | Gate to watch | Status |
|---|---|---|---|
| 1 | Step 1: tokens, fonts (four places), type sizes, base | `tokens.test.ts`, font lock, `ac.spec.ts` in both themes | Landed in [#3835](https://github.com/margince/margince/pull/3835) |
| 2 | Step 2: small parts and stories | `catalog.test.ts`, `onecard`, eyebrow baseline | Landed in [#3842](https://github.com/margince/margince/pull/3842) |
| 3 | Step 3: `Panel`, `StatCard` and `StatStrip`, the `RecordView` head, the `trailing` slot of `RecordTabs`, `PageAside` on the right and closed, timeline, spine, 360 kit, `EvidenceMark` hover, trust rows, map colours, the width ladder (section 8a) | `company-record.spec.ts` goes red here. Keep the old checks until pull request 4 by putting the head layout behind a setting, or land 3 and 4 together | Landed in [#3872](https://github.com/margince/margince/pull/3872) |
| 4 | Step 4: shell, top bar, settings second level, agent chrome | `rail.test.tsx`, `ac.spec.ts`, 390px sweep | Landed in [#3889](https://github.com/margince/margince/pull/3889) |
| 5 | Section 3 Company, with its new `e2e` checks and state stories | `company-record.spec.ts`, `history.spec.ts`, `perf-mobile` | Landed in [#3929](https://github.com/margince/margince/pull/3929) |
| 6 | Section 4 Contact | `contact-network.spec.ts`, `contactpage.test.tsx` | Open |
| 7 | Section 5 Deal, tabs into the URL | `deals.test.tsx`, `history.spec.ts` | Open |
| 8 | Section 6 lead and section 7 project | `leads.spec.ts`, `projects.spec.ts` | Open |
| 9 | Section 8 sub pages and maps | `recordtabs.spec.ts` | Open |
| 10+ | Section 9, one screen each | | Open |

Every pull request needs `make check` (both parts), Storybook in both themes, and the axe tests. It also needs
a light and a dark screenshot in the pull request body, and the `i18n` keys in all three files.

## 11. Fixes to the mock

Facts that the review showed the mock gets wrong. Fix them in the artifact and in `DESIGN.md` before pull
request 5:

- The closed width of the rail is 64px in the product, for touch targets. The mock draws 52px. Keep 64.
- The deal's stage stepper is a `fieldset` group, and it moves on with a confirm on a last stage. Each pill in
  the mock is only for looks.
- The project has no 360 written by an agent, and no tabs. The project verdict word `Slipping` in the mock is
  made up. The pane is put together, without a word, until a project read exists.
- In the product, the readings of a lead are score, status, source, company and first response. `Your move` and
  `Next` are derived in the mock. Keep the slot list of the product. Add the two derived ones only if the 360 carries
  them.
- The contact's main verb names the transport when there is only one. The fixed `Write email` of the mock is
  the base word for the plan. The transport name is allowed as the label when it is the only one.

# Adopt the design

The step-by-step plan for landing [`DESIGN.md`](../../DESIGN.md) in the
product: the design system first, then the five record pages (company,
contact, deal, lead, project) with every state they already handle, then the
sub pages and the rest. Each numbered step is one PR. The mock the plan
implements is the Aurora artifact linked from `DESIGN.md`; where the mock and
the running product disagree on a fact, the product wins and the mock is
corrected.

Two rules that bind every step:

- **Nothing the pages say today is lost.** Every state, refusal and feature
  in §3–§7 has a home in the new layout before the old one is removed. A
  restyle that drops a "hidden from you" sentence has failed even if it
  looks right.
- **The head shows base verbs, never the task of the day.** `Write email ·
  Log activity · Add task · more` on every company; the move the call names
  lives inside *What needs you* with its reason. Ask is a pane of prepared
  questions, not a free-text field.

## 1. What the gates require of a restyle

Read before touching a stylesheet. Each gate is exact, not fuzzy.

| Gate | What it holds | What the restyle must do |
|---|---|---|
| `tokens.test.ts` | Pins ~40 token values (`canonical`), the surface-luminance ladders in both themes, AA contrast of six inks on five grounds, chip composites, `--accent` = `#0b7a53`, `--bgRail` = `#13231d` and absent from dark, the dark `@media` arm byte-equal to `[data-theme="dark"]`, `brand.css` derivation-only | Change `tokens.css` and the `canonical` table in one commit; keep both ladders monotone; re-run the contrast math on the new grounds; keep the two dark arms identical; keep `--bgRail` declared (the rail stops using it, the pin stays) |
| `check-ds-purity.sh` + `conformance.test.ts` | No colour literal outside `tokens.css` (one exemption: `provider-mark.tsx`) | Every new colour is a token; glows and panes included |
| `check-font-lock.sh` + `conformance.test.ts` | Exactly three families: Outfit, Geist, Geist Mono (Outfit kept by decision after the mock's display face was tried in Step 1) | Change the family in **four** places in one PR: the script's strip list, `allowedFamilies` in `conformance.test.ts`, `--f-*` in `tokens.css` (pinned), the Google Fonts link in `index.html` |
| `check-ds-spacing.sh` | No new raw px in padding/margin/gap under `screens/` and `app/` | Screen sheets use `--space-*`; design-system sheets may keep optical px |
| `check-space-tokens.sh` | Every `var(--x)` is declared somewhere | Rename a token only with all its consumers |
| `onecard.test.ts` | No second rule declares `.card`'s full chrome | When `Panel` becomes the pane, `.card` must not end up identical to it |
| `eyebrow-one-spelling.test.ts` | Frozen per-sheet count of uppercase micro-type restatements | Use `.t-eyebrow`; lower the baseline when a restyle removes copies |
| `catalog.test.ts` | Every exported primitive named in the README table with a story | A new or renamed primitive ships with its row and story |
| `native-controls.test.ts` | No `<select>` | Keep `Select` |
| `table-scroll-coverage.test.ts` | Every table inside `TableScroll` | Keep it |
| `conformance.test.ts` motion/focus/pending | Reduced-motion answers ordered after their rule; every outline is `--focus-ring`; one `ds-pulse` wearer; three pinned decorative outlines | Add any new dashed outline to the allowlist; the glow is a background, not an animation |
| e2e `ac.spec.ts` | WCAG 2.2 AA with axe on every core screen; 390px no horizontal scroll; the rail's ten items in order | Run it in both themes after the token PR |
| e2e `company-record.spec.ts` | "KPI strip above the tab strip", "one Company 360 card", "left rail carries the account's context as one panel of named sections", "lifecycle control is a control, not a tag", "logo not favicon" | These assertions describe the old shape. Rewrite them **in the company PR**, in the same commit as the layout, to the new shape (readings under the tab strip, the 360 as the first pane, the context panel on the right) |
| e2e `perf-mobile.spec.ts` | The record's `<h1>` renders from the router before any record read returns, p95 under 300 ms on Fast 3G | The head keeps drawing from route state; nothing in the head may wait on the 360 |
| `history.spec.ts` | A record's tab is an address | Keep tabs in the URL; the deal's tabs move there (§5) |
| 500-line file cap | `organizations.tsx`, `company360.tsx`, `companyheader.tsx`, `deals.tsx`, `leads.tsx` are already over | New code goes in new files; do not grow these |
| `i18n.test.ts` | `en.ts`, `de.ts`, `vi.ts` carry the same keys | Every new key lands in all three |

## 2. The design system

Order matters: tokens first, because every later PR is measured against
them; the shell last in this phase, because it is the most visible and the
least risky once the tokens hold.

### Step 1 — Tokens and type (`tokens.css`, `tokens.test.ts`, `base.css`, `app.css`, `index.html`)

1. **Grounds.** Move the light ladder to the lit ground: `--bgPage` → the
   pale green `#f1f5f2`, `--bgElevated` → the pane's solid fallback,
   `--bgCard` and `--bgHover` a step apart, `--bgSidebar` → the glass value.
   Add `--pane`, `--paneEdge`, `--glowA`, `--glowB` (the two corner glows at
   `.06`/`.10` light and `.10`/`.20` dark). Re-run the luminance ladder and
   the contrast math; the ladder order is asserted, so pick values that keep
   it, not values that look right in isolation.
2. **Inks.** `--textPrimary/--textContent/--textSecondary/--textTertiary/
   --textMuted/--textMeta` take the `--ink…--ink4` values from `DESIGN.md`
   §3. All six must clear 4.5:1 on all five grounds in both themes.
3. **Accent and agent.** `--accent` stays `#0b7a53` (pinned). `--ai` family
   stays; add `--aiBg` (the row tint) and `--aiLine` if the existing
   `--aiLight`/`--aiMed` do not match the mock's values, or map the mock to
   them and update the mock.
4. **Rail tokens.** The design has no dark rail. `--bgRail` and the
   `--rail*` family stay declared (pinned) and stop being consumed by
   `shell.css`; note in `tokens.css` that they are retired.
5. **Fonts.** Outfit (display, 600), Geist (body, 400/500/600), Geist Mono
   (figures, 400/500). The mock's display face was tried in Step 1 and Outfit
   kept by decision; a family change is four places in one PR. Keep
   `--f-display`, `--f-body`, `--f-mono` as the names.
6. **Type scale.** Make `base.css`'s `.t-*` classes read `--fs-*` instead of
   their own px (they diverge today). Set `--fs-body` 13.5px, `--lh-normal`
   1.55, `--fs-display` 32px with `--tracking-display` -0.03em. `body` in
   `app.css` reads the tokens rather than `14px/1.5`.
7. **Radius and depth.** `--r-md` 12→14px, `--r-lg` 20→18px (the pane),
   `--shadow-card` becomes the hairline `--paneEdge` (no drop shadow on a
   pane), `--shadow-pop` stays for popovers and the drawer.
8. **Dark.** Add the new tokens to both dark arms with the dark values from
   `DESIGN.md` §3; the two arms must stay byte-identical to each other (the
   gate holds that), not to the light block.

Done when: `make fe-unit` green with the updated `canonical` table,
`make fe-ds-gates` green, `ac.spec.ts` green in both themes, and Storybook
shows every existing story on the new ground with no visual regression a
reader would call broken.

### Step 2 — Atoms (`atoms.tsx`, `atoms.css`, `base.css`)

| Primitive | Change |
|---|---|
| `Button` | Flat. Primary: `--accent` fill, `--textOnAccent`, 36px, 10px radius, weight 500. Ghost: `--pane` fill, `--line2` outline. Small: 32px. Icon-only: 36×36 square (the more button). The agent's fill for Accept on a staged row is the existing `variant="ai"`. No gradients, no 3D. |
| `Badge` | `quiet` becomes the default: a 6px dot and a word. The pill (`--bg3`, 20px) only for standing badges beside a name and the one status that must not be missed. |
| `Card` | Keep as is (`onecard.test.ts`); the pane is `Panel`, not `Card`. |
| `StatCard` | The reading card: the eyebrow as its label, 26px mono figure (down to 20px where five share a narrow row), 12.5px basis, `--pane` ground, 18px radius, 138px min height. `numeric` for figures. |
| `Skeleton`, `PendingBody`, `EmptyState` | Recolour to tokens; `EmptyState` left-aligned in its pane, one sentence and one verb. |
| `SegmentedControl`, `TextInput`, `Select`, `ComboBox`, `Kbd`, `Modal`, `Callout`, `Switch` | Token and radius pass only. `Modal placement="right"` becomes the compose drawer's surface (560px, `--paneSolid`). |
| `.t-eyebrow` | The one uppercase micro-type: 10.5px, `.08em`. Lower the eyebrow baseline where the restyle removes restatements. |

Stories: update every changed story; the catalog test needs no new rows
unless a primitive is added (`variant="agent"` is a prop, not a primitive).

### Step 3 — Composed and record primitives

| Primitive | Change |
|---|---|
| `Panel` / `PanelPlate` / `PanelBody` / `PanelRow` (`panel.css`) | The **zone pane**: `--pane` with `--paneEdge`, 18px radius, `backdrop-filter: blur(12px)`, padding `--space-5 --space-6`, a display-face 17px title with its count (`small`) and verb (`a`) on one line, rows at `--space-3` with hairlines. `PanelPlate` becomes a row on `--bg3`. |
| `StatStrip` | Becomes the readings row: five `StatCard`s in a grid, 16px gap, no plate. Slots that cannot be read stay absent or say so (the strip's current rule). |
| `RecordView` (`composed.tsx/.css`) | The head: 56px mark and 32px name **centred on one axis**; facts line 13px, wraps in rows, carries the live dot and the way in; verbs right-aligned, wrap under at narrow widths; the `more` icon button last. Remove `PageAsideToggle` from `actions`; render it at the right end of the tab strip. The `controls` slot (deal only) is dropped: the deal's worth/stage/owner move to the facts line. `nameBadge` stays on the name line. |
| `RecordTabs` | Quiet: no rule under the strip, 13px, 2px `--accent` under the current tab, counts at 11px `--ink4`; a `trailing` slot for the Details control. |
| `PageAside` | Right column, 300px, one pane, **closed by default**, remembers its state per reader (already does). The aside's own children decide their inner anatomy (§3.4: five subjects vs one story). |
| `PageZones` | `.page-zones-aside` becomes `minmax(0,1fr) 300px`; `both` and `rail` shapes stay for pages that use them. |
| `GroupedTimelineList` / `TimelineRow` (`composed.css .timeline`) | Already the rail. Restyle only: 76px mono date column, marks by kind (solid, hollow for `change`, indigo for an agent change, dashed indigo for staged, circled glyph for a thread group), kind eyebrow, direction words, title 13.5/600, text clamped to three lines, meta line; a thread group as a card on the body side. |
| `record360/spine.css` | Keep the geometry (it is the product's); take the mock's sizes: gap day count 26px display amber, today bar 2×15px, dotted grey ahead. Nothing structural. |
| `record360/verdict.tsx` + `brieftitle.tsx` + `citations.tsx` | The 360 pane: `BriefTitle` becomes the indigo tile + "{name} · 360" + "read this record {when}" + "Write it again"; `VerdictHead` puts the standing word at 34px display left of the because-sentence; `Citations` render inline in the sentence as source chips. |
| `EvidenceMark` | Add hover/focus preview: the popover (`--paneSolid`, `--shadow-pop`, 300px) with the quote in a left-ruled indigo block and the origin line; click keeps opening the full receipt (`EvidenceModal`). |
| `trust.tsx` (`StagingCard`, `ApprovalGate`, `FieldDiff`) | The agent's row: `--aiBg` ground, 14px radius, eyebrow in `--aiText`, headline 16px display, sentence, "Rests on" chips, verbs. Staged: dashed `--aiLine` edge, Accept in the agent fill. |
| `DecisionCard`, `BriefItemCard`, `Callout` | Token pass; `Callout` is a row with a tone dot, never a filled box. |
| `RelationshipMap` (`relationshipmap.css`) | Colours only: node boxes on `--pane`, gap node dashed amber, edges banded (strong 2.5px accent, developing dashed, cold dotted), selection lights in ink and fades the rest to 35%, panel as a pane. Geometry is untouched (`relationshipmap.layout.ts` is pure and tested). |
| `ListTable` / `DataTable` | Headers 11.5px `--ink3`, 44px rows, hairlines, figures right-aligned mono, selected row on `--accentBg`. |

### Step 4 — The shell (`shell.css`, `shell.tsx`, `topbar.css`, `agentrail.css`, `agent-edge.css`, `navlevel.tsx`)

1. **Ground and glows.** `.app` paints `--bgPage` with the two radial glows
   (`--glowA` top-left, `--glowB` bottom-right) as a background, not an
   element and not an animation.
2. **Rail.** Glass (`--bgSidebar` + blur) over the glow, a hairline on the
   right, no dark ground. Collapsed stays **64px** (the 44px touch targets
   and the tooltip constraint in `shell.css` depend on it; the mock's 52px is
   not worth breaking them), expanded 224px instead of 252. Rows 34px,
   sentence-case group labels as `.t-eyebrow`, the orb at the foot. The ten
   rows and their order are pinned by `rail.test.tsx` and `ac.spec.ts` and
   do not change.
3. **Top bar.** 50px stays; glass, hairline under; breadcrumb left, the ⌘K
   field centred, the SoR-mode chip and the account menu right. **Nothing
   that belongs to a record sits in it.**
4. **Settings second level.** On a settings route the sidebar itself becomes
   the 210px column that carries the level (`SettingsRail`/`navlevel.tsx`),
   on the same glass. It still shows one level at a time — the ten
   destinations step aside for the section's entries, with Back above them —
   because `rail.test.tsx` and `ac.spec.ts` hold exactly one level in the
   sidebar; the mock's two columns side by side would be a navigation change,
   not a restyle.
5. **Agent chrome.** `agentrail` and `agent-edge` take the tokens; the orb
   stays where it is.

Done when: `ac.spec.ts`, `rail.test.tsx`, `shell.test.tsx`, `shell.stories`
in both themes, and the 390px sweep.

## 3–9. The pages

The record pages, each with every state it handles today, are in
[adopt-the-design-records.md](adopt-the-design-records.md) (§3 company as
the reference, §4 contact, §5 deal, §6 lead, §7 project). The sub pages and
maps, both sides of the Deal Room, the responsive ladder, thin and rich
records and the remaining screens are in
[adopt-the-design-surfaces.md](adopt-the-design-surfaces.md) (§8, §8a–c,
§9). The section numbers below refer to those pages.

## 10. Sequencing and PR list

| PR | Scope | Gate to watch |
|---|---|---|
| 1 | Step 1: tokens, fonts (four places), type scale, base | `tokens.test.ts`, font lock, `ac.spec.ts` both themes |
| 2 | Step 2: atoms + stories | `catalog.test.ts`, `onecard`, eyebrow baseline |
| 3 | Step 3: Panel, StatCard/StatStrip, RecordView head, RecordTabs trailing slot, PageAside right/closed, timeline, spine, 360 kit, EvidenceMark hover, trust rows, map colours, the responsive ladder (§8a) | `company-record.spec.ts` will go red here; keep the old assertions until PR 4 by feature-flagging the head layout, or land 3 and 4 together |
| 4 | Step 4: shell, top bar, settings second level, agent chrome | `rail.test.tsx`, `ac.spec.ts`, 390px sweep |
| 5 | §3 Company, with its e2e rewrite and state stories | `company-record.spec.ts`, `history.spec.ts`, `perf-mobile` |
| 6 | §4 Contact | `person-network.spec.ts`, `person360.test.tsx` |
| 7 | §5 Deal, tabs into the URL | `deals.test.tsx`, `history.spec.ts` |
| 8 | §6 Lead + §7 Project | `leads.spec.ts`, `projects.spec.ts` |
| 9 | §8 sub pages and maps | `recordtabs.spec.ts` |
| 10+ | §9, one screen each | |

Every PR: `make check` (both halves), Storybook in both themes, the axe
suite, a screenshot pair (light, dark) in the PR body, and the i18n keys in
all three files.

## 11. Corrections to the mock

Facts the inventory found that the mock gets wrong, to fix in the artifact
and `DESIGN.md` before PR 5:

- The rail's collapsed width is 64px in the product for touch-target
  reasons; the mock draws 52px. Keep 64.
- The deal's stage stepper is a `fieldset` group and advances with a
  confirm on a terminal stage; the mock's pills are decorative.
- The project has no agent-written 360 and no tabs; the mock's project
  verdict word "Slipping" is invented. The pane is assembled, without a
  word, until a project read exists.
- The lead's readings in the product are score, status, source, company and
  first response; "Your move" and "Next" are derived in the mock. Keep the
  product's slots and add the two derived ones only if the 360 carries them.
- The contact's primary verb names the transport when there is exactly
  one; the mock's fixed "Write email" is the base word for the plan, with
  the transport name allowed as the label when it is the only one.

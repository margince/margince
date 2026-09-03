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
| `check-font-lock.sh` + `conformance.test.ts` | Only Outfit, DM Sans, JetBrains Mono | Change the family in **four** places in one PR: the script's strip list, `allowedFamilies` in `conformance.test.ts`, `--f-*` in `tokens.css` (pinned), the Google Fonts link in `index.html` |
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
5. **Fonts.** Bricolage Grotesque (display, 600), Geist (body, 400/500/600),
   Geist Mono (figures, 400/500). Four places, one PR. Keep `--f-display`,
   `--f-body`, `--f-mono` as the names.
6. **Type scale.** Make `base.css`'s `.t-*` classes read `--fs-*` instead of
   their own px (they diverge today). Set `--fs-body` 13.5px, `--lh-normal`
   1.55, `--fs-display` 32px with `--tracking-display` -0.03em. `body` in
   `app.css` reads the tokens rather than `14px/1.5`.
7. **Radius and depth.** `--r-md` 12→14px, `--r-lg` 20→18px (the pane),
   `--shadow-card` becomes the hairline `--paneEdge` (no drop shadow on a
   pane), `--shadow-pop` stays for popovers and the drawer.
8. **Dark.** Copy the light block's new tokens into both dark arms
   byte-identically; the mock's dark values are in `DESIGN.md` §3.

Done when: `make fe-unit` green with the updated `canonical` table,
`make fe-ds-gates` green, `ac.spec.ts` green in both themes, and Storybook
shows every existing story on the new ground with no visual regression a
reader would call broken.

### Step 2 — Atoms (`atoms.tsx`, `atoms.css`, `base.css`)

| Primitive | Change |
|---|---|
| `Button` | Flat. Primary: `--accent` fill, `--textOnAccent`, 36px, 10px radius, weight 500. Ghost: `--pane` fill, `--line2` outline. Small: 32px. Icon-only: 36×36 square (the more button). Add `variant="agent"` (`--ai` fill) for Accept on a staged row. No gradients, no 3D. |
| `Badge` | `quiet` becomes the default: a 6px dot and a word. The pill (`--bg3`, 20px) only for standing badges beside a name and the one status that must not be missed. |
| `Card` | Keep as is (`onecard.test.ts`); the pane is `Panel`, not `Card`. |
| `StatCard` | The reading card: uppercase 10.5px label, 26px mono figure (22px under 1400px), 12.5px basis, `--pane` ground, 18px radius, 138px min height. `numeric` on by default for figures. |
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
4. **Settings second level.** `SettingsRail`/`navlevel.tsx` render as a
   210px column beside the rail on the same glass.
5. **Agent chrome.** `agentrail` and `agent-edge` take the tokens; the orb
   stays where it is.

Done when: `ac.spec.ts`, `rail.test.tsx`, `shell.test.tsx`, `shell.stories`
in both themes, and the 390px sweep.

## 3. Company — the reference implementation

The company page goes first because it has the most zones, the most states,
and the tightest e2e. Every later page copies its decisions. Files:
`organizations.tsx` (over the cap: new zones go in new files under
`screens/company/`), `company360.tsx`, `companyheader.tsx`, `companyrail*.tsx`,
`companytoday.tsx`, `companywork.tsx`, `companyrecent.tsx`,
`companycommercial.tsx`, `companydossier.tsx`, `companypeople/`.

### 3.1 Head

- Verbs, unchanged in substance: **Write email (primary) · Log activity ·
  Add task · more**. The `more` menu keeps its seven items in order (Edit,
  Merge, Partner set-up, Share, Full history, Decisions, Archive) with the
  refusal caption first. The `PageAsideToggle` leaves `actions` for the tab
  strip's trailing slot.
- Facts line: domain · industry · size · owner · **way in** (restore the
  known loss noted in `companyheader.tsx:891`) · last contact. The
  lifecycle control stays a control on the name line (`CompanyLifecycleControl`),
  the relationship badges beside it.
- The live dot: "In conversation" from the 360's pulse; absent when the
  pulse section is withheld.
- Archived: `record.archivedReadOnly` once, before the verbs; create verbs
  removed.

### 3.2 Zones, in order, with their states

| Zone (mock) | Built from | Data | States it must keep |
|---|---|---|---|
| Readings row (5) | `StateStrip` → `StatStrip` of `PipelineCard`, `MoneyStat`, conversation (from `pulse`), last touch, next (`view.next`); **each card carries an evidence chip** (`EvidenceMark` with the hover popover, opening `EvidenceModal` on click: the rows summed, the read date, the connection) **and is a link to its tab** (`companyTabRoute`); the chip stops the click | `state_strip`, `useFinanceSummary`, the citations already on the strip | `co.strip.notAssessed`; finance `notACustomer / noConnection / unmapped / syncing / withheld / staleFigure / errorFigure / nothingBilled / error / loading`; `co.strip.unpriced`, `pricedPartly`, `convertedAsOf`; `co.section.restricted`; a slot that cannot be read is absent or says so, never €0 or "—" without a reason |
| The 360 | `TodayOnThisAccount` + `VerdictHead` (`HealthStat`/`AccountHealthStat` as the word and dims) + `RecordSpine` + the thread from `useChronologySlots` folded behind "Read the thread · N" | 360 sections, chronology | `today.quiet` (nothing to say draws a calm 360, not an empty pane); `today.failed`; `co.section.unavailable`; withheld sections drop their row and say so once; `since_last_visit` with a withheld baseline never becomes a claim; "Write it again" only when the reader may |
| Deep-read offer | `DeepReadCard` **leads the column** instead of the 360 when `nothingOnFile(view)` | | the two honest scan phases; `SiteReadPanel` pages read/skipped and why; 422 no website; 501 seam unwired; `SiteReadDeferral` |
| What needs you | one list: the moment as the lead row, `co.suggest.*` rows (`draftReply / openDeal / addTask` verbs; `add_task` has no surface, so dismiss only), tasks from `CompanyTasksTab`'s source, the next meeting (`onPrepareMeeting`) | 360 suggestions, tasks, meetings | `co.next.empty`; `co.suggest.more` on the cap; withheld suggestions remove the row, not the verb; `DecisionsChip` count stays in the menu |
| Deals | `CompanyWorkCard` (three rows, `workVerbs`) | `view.deals`, `view.projects` | `co.work.noDeals` + detail; `co.work.statusesWithheld`; `co.work.countAtLeast`; `leadingDeal` refuses to pick on a truncated page or mixed currencies |
| Ask (prepared questions) | `AssistantPanel` with a **fixed question list per record type** (new: `co.ask.q.*` keys), no free field; each row runs the existing ask with that question | | `co.ask.nothing`; disabled in overlay (`enabled={!overlay}`) |
| About | `DossierPanel` lead + paragraph + sources, `SignalsSection` rows, `GrowthFitPanel` verdict row (only when `!hasWorkInFlight`), "Profile" link | own reads | `co.dossier.empty` (write it), `co.dossier.stale` "Read over a month ago", `co.dossier.unavailable`; `co.factSuspect.*` shown with evidence |
| People (chips) | `PeopleSection` (`RAIL_ROW_LIMIT`) as chips with "+N" | 360 | withheld → absent with the sentence |
| Commercial | `CommercialSection` (`CompanyLastOffer`, `CompanyContractState`) moves to the **Deals tab** head and the details panel's contract line; on the overview only the contract line under the Deals pane title | | `contracts.state.none`; `co.commercial.truncated` |
| Details (right, closed) | `CompanyRail` as one pane with five titled sections (keep the five-subject anatomy inside the pane): Details grid with inline edit, Deals top 3, People top 3, Hold, Tags | | every rail state as today; absent while a composer is open |
| Chronology zone | stays the RecordView timeline slot, drawn on the History tab only (the 360 carries the fold) | | `timelineNotice`, `chronologyNotice` |

Header-level states: overlay replaces the **whole page** with
`OverlayFallback` (both triggers); `co.partial` on `view.isError`; 403 keeps
the one shared sentence; version skew banners unchanged.

### 3.3 Features to carry (checklist)

Present in the mock: 360 with sources, spine, folded thread, needs list with
suggestions and tasks, deals, about with sources and staleness, people,
details, tags, history rail, People map, Profile, Finance, Documents,
Partner, ⌘K, compose. **Not in the mock, must be built in this step:**
evidence receipts (`EvidenceModal` with steps) behind every source chip; fact
contradictions (`co.factSuspect`); the deep-read card and site-read panel;
the full-history modal with restore; the decisions panel from the menu;
counterparty hold row; VAT mark; custom fields on Edit; document extraction
staging on the Documents tab (three states); the meeting brief drawer;
hierarchy rollup with the FX 422; provenance line (`captured_by`, "agent:
deepread") restored under the facts; since-last-visit acknowledgement with
its 5 s dwell.

### 3.4 Tests

Rewrite the shape assertions in `company-record.spec.ts` in the same commit
(readings under the tabs; "one Company 360 pane" leads; context panel on the
right, closed, one pane of named sections). Update `company360.test.tsx`,
`companyheader.test.tsx`, `companyrail.test.tsx` for markup, not behaviour.
`history.spec.ts` unchanged. Storybook: `Records/Company` stories for every
state row in §3.2 (this is where "empty", "withheld", "never read", "stale",
"overlay" get their pictures).

## 4. Contact

Files: `personpage.tsx`, `person360.tsx`, `personrail.tsx`, `personstrip.tsx`,
`persontoday.tsx`, `personcards.tsx`, `personmemory.tsx`,
`personcorrections.tsx`, `personnetwork/`.

- **Head.** Verbs: **Write email (primary) · Call · Add task · more**. The
  primary keeps `primaryTransportAction`'s routing and refusal
  (`writeRefusal`: reachability first, then consent; an unanswered guard
  refuses nothing) but its label is the base word unless the transport is
  the only one. Call and Meetings stay as icon verbs or move into `more`.
  **Add to `more` what is missing today**: Edit (inline stays), Archive,
  Share, Research, Full history. Facts: title · employer · email · phone ·
  the way in (from the network's lead route).
- **Readings.** `PersonStrip`: whose move, open promises (`person.loops`),
  the deal she decides, next meeting, answers in. `record.notShown` with no
  tone on a withheld slot; `noOpenDeal`/`noMeeting` are answers, not
  withholdings.
- **The 360.** `PersonToday` (the moment, with `readiness()` reasons and
  freshness) as the word and sentence, `RecordSpine` from the 360's
  activities, the thread folded from the timeline tab's chronology. A quiet
  moment renders through the same pane (`isQuiet`).
- **What needs you.** The moment's actions (readiness on the button, blocked
  reason rendered), `PersonCommitmentsCard` rows, open loops, the next task.
  `runPersonMomentAction` still opens nothing for an unroutable action.
- **The deal she decides.** `PersonCommercialCard` + the room as chips.
- **Ask.** Prepared questions (`person.ask.q.*`).
- **Understanding her.** `PersonBriefCard` + `PersonMattersCard` as the
  Priorities / Objections / Success rows with `Absent()` kept, sources,
  "Correct something" → `EnrichedFields`.
- **Around her.** `WhoKnows`, `Employers`, the lead route.
- **Details (right, closed).** `PersonRail` stays **one pane with hairline
  slices** (its documented anatomy), plus `PersonEmailPanel` under it.
- **States to keep:** `person.page.loading` becomes a skeleton (the one page
  without one); `person.page.notOpened`; `ThinState`; `withheldSections`
  read once; consent verdict from the server key; `provider.profile.neverRun`
  mark on the Research tab (and a cancelled run reads as never run);
  `person.graph.*` incompleteness on the map; archived and overlay verb
  removal.
- **Tabs.** Unchanged (`persontab.ts`). Network keeps its order: decision
  strip, lead panel, routes, then the map (`person-network.spec.ts` asserts
  it).
- **Carry:** consent & channels (both places), hold section, relink,
  research drawer, meeting brief with its four refusals, intro requests and
  relay steps, composer intent plumbing.

## 5. Deal

Files: `deals.tsx` (over the cap: the page moves to `screens/deal/`),
`deal360/*`, `dealstatus.tsx`, `dealroom.tsx`, `dealfiles.tsx`.

- **Head.** Verbs: **Write email (primary, from `DealEmailAside`) · Log
  activity · Edit deal · more** (Archive, Share, Reopen when won/lost). The
  `controls` slot goes: worth, stage, owner, close, forecast, partner join the
  facts line, masked fields still **named** (`FieldGuard mode="masked"`).
  `dealPulse` becomes the live dot ("Your move"). Edit keeps
  `overlay.partialWriteBack`.
- **Readings.** `DealStrip` (money with the newest offer, close with
  provisional/waiting, people with the withheld flag, momentum) plus stage
  with days here.
- **The 360.** `DealStatusCardPanel` (Deal360) is the word, sentence and
  citations; the stepper (`fieldset.stepper`, a group not a nav) inside the
  pane above the spine; the ledger folded. `deal360.unreadable` when the
  story is not the promised shape. **Absent in overlay** with the pane
  saying why.
- **What needs you.** Deal360's next move as the lead row, `DealApprovals`
  as staged rows (dashed), the reply owed from `useWaitingReply`.
- **The buying committee.** `DealPeoplePanels` rows beside `DealCommitteeMap`
  (ghost seats stay).
- **Ask.** Prepared questions (`deal.ask.q.*`).
- **What this deal is.** Deal360's story with "What is holding this up" and
  "What the buyer wants"; an absent section renders nothing.
- **Offers.** `OffersPanel`. **Deal Room.** `DealRoomAside` card
  (`OpenRoomCard` when none).
- **Details (right, closed).** `DealSeats` (present in overlay, stating the
  refusal), `FxLine`, wait-until, forecast, custom fields, project link,
  partner attribution, files (top two).
- **Tabs.** `overview · history · documents`, **moved into the URL** to
  match the other records (`history.spec.ts`).
- **States to keep:** provisional close, masked fields, unknown standing
  never healthy, unmapped forecast rendered raw, archived once in the band,
  closed vs archived refusals, advance failure caption, one advance at a
  time, optimistic concurrency on edit, `OverlayUnavailable` on Files and
  History.
- **Carry:** `StartDeliveryPrompt`, won reason dialog, file hide (with undo)
  vs delete, rewrite the briefing, project binding through project write
  authority.

## 6. Lead

Files: `leads.tsx` (over the cap: the record moves to `screens/lead/`),
`leads.stepper.tsx`, `leadsignals.tsx`, `leadvocab.tsx`.

- **Head.** Verbs: **Qualify (primary, `lead.promote` with
  `promoteIneligible` on the control) · Write email · Edit · Disqualify ·
  more (Share)**. The "Lead" marker stays on the name line. Facts: title ·
  company (text, no record) · email · source and date · owner.
- **Readings.** `LeadStrip` re-cut to five: company, score (with override
  badge / top factor / `scoreNoSignals`), first response (SLA line, only
  when the target is on), next (the booked meeting), your move. Every slot
  states its absence in words.
- **The 360.** Assembled, not written: the standing word from
  `statusReading`, a sentence from the ladder explanation
  (`ladderExplanation`: who moved it and what it read), the ladder
  (`LeadStepper`), the spine from the timeline, the thread folded. The foot
  says "Assembled from your records · a lead carries no suggestions".
- **What needs you.** "Ready to qualify" when `promoteEligible` (the
  `PreviewSentence` as its reason, including `previewMergeWithheld`), the
  reply owed (`RecordEmailAside detectWaitingReply`), tasks.
- **The score.** `LeadScorePanel` promoted from the rail disclosure to a
  pane: breakdown, `ScoreShortfall`, override, `LeadManualSignals`.
- **If she is qualified.** The preview sentence and `PromotedLeadPanel`
  after the fact (the page stays; `lead.qualify.done` toast with the refs).
- **Ask.** Prepared questions (`lead.ask.q.*`).
- **Details (right, closed).** `LeadIdentityFields`, `LeadOwner`, custom
  fields.
- **States to keep:** the terminal sentence naming **which** closure
  (`terminalPromoted` vs `terminalDisqualified`), one id every refused
  control points at; write-failure callout in the band; overlay removes
  promote/disqualify/share; promotion outcome `unknown/pending/failed`;
  qualify amount without currency waits; disqualify reason required; one
  write at a time.

## 7. Project

Files: `project360.tsx`, `projectsections.tsx`, `projectreadings.tsx`,
`projectphase.tsx`, `projectcompanies.tsx`.

- **Head.** Verbs: **Edit project · more (Archive with the synthesised id
  and If-Match, Assign owner, Share)**. Log activity only if `LogActivity`
  gains a `project` entity type; otherwise it is not a base verb here. Facts:
  phase · client · owner · key · go-live.
- **Readings.** `RollupsStrip` as five cards (open, won, commitments, last
  activity with `rollups.never`, activity count) under `SurfaceState`.
- **The 360.** Assembled: phase word, the read-only sentence when it
  applies, `PhaseStepper`, the spine from the chronology, the thread folded.
  No verdict word is written for a project today; the pane says
  "Assembled from your records".
- **What needs you.** `CommitmentsCard` with the overdue badge; a blocker
  row when a contract or document is missing.
- **Deals.** `ProjectDealsCard` with `NewDealAction` (project write
  authority). **Companies.** `ProjectCompanies` by role. **Stakeholders.**
  `StakeholdersCard` as chips.
- **Ask.** Prepared questions (`project.ask.q.*`).
- **Details (right, closed).** Contracts, documents, phase history.
- **States to keep:** all nine through `SectionPanel`; `documents` never in
  `sections_omitted`; the one read-only sentence minted by the band; page
  cap `project.deals.more`; unknown phase word.
- **No tabs** today; none added.

## 8. Sub pages and maps

One PR per record after its overview lands:

- **History** on all four: the rail restyle from Step 3 with the filter
  strip above (`ChronologyFilter`, `TimelineFilterBar`); undo/reversal rows
  (`historyreversalrow.tsx`) as hollow-dot changes with Restore.
- **Company People**: `CoverageBand`, then the committee map
  (`mapModelFromCoverage`), then the roster; `IntroRequestModal`,
  `coverageexplorer.tsx` behind "Where are we thin".
- **Contact Network**: order pinned by e2e; the map's panel gets the
  `EdgeDetail` write.
- **Deal Documents / Files**, **Company Documents** with extraction staging,
  **Finance**, **Profile** with `ReferenceDisclosures`, **Partner**,
  **Meetings**, **Research** (never-run mark), **Data & tools**.

## 8c. The Deal Room, both sides

Two files, one board (`dealroomthreads.tsx` renders for both sides and takes
the verbs as callbacks; that stays).

- **Seller** (`dealroompage.tsx`, `dealroomaccess.tsx`, `dealroomdocuments.tsx`,
  `dealroomconversation.tsx`): the record-page shell from Step 3 (head with
  "Back to the deal", facts line, View as buyer · Room access · more), the
  state banner as the `band2` row, Title and welcome as a pane with an Edit
  verb over `RoomText`, the board with the four `DOCUMENT_GROUPS` as eyebrow
  labels and a `dcard` per document, room-wide threads below, and on the
  right `DealRoomAccess` (rows with capability, state, last seen, downloads,
  the row menu: issue a new link, change capability, revoke; the one-time
  link dialog with mailed/copied), what is shared, the lifecycle. Every
  `room.*` and `roompage.*` key stays; `FINISHED_STATES` and `refusalFor` keep
  removing the composer and the verbs.
- **Buyer** (`buyerroom.tsx`, `buyerroom.css`): **not the app shell.** Keep
  the rail-less route and the credential handling exactly; restyle the page
  to the lit ground with the one glow, a 720px `buyer-column`, the eyebrow,
  the display-face title, `buyer.contact` in every state, the welcome, the
  board with Download per document, the composers only for `comment`, the
  foot with Sign out, the fixed "Powered by Margince" mark (`Wordmark`).
  The four states keep their copy (`buyer.deadTitle` + the link-request
  form, `buyer.pausedTitle`, `buyer.expiredTitle`, `buyer.closedNote`) and the
  preview banner. `buyer-column > * { flex: none }` stays (the page grows,
  panels do not compress). Stories: `buyerroom.stories.tsx` for every state at
  1024px and 390px; `buyerroom.test.tsx` unchanged in behaviour.

## 8a. Base responsiveness

The product already folds at three widths (`pagezones.css` at 1200px and
720px, the phone bar and the 390px axe sweep in `shell.css` and
`ac.spec.ts`). The restyle keeps those breakpoints and gives every record
page one ladder, in Step 3 (`composed.css`, `pagezones.css`, `statstrip.css`)
and Step 4 (`shell.css`):

| Width | What changes |
|---|---|
| ≥ 1400px | The full layout: five readings, the 360, two columns 3fr/2fr, the details panel opens beside the reading at 300px. |
| 1200–1400px | Readings keep five across at 22px figures; the details panel, when open, takes the readings to 21px and the columns stay two. |
| 720–1200px | Readings wrap to three then two per row; the two columns stack into one; the details panel opens as a **drawer over the page** (the `Modal placement="right"` surface), never squeezing the reading; the tab strip scrolls sideways inside its own box (`TableScroll` discipline: the page never scrolls sideways). |
| < 720px | The head stacks: mark and name, then facts, then the verbs as **primary + more** only (the others move into `more`); readings two per row; the spine scrolls sideways in its own box (`.co-spine-scroll` already does); the map scrolls (`.rmap-scroll` already does); the rail becomes the phone bar. |
| 390px | The existing sweep: no horizontal scroll, 44px targets, `--stickyBottomInset`. |

Rules: widths in `rem`/`fr`/`minmax(0, …)`, never fixed px on a column
except the details panel and the map; every table, spine, strip and map
inside its own `overflow-x: auto` box; `min-width: 0` on every grid child;
the head's `h1` may wrap, the readings' figures may not. Every state story
in §3.4 gets a 390px and a 1024px viewport variant in Storybook, and
`ac.spec.ts`'s 390px sweep runs on every record page in PR 5–8.

## 8b. Records with more, and records with less

The mock draws one well-fed company. The product opens records with nothing
on file and records with forty deals, and the layout must hold at both ends
without a second layout. The rule, per pane, in Step 3 and the record PRs:

**A pane exists only when it has something to say.** A zone with no rows
renders as one line in its place (its `emptyLabel`: "No deals yet ·
New deal"), not as an empty box and not as nothing — the reader must still
find the door. A zone the reader may not see renders the withheld sentence
in that line. The order of zones never changes with content, so a rep's hand
learns one page.

**The thin record.**

- Company with nothing on file (`nothingOnFile(view)`): the readings row
  shows the slots that can be read and states the rest in words
  ("Not assessed · 0 of 3 rated", "Nothing billed"); the deep-read card
  **leads the column in the 360's place** and the 360 pane is not drawn
  until a read exists; What needs you holds the one row "Read their site"
  or nothing; About is the empty dossier line with "Write it"; People is
  "Add a contact"; the details panel opens with the fields to fill.
- Contact thin (`ThinState`): the identity, the readings that exist (a
  strip slot that cannot be read is "Not shown" with no tone), the consent
  section (drawn on every contact), the enriched-fields surface if anything
  was read, and the Research tab's never-run mark. No 360 word: the moment
  pane says nothing has been captured yet.
- Deal just created: the facts line, the stepper on its first stage, the
  readings with the newest offer absent, the 360 pane absent until a
  status exists (`deal360.unreadable` and "no story yet" are different
  sentences), the seats panel with "Add stakeholder".
- Lead from a form: the ladder at New, first response counting, the score
  with `scoreNoSignals`, the preview sentence for qualifying.
- Project just born: phase Scope, rollups empty (`project.rollups.empty`),
  one "Add a deal" line.

**The rich record.**

- Lists on the overview are **capped at three rows with "N more"** into the
  tab (`RAIL_ROW_LIMIT` holds); the tab carries the full table with paging.
- The readings stay five; a sixth fact is a detail, not a card.
- The 360 sentence is at most three claims; the spine at most six stops
  plus the marker; the folded thread at most five; the rest is History.
- What needs you shows the lead row plus at most three, then "N more" into
  the Worklist filtered to this record.
- People as chips: three named plus "+N"; Stakeholders the same.
- The page cap (`page.has_more`, `co.commercial.truncated`,
  `project.deals.more`) keeps its sentences; a count on a truncated page
  reads "at least N", never N.
- A long name wraps in the head (`text-wrap: balance`); a long facts line
  wraps into rows; the verbs never wrap onto the name.

**Storybook carries both ends.** Every record's `Records/…` stories get a
`Thin` and a `Rich` variant beside the state variants in §3.4, at 1024px and
390px, and the axe suite runs on both.

## 9. Then the rest

Worklist, Reports (with Explain), Ask Margince, Filters & views, Settings
(the second level), Deal Room, the ⌘K palette (`palette.tsx`), compose
(`compose.tsx`, 3.5k lines: token and drawer pass only), Home/Brief,
Pipeline board (`PipelineBoard`, `DealCard`), the list pages (`ListTable`),
onboarding, booking, the buyer room, imports, privacy settings. Each is a
token-and-pane pass over an existing screen, in the order the rail lists
them.

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

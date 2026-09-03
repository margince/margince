# Adopt the design: the record pages

Sections §3–§7 of the plan in [adopt-the-design.md](adopt-the-design.md):
the five record pages, each with every state it already handles. Read the
gates table and the design-system steps there first; the sub pages, the
Deal Room and the responsive ladder (§8–§9) are in
[adopt-the-design-surfaces.md](adopt-the-design-surfaces.md).

## 3. Company — the reference implementation

The company page goes first because it has the most zones, the most states,
and the tightest e2e. Every later page copies its decisions. Files:
`organizations.tsx` (over the cap: new zones go in new files under
`screens/company/`), `company360.tsx`, `companyheader.tsx`, `companyrail*.tsx`,
`companytoday.tsx`, `companywork.tsx`, `companyrecent.tsx`,
`companycommercial.tsx`, `companydossier.tsx`, `companypeople/`.

### 3.1 Head

- Verbs, unchanged in substance: **Write email · Log activity · Add task ·
  more**, all outlined (`Button` ghost); the page's one primary is inside
  What needs you. The `more` menu keeps its seven items in order (Edit,
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

- **Head.** Verbs: **Write email · Call · Add task · more**, all outlined.
  Write email keeps `primaryTransportAction`'s routing and refusal
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

- **Head.** Verbs: **Write email (from `DealEmailAside`) · Log activity ·
  Edit deal · more**, all outlined (Archive, Share, Reopen when won/lost). The
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
- **Readings.** `LeadStrip` keeps the product's five slots: score (with
  override badge / top factor / `scoreNoSignals`), status, source, company,
  first response (SLA line, only when the target is on). "Next" (the booked
  meeting) and "Your move" are added only once the lead's 360 carries them
  (§11). Every slot states its absence in words.
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

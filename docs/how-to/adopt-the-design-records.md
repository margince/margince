<!-- prose:plain -->
# Adopt the design: the record pages

Sections 3–7 of the plan in [adopt-the-design.md](adopt-the-design.md): the five record pages, each with every
state it already handles. Read the gates table and the design-system steps there first. The sub pages, the Deal
Room and the width ladder (sections 8–9) are in [adopt-the-design-surfaces.md](adopt-the-design-surfaces.md).

## 3. Company: the reference page

The company page goes first, because it has the most zones, the most states and the most `e2e` checks. Every
later page copies what it decides. Files: `companies.tsx` (over the limit: new zones go in new files under
`screens/company/`), `company360.tsx`, `companyheader.tsx`, `companyrail*.tsx`, `companytoday.tsx`,
`companywork.tsx`, `companyrecent.tsx`, `companycommercial.tsx`, `companydossier.tsx`, `companycontacts/`.

### 3.1 Head

- Verbs, as they are today: **Write email · Log activity · Add task · more**, all outlined (`Button`
  ghost). The page's one primary button is inside What needs you. The `more` menu keeps its six entries in order
  (Merge, Partner set-up, Share, Full history, Decisions, Archive), with the refusal caption first. The
  `PageAsideToggle` leaves `actions` for the `trailing` slot of the tab strip.
- Facts line: domain · industry · size · owner · **way in** · last contact. Restore the way in: the contact
  the relationship runs through, as the `WithWayIn` story in `companyheader.stories.tsx` draws it. The lifecycle
  control stays a control on the name line (`CompanyLifecycleControl`), with each relationship badge beside it.
- The live dot: `"In conversation"` from the pulse of the 360. It is absent when the pulse section is withheld.
- Archived: `record.archivedReadOnly` once, before the verbs. The create verbs are removed.

### 3.2 Zones, in order, with their states

| Zone (mock) | Built from | Data | States it must keep |
|---|---|---|---|
| Readings row (5) | `StateStrip` → `StatStrip` of `PipelineCard`, `MoneyStat`, chat state (from `pulse`), last touch, next (`view.next`). **Each card carries an evidence chip**: `EvidenceMark` with the hover popover, which opens `EvidenceModal` on click. The modal shows the rows added up, the read date and the connection. **Each card also links to its tab** (`companyTabRoute`); the chip stops the click | `state_strip`, `useFinanceSummary`, each citation already on the strip | `co.strip.notAssessed`; finance `notACustomer / noConnection / unmapped / syncing / withheld / staleFigure / errorFigure / nothingBilled / error / loading`; `co.strip.unpriced`, `pricedPartly`, `convertedAsOf`; `co.section.restricted`. A slot that cannot be read is absent or says so, never €0 or "—" without a reason |
| The 360 | `TodayOnThisAccount` + `VerdictHead` (`HealthStat`/`AccountHealthStat` as the word and the measures) + `RecordSpine` + the thread from `useChronologySlots`, folded behind `"Read the thread · N"` | 360 sections, chronology | `today.quiet` (with nothing to say, it draws a 360 with nothing to report, not an empty pane); `today.failed`; `co.section.unavailable`. Withheld sections drop their row and say so once. `since_last_visit` with a withheld baseline never becomes a claim. `"Write it again"` shows only when the reader may use it |
| Deep-read offer | `DeepReadPanel` **leads the column** in place of the 360 when `nothingOnFile(view)` | | the two scan phases; `SiteReadPanel` pages read or skipped, and why; 422 no site; 501 seam not wired; `SiteReadDeferral` |
| What needs you | one list. The moment is the lead row. Then the `co.suggest.*` rows (`draftReply / openDeal / addTask` verbs; `add_task` has no surface, so it can only be dismissed). Then tasks from the source of `CompanyTasksTab`, and the next meeting (`onPrepareMeeting`) | 360 suggestions, tasks, meetings | `co.next.empty`; `co.suggest.more` on the limit. A withheld suggestion removes the row, not the verb. The `DecisionsChip` count stays in the menu |
| Commercial (the money) | One pane, the `DESIGN.md` zone. First `CompanyContractState` and won or lost on one line. Then the open deals of `CompanyWorkCard`, with their status line (`workVerbs`). Then the project. `CompanyLastOffer` and the full table live on the Deals tab; the details panel shows the contract line too | `view.deals`, `view.projects` | `contracts.state.none`; `co.work.statusesWithheld`; `co.work.countAtLeast`; `leadingDeal` refuses to pick on a page with more rows than it shows, or with more than one currency |
| Ask (prepared questions) | `AssistantPanel` as full-width rows, with no free field. The three questions are the ones the server answers (`CompanyQuestion`), so there are no new keys. One more question is a server change first | | `co.ask.nothing` |
| About | `DossierPanel` lead, body text and sources; `SignalsSection` rows; the `GrowthFitPanel` verdict row (only when `!hasWorkInFlight`); a `"Profile"` link | own reads | `co.dossier.empty` (write it), `co.dossier.stale` `"Read over a month ago"`, `co.dossier.unavailable`; `co.factSuspect.*` with its evidence |
| Contacts (chips) | `ContactsSection` (`RAIL_ROW_LIMIT`) as chips with `"+N"` | 360 | withheld → absent, with the sentence |
| Details (right, closed) | `CompanyRail` as one pane with five titled sections (keep the layout of five subjects inside the pane): a Details grid with inline edit, the top 3 deals, the top 3 contacts, Hold, Tags | | every rail state as today; absent while a composer is open |
| Chronology zone | stays the timeline slot of `RecordView`, on the History tab only (the 360 carries the folded thread) | | `timelineNotice`, `chronologyNotice` |

States at the header level: `co.partial` on `view.isError`. A 403 keeps the one shared sentence. Each banner for
version skew stays as it is.

### 3.3 Features to carry (checklist)

In the mock: the 360 with sources, the spine, the folded thread, and the needs list with suggestions and tasks.
Also deals, about with sources and how old it is, contacts, details, tags, and the history rail. Then the Contacts map,
Profile, Finance, Documents, Partner, `⌘K`, and compose.

**Built in this step** (not in the mock):

- evidence (`EvidenceModal` with steps) behind every source chip
- facts that disagree (`co.factSuspect`)
- the deep-read card and the site-read panel
- the full-history modal with restore
- the decisions panel from the menu
- the hold row for the other party
- the VAT mark
- custom fields in their own Details section
- document extraction staging on the Documents tab (three states)
- the meeting brief drawer
- the rollup of the company tree, with the FX 422
- the source line (`captured_by`, `"agent: deepread"`), restored under the facts
- the `since-last-visit` note, with its wait of 5 seconds

### 3.4 Tests

Write the shape checks in `company-record.spec.ts` again, in the same commit. That is readings under the tabs, the
`"one Company 360 pane"` first, and the details panel on the right, closed, as one pane of named sections. Update
`company360.test.tsx`, `companyheader.test.tsx` and `companyrail.test.tsx` for markup, not for what they do.
`history.spec.ts` stays as it is. Storybook: the `Records/Company 360/` stories for every state row in section
3.2. This is where "empty", "withheld", "never read" and "stale" each get a story.

## 4. Contact

Files: `contactpage.tsx`, `contact360.ts`, `contactrail.tsx`, `contacttoday.tsx`, `contactcards.tsx`,
`contactmemory.tsx`, `contactcorrections.tsx`, `contactnetwork/`.

- **Head.** Verbs: **Write email · Call · Add task · more**, all outlined.
  Write email keeps the routing and refusal of `primaryTransportAction`.
  `writeRefusal` checks reach first, then consent; a guard with no answer refuses nothing.
  But its label is the base word, unless the transport is the only one.
  Call and Meetings stay as icon verbs, or move into `more`.

  **Add to `more` what is missing today**: Archive, Share, Research, Full history. Facts: title · employer ·
  email · phone · the way in (from the lead route of the network).
- **Readings.** `ContactStrip`: whose move, open promises (`contact.loops`), the deal they decide on, the next
  meeting, answers in. `record.notShown` with no tone on a withheld slot. `noOpenDeal` and `noMeeting` are
  answers, not withheld values.
- **The 360.** `ContactToday` is the word and the sentence: the moment, with `readiness()` reasons and how
  new it is. `RecordSpine` comes from the activities of the 360, and the thread is folded from the chronology
  of the timeline tab. A moment with nothing to say draws through the same pane (`isQuiet`).
- **What needs you.** The moment's actions (`readiness()` on the button, and the reason when it is blocked), the
  `ContactCommitmentsCard` rows, open loops, the next task. `runContactMomentAction` still opens nothing for an
  action with no route.
- **The deal they decide on.** `ContactCommercialCard`, with the Deal Room as chips.
- **Ask.** Prepared questions (`contact.ask.q.*`).
- **Knowing them.** `ContactBriefCard` and `ContactMattersCard` as the `Priorities`, `Objections` and `Success`
  rows, with `Absent()` kept. Sources, and `"Correct something"` → `EnrichedFields`.
- **Their network.** `WhoKnows`, `Employers`, the lead route.
- **Details (right, closed).** `ContactRail` stays **one pane with hairline parts** (its written layout), plus
  `ContactEmailPanel` under it.
- **States to keep:** `contact.page.loading` becomes a skeleton (the one page without one). Also keep
  `contact.page.notOpened`, and read `withheldSections` once. Keep the consent verdict from the server key, and
  the `provider.profile.neverRun` mark on the Research tab (a run that was stopped reads as never run). Keep the
  `contact.graph.*` notes that the map has gaps, and the verbs removed when archived.
- **Tabs.** As they are (`contacttab.ts`). Network keeps its order: decision strip, lead panel, routes, then the
  map (`contact-network.spec.ts` checks it).
- **Carry:** consent and channels (both places), the hold section, relink, and the research drawer. Also the
  meeting brief with its four refusals, intro requests and relay steps, and the composer intent wiring.

## 5. Deal

Files: `deals.tsx` (over the limit: the page moves to `screens/deal/`), `deal360/*`, `dealstatus.tsx`,
`dealroom.tsx`, `dealfiles.tsx`.

- **Head.** Verbs: **Write email (from `DealEmailAside`) · Log activity · more**, all outlined (Archive, Share,
  and `Reopen` when won or lost). The `controls` slot goes. Value, stage, owner, close, forecast and partner join
  the facts line. Fields that are masked are still **named** (`FieldGuard mode="masked"`). `dealPulse` becomes the live dot
  (`"Your move"`).
- **Readings.** `DealStrip`, plus the stage with its days here. The strip holds money with the newest offer,
  close with provisional or waiting, contacts with the withheld mark, and momentum.
- **The 360.** `DealStatusCardPanel` (Deal360) is the word, the sentence and each citation. The stepper
  (`fieldset.stepper`, a group, not a menu) sits inside the pane above the spine, and the ledger is folded.
  `deal360.unreadable` shows when the story is not in the promised shape.
- **What needs you.** The next move of Deal360 as the lead row, `DealApprovals` as staged rows (dashed), and the
  reply still to send, from `useWaitingReply`.
- **The buying committee.** `DealContactsPanels` rows beside `DealCommitteeMap` (ghost seats stay).
- **Ask.** Prepared questions (`deal.ask.q.*`).
- **What this deal is.** The story of Deal360, with `"What is holding this up"` and `"What the buyer wants"`.
  An absent section draws nothing.
- **Offers.** `OffersPanel`. **Deal Room.** The `DealRoomAside` card (`OpenRoomCard` when there is none).
- **Details (right, closed).** `DealSeats`, `FxLine`, wait-until, forecast, custom fields, project link, partner
  attribution, files (top two).
- **Tabs.** `overview · history · documents`, **moved into the URL** to match the other records
  (`history.spec.ts`).
- **States to keep:** a provisional close, and masked fields. A standing the app does not know is never
  `healthy`. An `unmapped` forecast draws as it is, and archived shows once in the band. Closed and archived have different
  refusals. Keep the caption when a move to the next stage fails, one move at a time, and the version
  check on edit.
- **Carry:** `StartDeliveryPrompt`, the dialog for the won reason, and file hide (with undo) apart from delete.
  Also writing the briefing again, and the project link through project write rights.

## 6. Lead

Files: `leads.tsx` (over the limit: the record moves to `screens/lead/`), `leads.stepper.tsx`,
`leadsignals.tsx`, `leadvocab.tsx`.

- **Head.** Verbs: **Qualify (primary, `lead.promote` with `promoteIneligible` on the control) · Write email ·
  Disqualify · more (Share)**. The `"Lead"` mark stays on the name line. Facts: title · company (text, no
  record) · email · source and date · owner.
- **Readings.** `LeadStrip` keeps the five readings of the product. They are score (with the override badge,
  the top reason or `scoreNoSignals`), status, source, company, and first response. The first response shows the SLA
  line, only when the target is on. `"Next"` (the next meeting on the calendar) and `"Your move"` are added only once the
  lead's 360 carries them (section 11). Every slot states in words when it is absent.
- **The 360.** Put together, not written. The standing word comes from `statusReading`, and a sentence from the
  ladder explanation (`ladderExplanation`: who moved it, and what it read). Then the ladder (`LeadStepper`), the
  spine from the timeline, and the folded thread. The last line says
  `"Assembled from your records · a lead carries no suggestions"`.
- **What needs you.** `"Ready to qualify"` when `promoteEligible` (the `PreviewSentence` as its reason,
  `previewMergeWithheld` included). Then the reply still to send (`RecordEmailAside detectWaitingReply`), and tasks.
- **The score.** `LeadScorePanel` moves up from the rail list to a pane: the parts of the score,
  `ScoreShortfall`, override, `LeadManualSignals`.
- **If the lead is qualified.** The preview sentence and `PromotedLeadPanel` after the fact. The page stays, and the
  `lead.qualify.done` toast shows with the links.
- **Ask.** Prepared questions (`lead.ask.q.*`).
- **Details (right, closed).** `LeadIdentityFields`, `LeadOwner`, custom fields.
- **States to keep:** the closing sentence that names **which** close (`terminalPromoted` or
  `terminalDisqualified`), with one id that every refused control points at. Keep the callout in the band when a
  write fails, and the promotion result `unknown/pending/failed`. A qualify value with no currency waits. A
  disqualify reason is needed. One write at a time.

## 7. Project

Files: `project360.tsx`, `projectsections.tsx`, `projectreadings.tsx`, `projectphase.tsx`,
`projectcompanies.tsx`.

- **Head.** Verbs: **Edit project · more**. The `more` menu holds Archive (with the built id and `If-Match`),
  `Assign owner` and Share.
  Log activity only if `LogActivity` gets a `project` record type; otherwise it is not a base verb here. Facts:
  phase · client · owner · key · go-live.
- **Readings.** `RollupsStrip` as five cards under `SurfaceState`: open, won, `commitments`, last activity (with
  `rollups.never`), and the activity count.
- **The 360.** Put together. The phase word, the read-only sentence when it applies, `PhaseStepper`, the spine
  from the chronology, and the folded thread. No verdict word is written for a project today; the pane says
  `"Assembled from your records"`.
- **What needs you.** `CommitmentsCard` with the `overdue` badge, and a block row when a contract or document is
  missing.
- **Deals.** `ProjectDealsCard` with `NewDealAction` (project write rights). **Companies.** `ProjectCompanies`
  by role. **Stakeholders.** `StakeholdersCard` as chips.
- **Ask.** Prepared questions (`project.ask.q.*`).
- **Details (right, closed).** Contracts, documents, phase history.
- **States to keep:** all 9 through `SectionPanel`; `documents` never in `sections_omitted`. The one
  read-only sentence that the band makes. The page limit `project.deals.more`, and a phase word the app does not know.
- **No tabs** today, and none added.

## Editing core records

Company, contact, deal and lead fields are edited in the Details pane. There is no Edit dialog for the whole
record. Fields with no value still show. Custom fields form their own section, from the workspace catalog in
use, even when no values exist.

Text fields, and fields where you pick one value, save in line; Escape stops the edit. Text of many lines saves
on `Cmd/Ctrl+Enter`, or when you leave the field. Some changes use a local `Save` and `Cancel`, so the parts that
go together are saved together. Those are
address parts, email, phone and domain lists, deal money, company and project links, and partner attribution.
A failed save keeps the draft.

Writes carry the version read when the edit started, then read the record and what is built from it again. An
open edit blocks closing Details, and warns when you leave the page.

Qualifying leads, changing the lifecycle, setting a score by hand, closing deals, merging, archiving and consent
changes keep their own controls and needed evidence. Fields the app works out, masked and archived fields, and
fields you have no rights to stay read-only. Header buttons use the same record writer as Details. Dialogs for a new record stay.

`RecordFields` composes the inline controls of the design system and `RecordFormBody`. Each object gives its
field list that exists, and a small request mapper. `RecordCustomFields` gives the section built from the
catalog for all four kinds.

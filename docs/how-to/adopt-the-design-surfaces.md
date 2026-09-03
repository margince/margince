# Adopt the design: sub pages, the Deal Room and the rest

Sections §8–§9 of the plan in [adopt-the-design.md](adopt-the-design.md):
the sub pages and maps, both sides of the Deal Room, base responsiveness,
records with more and with less, and the remaining screens. The record
pages they follow (§3–§7) are in
[adopt-the-design-records.md](adopt-the-design-records.md).

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
  link dialog with mailed/copied), what is shared, the lifecycle, and — new,
  not on main today — **the deal behind this room**: the deal's standing word,
  stage, value, whose move, committee coverage and next meeting, read from
  the deal's own `useDealStatusCard`/`useDealCoverage` and drawn seller-only
  (the buyer projection never carries it). Every
  `room.*` and `roompage.*` key stays; `FINISHED_STATES` and `refusalFor` keep
  removing the composer and the verbs.
- **Buyer** (`buyerroom.tsx`, `buyerroom.css`): **not the app shell.** Keep
  the rail-less route and the credential handling exactly; rebuild the page
  as the small site in `DESIGN.md`: a 1040px frame on the lit ground, the
  hero (mark, "Prepared for … by …", the live pill from `expires_at`, title,
  welcome), the documents as a gallery of tiles (a page preview needs a
  first-page thumbnail from the file store — new; until it exists the tile
  draws the type mark on `--bg3`), the threads in place, the contact card,
  the foot, the mark (`Wordmark`). Three of its pieces are **additions to
  the contract**, one PR each after the restyle: "New since your last
  visit" (derive from the participant's `last_seen_at` against document and
  thread timestamps the view already carries — no schema change), **next
  steps written by the contact** (a `next_steps` list on the room beside
  `welcome_message`, edited on the seller page under Title and welcome), and
  **"Book a call"** (a link to the steward's `#/book/<hostSlug>` page when
  the steward has one; hidden otherwise). The composers stay only for
  `comment`; a seller preview stays read-only.
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

<!-- prose:plain -->
# Adopt the design: sub pages, the Deal Room and the rest

Sections 8–9 of the plan in [adopt-the-design.md](adopt-the-design.md). They cover the sub pages and maps, both
sides of the Deal Room, and how pages change with the width. They also cover records with more and with less on file,
and the other screens. The record pages they follow (sections 3–7) are in
[adopt-the-design-records.md](adopt-the-design-records.md).

## 8. Sub pages and maps

One pull request per record, after its overview lands:

- **History** on all four: the rail style from Step 3, with the filter strip above (`ChronologyFilter`,
  `TimelineFilterBar`). Undo rows (`historyreversalrow.tsx`) show as changes with an empty dot, with
  Restore.
- **Company Contacts**: `CoverageBand`, then the committee map (`mapModelFromCoverage`), then the list of
  contacts. `IntroRequestModal` and `coverageexplorer.tsx` sit behind `"Where are we thin"`.
- **Contact Network**: the order is pinned by `e2e`; the map's panel gets the `EdgeDetail` write.
- **Deal Documents / Files**, **Company Documents** with extraction staging, **Finance**, **Profile** with
  `ReferenceDisclosures`, **Partner**, **Meetings**, **Research** (never-run mark), **Data & tools**.

## 8c. The Deal Room, both sides

Two files, one board. `dealroomthreads.tsx` draws for both sides and takes the verbs as callbacks; that stays.

- **Seller** (`dealroompage.tsx`, `dealroomaccess.tsx`, `dealroomdocuments.tsx`,
  `dealroomconversation.tsx`). It uses the record page shell from Step 3. The head has `"Back to the deal"`, the
  facts line, and `View as buyer · Room access · more`. The state banner sits as the `band2` row. Title and
  welcome sit as a pane, with an Edit verb over `RoomText`.

  The board shows the four `DOCUMENT_GROUPS` as eyebrow labels, and a `dcard` per document, with threads for
  the whole room below. On the right sits `DealRoomAccess`. It has rows with capability, state, `last seen` and
  downloads, and the row menu: give a new link, change capability, revoke. It also has the link dialog that
  shows once, with mailed or copied. Then what is shared, and the lifecycle.

  New, and not on main today: **the deal behind this room**. It shows the deal's standing word, stage, value,
  whose move, committee coverage and next meeting. It reads them from the deal's own `useDealStatusCard` and
  `useDealCoverage`. Only the seller sees it (the buyer view never carries it). Every `room.*` and `roompage.*`
  key stays. `FINISHED_STATES` and `refusalFor` still remove the composer and the verbs.
- **Buyer** (`buyerroom.tsx`, `buyerroom.css`): outside the app shell. Keep the route with no rail, and keep the
  handling of the key as it is. Build the page again as the small site in `DESIGN.md`. That is a 1040px column
  on the light ground, and the top block: the mark, `"Prepared for … by …"`, the live pill from `expires_at`,
  title and welcome.

  Then the documents, each as a tile. A page preview needs a new image of the first page from the file store.
  Until that exists, the tile draws the type mark on `--bg3`. Then the threads in place, the contact card, the
  last block, and the mark (`Wordmark`). Three of its parts are **new parts of the contract**, one pull request each
  after the new style:
  - `"New since your last visit"`: derived from the member's `last_seen_at`, against the document and thread
    times the view already carries, with no schema change.
  - **Next steps written by the seller**: a `next_steps` list on the room beside `welcome_message`, edited on
    the seller page under Title and welcome.
  - **`"Book a call"`**: a link to the `#/book/<hostSlug>` page of the owner of the room.
    If the owner has none, the link does not show. The composers stay only for `comment`, and a seller preview stays read-only.

  The four states keep their text (`buyer.deadTitle` and the link request form, `buyer.pausedTitle`,
  `buyer.expiredTitle`, `buyer.closedNote`) and the preview banner. `buyer-column > * { flex: none }` stays (the
  page gets longer; panels keep their size). Stories: `buyerroom.stories.tsx` for every state at 1024px and 390px.
  `buyerroom.test.tsx` does the same as it does today.

## 8a. Base layout for each width

The product already folds at three widths. `pagezones.css` folds at 1200px and 720px. The phone bar and the
390px axe sweep live in `shell.css` and `ac.spec.ts`. The new style keeps those break points and gives every
record page one ladder. That work is in Step 3 (`composed.css`, `pagezones.css`, `statstrip.css`) and Step 4
(`shell.css`):

| Width | What changes |
|---|---|
| Above 1400px | The full layout: five readings, the 360, two columns `3fr/2fr`. The details panel opens beside the reading at 300px. |
| 1200–1400px (both ends included) | Readings keep five across at 22px numbers. When the details panel is open, it takes the readings to 21px, and the columns stay two. |
| 720–1200px | Readings wrap to three, then two per row, and the two columns stack into one. The details panel opens as a **drawer over the page** (`Modal intent="drawer"`, 560px), and never makes the reading smaller. The tab strip scrolls to the side inside its own box (`TableScroll` rule: the page never scrolls to the side). |
| < 720px | The head stacks: mark and name, then facts, then the verbs as **the first base verb + more** only. The head's first verb keeps its outline, and the rest move into `more`. Only a lead's Qualify is filled, as in every other place. Readings sit two per row. The spine scrolls to the side in its own box (`.co-spine-scroll` already does). The map scrolls (`.rmap-scroll` already does), and the rail becomes the phone bar. |
| 390px | The sweep that exists: no scroll to the side, 44px targets, `--stickyBottomInset`. |

Rules:

- widths in `rem`, `fr` or `minmax(0, …)`; never a fixed `px` width on a column, but for the details panel and
  the map;
- every table, spine, strip and map inside its own `overflow-x: auto` box;
- `min-width: 0` on every grid child;
- the head's `h1` may wrap; the numbers of the readings may not.

Every state story in section 3.4 gets a 390px and a 1024px viewport form in Storybook. The 390px sweep of
`ac.spec.ts` runs on every record page in pull requests 5–8.

## 8b. Records with more, and records with less

The mock draws one company with much on file. The product opens records with nothing on file, and records with
40 deals. The layout must hold at both ends without a second layout. The rule, per pane, in Step 3 and the
record pull requests:

**A pane draws only with content.** A zone with no rows draws as one line in its place (its `emptyLabel`:
`"No deals yet · New deal"`). The reader can still find the way in, and the zone never draws an empty box or
goes away. A zone the reader may not see draws the withheld sentence in that line. The order of zones never
changes with content, so a rep's hand learns one page.

**The record with no data yet.**

- A company with nothing on file (`nothingOnFile(view)`). The readings row shows each slot that can be read,
  and states the rest in words (`"Not assessed · 0 of 3 rated"`, `"Nothing billed"`). The deep-read card
  **leads the column in place of the 360**, and the 360 pane does not draw until a read exists. What needs you
  holds the one row `"Read their site"`, or nothing. About is the empty dossier line with `"Write it"`;
  Contacts is `"Add a contact"`; the details panel opens with the fields to fill.
- A contact with no data yet: the identity and the readings that exist. A strip slot that cannot be read is
  `"Not shown"` with no tone. Then the consent section (on every contact), and the field surface the AI
  filled if it read anything. Then the never-run mark of the Research tab. No 360 word: the moment pane says
  nothing is captured yet.
- A deal that is new: the facts line, the stepper on its first stage, and the readings with no newest offer.
  The 360 pane is absent until a status exists (`deal360.unreadable` and `"no story yet"` are different
  sentences), and the seats panel has `"Add stakeholder"`.
- A lead from a form: the ladder at New, first response counting, the score with `scoreNoSignals`, and the
  preview sentence for qualifying.
- A new project: phase Scope, an empty rollup (`project.rollups.empty`), and one `"Add a deal"` line.

**The record with much.**

- Lists on the overview **stop at three rows with `"N more"`**, which leads into the tab (`RAIL_ROW_LIMIT`
  holds). The tab carries the full table with pages.
- The readings stay five; one more fact is a detail, not a card.
- The 360 sentence makes at most three claims. The spine has at most six stops plus the marker, and the folded
  thread at most five. The rest is History.
- What needs you shows the lead row plus at most three. Then `"N more"` leads into the Worklist, filtered to
  this record.
- Contacts as chips: three named plus `"+N"`; Stakeholders the same.
- The page limit (`page.has_more`, `project.deals.more`) keeps its sentences. A count on a page with more rows than it
  shows reads `"at least N"`, never `N`.
- A long name wraps in the head (`text-wrap: balance`), and a long facts line wraps into rows. The verbs never
  wrap into the name line.

**Storybook carries both ends.** The `Records/…` stories of every record get a `Thin` and a `Rich` form, beside
the state forms in section 3.4. They come at 1024px and 390px, and the axe tests run on both.

## 9. Then the rest

The other screens: Worklist, Reports (with Explain), Ask Margince, Filters & views, and Settings (the second
level). Then the Deal Room, the `⌘K` palette (`palette.tsx`), and compose (`compose.tsx`, 3,500 lines: a token
and drawer pass only). Then Home/Brief, the Pipeline board (`PipelineBoard`, `DealCard`), the list pages
(`ListTable`), the first-run steps, booking, the buyer room, imports, and privacy settings. Each one is a pass of
tokens and the pane over a screen that exists, in the order the rail lists them.

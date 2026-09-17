import { useQueryClient } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { Badge, Button } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { OpenEmailDrawer } from "../design-system/openemaildrawer";
import { PageZones } from "../design-system/pagezones";
import { Panel } from "../design-system/panel";
import { SurfaceState } from "../design-system/surfacestate";
import { formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { type Translator, useLocale, useT } from "../i18n";
import { rosterOwnerNaming, useRoster } from "./entityref";
import { useOpenEmail } from "./openemail";
import { useWorklistAddress } from "./worklist.address";
import {
  bandSections,
  canReportEmptyBands,
  unbandedRows,
} from "./worklist.bands";
import { TeamBoard } from "./worklist.board";
import { sourceUnavailableText } from "./worklist.copy";
import {
  reviewShortfall,
  reviewWork,
  sellerWork,
} from "./worklist.destinations";
import { TeamExceptionsPanel } from "./worklist.exceptions";
import { HandledForYouPanel } from "./worklist.handled";
import { WORKLIST_FILTER_PARAM, WorklistHeader } from "./worklist.header";
import { HiddenBacklogPanel } from "./worklist.hidden";
import { CoachControl } from "./worklist.manager";
import { hasPane, WorklistPane } from "./worklist.pane";
import {
  loadedQueue,
  useRefreshWalk,
  useWorklist,
  type Worklist,
  type WorklistFilter,
  type WorklistItem,
  type WorklistScope,
  type WorklistWalk,
  worklistKey,
} from "./worklist.queries";
import { QueueBand } from "./worklist.queuebands";
import { WorklistReadings } from "./worklist.readings";
import { WorklistRow } from "./worklist.row";
import { WalkNotice } from "./worklist.walknotice";
import "./worklist.css";

// The Worklist: one ranked list, not fourteen lanes.
//
// The page it replaces was organized by producer, so a reader had to compare
// the position of one panel against another to work out that an item several
// screens down mattered more. This draws the order the server decided, and
// never re-sorts: the tie-breaks depend on a base-currency conversion and a
// materiality threshold the browser does not hold.
//
// Every row answers the five questions the surface exists for — what happened,
// why now, what is at stake, what to do, and can I do it here.

// The address dial, re-exported from where it lives. It belongs beside the
// strip that draws the lanes (`worklist.header.tsx`), and it is reachable from
// the SCREEN because another surface names one of this screen's lanes to open
// it — a reading in the Brief. One spelling of the parameter, two ways in.
export { WORKLIST_FILTER_PARAM };

// Which narrowing actually CONTAINS this group's members.
//
// Every group used to send the reader to `decisions`, which excludes system
// rows — so pressing Review on a broken automation filtered its own failures
// out of view and drew an empty page. A verb that hides what it promises to
// show is worse than no verb.
function reviewFilter(item: WorklistItem): WorklistFilter {
  return item.category === "system" ? "system" : "decisions";
}

// What identifies one row on this page.
//
// The SOURCE and the id together, because `id` alone is not unique across the
// queue: a task and a waiting message may carry the same underlying record's
// id, and the lanes mint ids independently. The React key has always spelled
// it this way; the selected-row state used the bare id, so two rows sharing one
// could both light up pressed while the pane resolved to whichever came first.
// One function now, read by both, so they cannot drift apart again.
function rowIdentity(item: WorklistItem): string {
  return `${item.source}-${item.id}`;
}

/**
 * The reader has put every row down.
 *
 * Distinct from `""`, which means they have chosen NOTHING yet — and the two
 * must not share a value. With nothing chosen the pane falls back to the first
 * row, so a "closed" spelled as `""` resolved straight back to row one and the
 * rank button became a control that did nothing when pressed.
 *
 * An identity no row can carry: `rowIdentity` joins a source and an id, and no
 * source is empty.
 */
const NOTHING_IN_HAND = "none";

/**
 * The row the pane is about.
 *
 * Resolved from the identity rather than held as the item itself: a refetch
 * replaces every row object, and a held one would go on describing a version of
 * the day that is no longer on screen.
 *
 * With nothing chosen this is the FIRST row, which is what makes the top of the
 * queue the focus without a card above it saying so. The queue was already the
 * ranking's answer to what matters most; a separate card repeated that row, a
 * second list repeated the three after it, and one morning was drawn three
 * times on one screen.
 *
 * The fallback is deliberately not written back into state. The reader chose
 * nothing, so there is nothing to remember: a refetch that reorders the day
 * moves this with it, where a stored id would pin the pane to whichever row
 * happened to be first when the page loaded.
 */
function rowInHand(
  queue: readonly WorklistItem[],
  selectedId: string,
): WorklistItem | undefined {
  const chosen = queue.find((item) => rowIdentity(item) === selectedId);
  if (chosen) {
    return chosen;
  }
  if (selectedId !== "") {
    return undefined;
  }
  // Only a row that HAS a pane is taken up by default. A row without one draws
  // no aside and offers no rank button, so marking it selected would leave the
  // accent stripe on a row the reader cannot open and cannot put down — a
  // highlight that means nothing and never clears.
  //
  // The FIRST such row rather than the first row: a day led by a deal still has
  // a contact further down whose context is worth standing open, and skipping to
  // it keeps the pane useful without moving the queue's own order.
  return queue.find(hasPane);
}

// Everything a row needs that is the same for every row on the page.
//
// One object rather than eight props threaded through each section, so the
// banded sections and the unbanded tail cannot drift into two spellings of the
// same wiring.
type RowContext = Readonly<{
  // Each row's place in the WHOLE queue, by identity.
  //
  // Built once for the page rather than searched per row: the sections partition
  // an accumulated list that grows with every "load more", so asking the queue
  // for each row's index would walk it once per row and cost more the further a
  // reader pages. It is keyed on the identity rather than the object because
  // that is what the rest of this file compares rows by.
  positions: ReadonlyMap<string, number>;
  owner: string;
  selectedId: string;
  onSelect: (next: string) => void;
  onOpenEmail: (activityId: string) => void;
  onFilter: (next: WorklistFilter) => void;
}>;

// A run of rows under one heading, or the unbanded tail.
//
// `position` is the row's place in the WHOLE queue, not in this run: the rank
// number is the page's promise about the order, and restarting it at each
// heading would print two number ones.
function QueueRows({
  items,
  positions,
  owner,
  selectedId,
  onSelect,
  onOpenEmail,
  onFilter,
}: RowContext & Readonly<{ items: readonly WorklistItem[] }>) {
  return (
    <ol className="worklist-list">
      {items.map((item) => (
        <li key={rowIdentity(item)}>
          <WorklistRow
            item={item}
            position={(positions.get(rowIdentity(item)) ?? 0) + 1}
            owner={owner}
            selected={selectedId === rowIdentity(item)}
            // Only where pressing it OPENS something. WorklistRow draws a plain
            // number without this, which is what the Brief already relies on: a
            // rank that toggles a pressed state and opens nothing teaches the
            // reader that the page lies about what is pressable.
            onSelect={
              hasPane(item)
                ? () =>
                    onSelect(
                      selectedId === rowIdentity(item)
                        ? NOTHING_IN_HAND
                        : rowIdentity(item),
                    )
                : undefined
            }
            onOpenEmail={onOpenEmail}
            onReview={() => onFilter(reviewFilter(item))}
            // The ORDER the Brief's card reads in, on every row of the queue:
            // the set-asides lead, the prepared move closes. The queue is a
            // list of rows a reader answers one at a time, which is the same
            // act the card is shaped for — and the two surfaces drawing one
            // row's verbs in two orders is what sent a rep looking for the
            // answer at a different x depending on where they opened it.
            //
            // The ORDER only. No card frames these rows, so they keep their
            // own captions, the way to their record, and the pin.
            acts="triage"
          />
        </li>
      ))}
    </ol>
  );
}

// The day, drawn.
//
// TWO kinds of number reach this component and they must not be confused.
// `day` is the first page: its summary, counts, reach and scope options
// describe the whole assembled day and do not move as the reader pages.
// `queue` is every row loaded so far, which grows. Reading rows off `day`
// would draw only the first page; reading figures off the latest page would
// describe a slice as though it were the day.
// Which "there is nothing here" sentence an empty queue earns.
//
// A partial read outranks the rest: a day cannot be reported clear while
// something that would have filled it was never read. Then the Tasks pill names
// its HORIZON — this queue is today's, so a task due tomorrow is deliberately
// absent, and "Nothing is waiting on you" read as "you have no work" to a rep
// looking at three open tasks on the company page beside it. The other pills
// keep the unqualified sentence: the full queue carries replies and reviews
// that have no deadline, so "due today" would be the wrong frame for it.
//
// WHOSE day is clear is the last question, and only the unqualified sentence
// gets it wrong: it is the one arm that says "on YOU", and on a colleague's
// queue that named the reader over somebody else's empty day. The other two
// describe the READ rather than the reader and stay as they are.
//
// The name is the roster's, and its absence falls back to the unqualified
// sentence rather than to an id or a gap: a reader who cannot be named is a
// question this line does not have to answer, and "Nothing is waiting on
// 4f3c…" is worse than a sentence one word too general.
function clearSentence(
  partial: boolean,
  filter: WorklistFilter,
  colleague: string | null,
  t: Translator,
): string {
  if (partial) {
    return t("worklist.clearOfWhatWasRead");
  }
  if (filter === "tasks") {
    return t("worklist.clearOfTasksToday");
  }
  return colleague
    ? t("worklist.clearFor", { name: colleague })
    : t("worklist.clear");
}

function WorklistBody({
  embedded = false,
  day,
  walk,
  onRefresh,
  queue,
  scope,
  filter,
  owner,
  selectedId,
  onScope,
  onFilter,
  onOwner,
  onSelect,
  onOpenEmail,
  hasMore,
  loadingMore,
  moreFailed,
  onMore,
}: Readonly<{
  embedded?: boolean;
  day: Worklist;
  queue: readonly WorklistItem[];
  scope: WorklistScope;
  filter: WorklistFilter;
  owner: string;
  selectedId: string;
  onScope: (next: WorklistScope) => void;
  onFilter: (next: WorklistFilter) => void;
  onOwner: (next: string) => void;
  onSelect: (next: string) => void;
  // Opens a waiting email into the page's one drawer.
  onOpenEmail: (activityId: string) => void;
  // What has moved since this walk started, from the LATEST page: the two
  // figures answer over the whole walk and are recomputed on every page, so
  // page one's copy is stale the moment a reader pages once.
  walk: WorklistWalk | undefined;
  onRefresh: () => void;
  hasMore: boolean;
  loadingMore: boolean;
  moreFailed: boolean;
  onMore: () => void;
}>) {
  const t = useT();
  const missing = day.sources_unavailable;
  // Whose day this is, by name. The same roster read the owner picker above
  // already makes, under the same key — so this resolves out of that cache and
  // opens no request of its own, and asks for nothing at all on the reader's
  // own day. `rosterOwnerNaming` answers null both for nobody and for an id
  // this roster cannot name, which is the one answer this sentence needs: it
  // has a wording that names no one.
  const colleague = rosterOwnerNaming(useRoster("user", owner !== ""))(owner);
  // Default context follows seller work. An explicit choice may also name a
  // review row, whose record context must remain reachable from the queue.
  const selected = rowInHand(
    selectedId === "" ? sellerWork(queue) : queue,
    selectedId,
  );
  // The day cut into the two jobs it holds. `destination` says which, and the
  // server decides it — the counts above the queue are computed from the same
  // field, so a split derived here from `source` or `category` would drift
  // from the figures it is drawn beside.
  const today = sellerWork(queue);
  const review = reviewWork(queue);
  // How much review work the day holds that this panel has not loaded.
  const reviewMissing = reviewShortfall(
    review.length,
    day.summary.buckets?.review,
  );

  const rowProps: RowContext = {
    // Numbered WITHIN the panel each row is drawn in, not across the day.
    //
    // Ranking over the whole queue is arithmetically honest and reads as a
    // fault: the split puts 1, 4, 7 in one panel and 2, 3, 5 in the other on
    // one screen, and a reader meeting a list that starts at 4 and skips 5 has
    // no way to know they are seeing a correct number rather than a broken one.
    // A rank says WHERE IN THIS LIST, which is the only question the number is
    // asked, and the day's own order is what put the rows in these lists.
    positions: new Map([
      ...sellerWork(queue).map((item, at) => [rowIdentity(item), at] as const),
      ...reviewWork(queue).map((item, at) => [rowIdentity(item), at] as const),
    ]),
    owner,
    // The RESOLVED selection, not the raw state. The pane falls back to the
    // first row when the reader has chosen nothing, and a highlight reading
    // the state alone would leave the pane describing a row the queue does not
    // mark — one screen disagreeing with itself about which row is in hand.
    selectedId: selected ? rowIdentity(selected) : "",
    onSelect,
    onOpenEmail,
    onFilter,
  };
  return (
    <>
      {/* TWO COLUMNS where the page has the width: the LANES — the day's
          figures, whose day it is, and the cuts through it — stand beside the
          day and stay put while it scrolls, so a reader ten rows down changes
          the cut without going back to the top. The columns wrapper is what
          the page's own width is measured against (a box cannot answer a
          query about itself); under the fold all three wrappers dissolve
          (worklist.css, `display: contents`) and the blocks take the one
          column in the order the phone rules give them. */}
      <div className="worklist-columns">
        <div className="worklist-lanes">
          <WorklistHeader
            day={day}
            loaded={queue.length}
            scope={scope}
            filter={filter}
            owner={owner}
            onScope={onScope}
            onFilter={onFilter}
            onOwner={onOwner}
          />
        </div>
        <div className="worklist-day">
          {/* What the day is WORTH. A SIBLING of the queue rather than part of the
          header, which is what lets the phone put the work first.

          On a wide screen it reads above the queue, where a figure describing
          the whole day belongs. On a phone the four cards stack into a 338px
          block, and with the title and controls above them the first row's
          verb landed 972px down an 844px screen — a rep opened their morning
          and had to scroll before they could do anything. The stylesheet moves
          this below the queue under 720px, in PAINT only — worklist.layout.ts
          holds why the document order does not follow it.

          NOT IN THE DRAWER. The drawer is opened from a page that already
          carries these five readings under its own work, so a second set of
          them is the same figures said twice on one screen — and folded away
          behind a disclosure they were a rule and a word of chrome standing
          between the head and the queue the drawer exists to show. */}
          {!embedded && <WorklistReadings day={day} onLane={onFilter} />}
          {/* THE REASON A LEAD OPENED SOMEBODY ELSE'S DAY, at the head of that day
          in every state — below it the block moved as its own form opened, and
          a lead read three panels of somebody else's morning before the way to
          say anything about it. Only on a named queue. */}
          {owner !== "" && <CoachControl owner={owner} name={colleague} />}
          {/* What has moved since the reader started paging. An offer to refresh
          rather than a fault: the day on screen is correct, it is simply no
          longer complete. */}
          <WalkNotice walk={walk} onRefresh={onRefresh} />
          {/* A day cannot read as clear while something that would have filled it
          was never read. This is the surface speaking about ITSELF, which is
          what Callout is for. */}
          {missing.length > 0 && (
            <Callout
              tone="warning"
              kind="standing"
              title={t("worklist.partialTitle")}
            >
              {t("worklist.partial", {
                sources: missing
                  .map((source) => sourceUnavailableText(source, t))
                  .join(", "),
              })}
            </Callout>
          )}
          {queue.length === 0 ? (
            // One line, not a panel. No card is drawn to report a zero.
            //
            // And ONE line rather than the four per-band ones below, which is a
            // deliberate difference. Those exist because a reader whose Now band is
            // empty cannot otherwise tell that from a page that simply starts lower
            // — a question only worth answering when there IS a page. A wholly
            // clear day has nothing to distinguish, and four headings each saying
            // nothing is under them says less than the sentence that says so once.
            //
            // Under the Tasks pill the sentence names its HORIZON. This queue is
            // today's: the server takes open tasks due before the installation's
            // midnight, so a task due tomorrow is deliberately absent. "Nothing is
            // waiting on you" read as "you have no work" to a rep who could see
            // three tasks on the company beside it, and the page offered nothing
            // to reconcile the two.
            <p className="t-body worklist-clear">
              {clearSentence(missing.length > 0, filter, colleague, t)}
            </p>
          ) : (
            // The queue, and beside it what the SELECTED row is about.
            //
            // PageZones is used only where there IS a pane. Its aside shape
            // reserves a 7fr/3fr grid whatever the aside contains, so wrapping an
            // unselected page would leave the queue at seventy per cent width with
            // an empty third beside it — a column that reads as a pane which
            // failed to load.
            //
            // hasPane is asked BEFORE the element is made: a component returning
            // null is still an element, and an element still gets the aside column
            // and its landmark. The rule lives beside the component that obeys it.
            <PageZonesWhenPaned
              active={selectedId !== ""}
              onClose={() => onSelect(NOTHING_IN_HAND)}
              pane={
                // NOT IN THE DRAWER, and this is the one surface it is
                // withheld from. The pane says whom a row is about, when they
                // last wrote and when we did — and the ROW now says all three
                // itself, on every surface that draws it. On the queue's own
                // page the column beside it is free and the pane adds the
                // record's own reading to that; in a drawer it is a third of
                // an already narrow list spent repeating the line above it.
                !embedded && selected && hasPane(selected) ? (
                  <WorklistPane item={selected} />
                ) : null
              }
              label={t("worklist.pane.title")}
              queue={
                <Panel
                  // INDIGO, the band the Brief's Focus panel wears and a
                  // record's "what needs you today" pane before it: these rows
                  // are the agent's reading of the day — what it ranked and
                  // what it prepared — and indigo is the one claim the product
                  // makes about who did that. The same panel is drawn on this
                  // page and in the queue drawer, so the claim is made once
                  // and reads the same in both.
                  tone="ai"
                  title={t("worklist.queue")}
                  titleAction={
                    <Badge tone="ai">{t("co.assistant.aiTag")}</Badge>
                  }
                >
                  {/* The headings come from the SERVER's band list, in its draw
                  order, rather than from the rows — which is the only way a
                  band holding nothing can say so. Ranks are still counted over
                  the whole queue, so a row's number is its place on the page
                  and not its place within its heading. */}
                  {bandSections(day, today).map((section) => (
                    <QueueBand
                      key={section.band}
                      section={section}
                      canReportEmpty={canReportEmptyBands(hasMore)}
                      rows={QueueRows}
                      rowProps={rowProps}
                    />
                  ))}
                  {/* Rows an older server sent with no band. Real work, drawn under
                  no heading rather than dropped to keep the sections tidy. */}
                  {unbandedRows(today).length > 0 && (
                    <QueueRows items={unbandedRows(today)} {...rowProps} />
                  )}
                  {/* The way to the rest of the backlog.
                  Acceptance asks that the queue's counts be reachable, and
                  before this the page stopped at its first read with no route
                  to the rows behind it — the figures said work existed and
                  offered no way to it. */}
                  {hasMore && (
                    <div className="worklist-more">
                      <Button onClick={onMore} pending={loadingMore}>
                        {t("worklist.more")}
                      </Button>
                      {/* A refused page leaves the button looking exactly as an
                      unpressed one does. Saying so is what tells the reader
                      the backlog is still there and worth asking for again. */}
                      {moreFailed && (
                        <span className="co-part-error" role="alert">
                          {t("worklist.more.failed")}
                        </span>
                      )}
                    </div>
                  )}
                </Panel>
              }
            />
          )}
          {/* What is NOT the seller's to execute, below their day rather than
          inside it.
          A duplicate pair, a stopped mailbox and an approval somebody owes are
          three different jobs, and none of them is the next call to make. Drawn
          in the queue they competed with it: a rep scanning for their next
          customer stepped over the product's own housekeeping to find one.
          Below, and never hidden — this work is somebody's, and a screen that
          swallowed it would be the reason it went undone. */}
          <ReviewPanel
            items={review}
            shortfall={reviewMissing}
            rows={rowProps}
          />
          {/* Team oversight belongs to the explicitly selected wider scope. */}
          {owner === "" &&
            scope !== "mine" &&
            scope !== "unassigned" &&
            day.scope_options.includes("team") && (
              <TeamExceptionsPanel
                enabled={day.scope_options.includes("team")}
                onOwner={onOwner}
              />
            )}
          {owner === "" &&
            scope !== "mine" &&
            scope !== "unassigned" &&
            day.scope_options.includes("team") && (
              <TeamBoard
                onOwner={onOwner}
                onUnassigned={() => onScope("unassigned")}
              />
            )}
          {/* This diagnostic counts all readable history, not personal obligations. */}
          {owner === "" && scope === "all" && (
            <HiddenBacklogPanel enabled={day.scope_options.includes("team")} />
          )}
          {/* LAST, and open. A reader opens this page to find what to do next;
          what is already finished answers a different question — worth having,
          and not worth leading with, which is what its position says. It is
          also the only panel here that asks for nothing: the receipt is why the
          acts above it are safe to take, and a receipt folded shut is one
          nobody checks. */}
          <HandledForYouPanel />
        </div>
      </div>
    </>
  );
}

// The work that is not the day's, drawn below it and never hidden.
//
// Its own component because WorklistBody had grown past what one function
// should hold: the split, the bands, the pane and the paging all read there,
// and a panel that also decides what to admit about itself is a sixth job.
function ReviewPanel({
  items,
  shortfall,
  rows,
}: Readonly<{
  items: readonly WorklistItem[];
  shortfall: { loaded: number; total: number } | null;
  rows: RowContext;
}>) {
  const t = useT();
  const { locale } = useLocale();
  if (items.length === 0) {
    return null;
  }
  return (
    <Panel
      title={t("worklist.review")}
      // What this panel is NOT showing, in the FOOTER band — which is what a
      // figure belonging to the whole panel gets. It was a bare `<p>` among the
      // panel's direct children, and a direct child of `Panel` sits in no
      // padding at all: the rows around it are `PanelRow`s carrying the panel's
      // own inset, so the sentence started a full `--padPanel` to the left of
      // every row above it and read as a line that had escaped the card.
      //
      // The panel has no cursor of its own — review rows arrive as a side
      // effect of paging the day — so a reader with an approval past the page
      // cut sees a panel that looks complete and nothing that says otherwise.
      // The day's own total is the denominator, never drawn bare: it counts
      // every candidate the read weighed, so alone it would claim rows this
      // panel does not hold.
      footer={
        shortfall
          ? t("worklist.review.partial", {
              loaded: formatNumber(shortfall.loaded, locale),
              total: formatNumber(shortfall.total, locale),
            })
          : undefined
      }
    >
      <QueueRows items={items} {...rows} />
    </Panel>
  );
}

// The queue, with a pane beside it only where there is one to draw.
//
// A shape decision rather than a component: PageZones reserves its aside
// column unconditionally, and a page with nothing selected should read exactly
// as it did before selection existed.
function PageZonesWhenPaned({
  queue,
  pane,
  label,
  active,
  onClose,
}: Readonly<{
  queue: ReactNode;
  pane: ReactNode;
  label: string;
  active: boolean;
  onClose: () => void;
}>) {
  const t = useT();
  if (!pane) {
    return <>{queue}</>;
  }
  return (
    <PageZones
      shape="aside"
      className={active ? "worklist-context-open" : "worklist-context-idle"}
      mainClassName="worklist-main"
      aside={
        <>
          <Button className="worklist-context-back" onClick={onClose}>
            {t("brief.queue.back")}
          </Button>
          {pane}
        </>
      }
      asideLabel={label}
      main={queue}
    />
  );
}

// The Worklist screen.
export function WorklistScreen({
  opensOn,
  embedded = false,
}: Readonly<{
  // What the address asked for, from `#/worklist/<segment>`: a user id opens
  // that contact's queue, and the literal "unassigned" opens the unowned pile.
  // Both are doors a team board row needs — a row that could only reach this
  // page would ask the reader to pick the same thing a second time.
  //
  // An unassigned path supplies the default scope; an explicit scope query
  // overrides it so the scope dial remains usable after following that link.
  opensOn?: string;
  embedded?: boolean;
}> = {}) {
  const t = useT();
  const {
    scope,
    filter,
    owner,
    selectedId,
    setScope,
    setFilter,
    setOwner,
    setSelectedId,
  } = useWorklistAddress(opensOn, embedded);
  const [openEmail, setOpenEmail] = useOpenEmail();
  const day = useWorklist(scope, filter, owner === "" ? undefined : owner);
  const refreshWalk = useRefreshWalk();
  const queryClient = useQueryClient();
  // A failed SHOW MORE is not a failed page. `isError` covers both, and
  // treating them alike would replace a screen of rows the reader is working
  // through with an error panel because one extra page did not arrive. The
  // rows already loaded are still true, so the surface stays ready and the
  // control below says the request failed.
  const lostTheDay = day.isError && day.data === undefined;
  const state = day.isPending ? "loading" : lostTheDay ? "failed" : "ready";
  // The FIRST page carries the day's own figures — summary, counts, reach,
  // scope options. Those describe the assembled day and do not change as the
  // reader pages, so they are read from page one rather than from the latest
  // page, whose numbers describe only its own slice.
  const first = day.data?.pages[0];
  // The WALK is the exception to the rule above, and the reason it is spelled
  // apart from it. `changed_since_snapshot` and `new_available` answer over the
  // whole walk and are recomputed on every page, so page one's copy is stale
  // the moment a reader pages once — the opposite of the day figures beside it,
  // which describe an assembly that does not move.
  const walk = day.data?.pages.at(-1)?.walk;
  // The rows, which DO grow. Deduped in the query, because a re-rank between
  // reads can serve one row on two pages.
  const queue = day.data ? loadedQueue(day.data.pages) : [];
  return (
    <div className="wrap worklist">
      {/* No label: the shell already heads this page, and a second "Worklist"
          would be announced twice and nest an h3 above the queue's own h2. */}
      {/* The way BACK, drawn OUTSIDE the surface state.
          A named owner this reader may not open answers 403, so the day fails
          to load and the body — the owner picker with it — never renders.
          Without this the reader is stranded on a page whose only control is
          gone, and reloading is the way out. */}
      {owner !== "" && state === "failed" && (
        <Button onClick={() => setOwner("")}>
          {t("worklist.owner.backToMine")}
        </Button>
      )}
      <SurfaceState
        state={state}
        emptyLabel={t("worklist.clear")}
        loadingLabel={t("worklist.loading")}
        detail={{ onRetry: () => void day.refetch() }}
      >
        {first && (
          <WorklistBody
            embedded={embedded}
            day={first}
            walk={walk}
            // Refreshing starts a NEW walk, which is what brings in the work
            // that arrived behind the reader — and a refetch is NOT that: it
            // re-runs every loaded page with its own cursor, so a reader who
            // had paged once resumed the walk they were asking to leave.
            onRefresh={refreshWalk}
            queue={queue}
            scope={scope}
            filter={filter}
            owner={owner}
            selectedId={selectedId}
            onScope={setScope}
            onFilter={setFilter}
            onOwner={setOwner}
            onSelect={setSelectedId}
            onOpenEmail={setOpenEmail}
            hasMore={day.hasNextPage}
            loadingMore={day.isFetchingNextPage}
            moreFailed={day.isError && day.data !== undefined}
            onMore={() => void day.fetchNextPage()}
          />
        )}
      </SurfaceState>
      {/* One drawer over the whole queue, at page level rather than inside a
          row: two mounted dialogs would be two `aria-modal` elements, and the
          day stays legible behind the message being read. */}
      {/* A reply sent from the drawer refreshes the queue, exactly as the
          row's own Reply does. Without it the message a rep just answered
          keeps its place in the waiting lane and keeps offering to answer it,
          which is the one thing this lane exists to stop saying. Invalidated
          rather than reset: the walk stays where the reader had paged it. */}
      <OpenEmailDrawer
        activityId={openEmail}
        zone={viewerZone()}
        onClose={() => setOpenEmail(null)}
        onReplySent={() =>
          void queryClient.invalidateQueries({ queryKey: worklistKey })
        }
      />
    </div>
  );
}

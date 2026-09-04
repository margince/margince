import type { ReactNode } from "react";
import { useState } from "react";
import { useUrlParams } from "../app/urlstate";
import { Button, SegmentedControl } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Eyebrow } from "../design-system/eyebrow";
import { FilterPills } from "../design-system/filterpills";
import { OpenEmailDrawer } from "../design-system/openemaildrawer";
import { PageZones } from "../design-system/pagezones";
import { Panel } from "../design-system/panel";
import { SurfaceState } from "../design-system/surfacestate";
import { formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import { useOpenEmail } from "./openemail";
import {
  bandSections,
  canReportEmptyBands,
  unbandedRows,
} from "./worklist.bands";
import { TeamBoard } from "./worklist.board";
import {
  completenessText,
  pillCount,
  sourceUnavailableText,
} from "./worklist.copy";
import {
  reviewShortfall,
  reviewWork,
  sellerWork,
} from "./worklist.destinations";
import { TeamExceptionsPanel } from "./worklist.exceptions";
import { HandledForYouPanel } from "./worklist.handled";
import { HiddenBacklogPanel } from "./worklist.hidden";
import { CoachControl, OwnerPicker } from "./worklist.manager";
import { hasPane, WorklistPane } from "./worklist.pane";
import {
  loadedQueue,
  UNASSIGNED,
  useWorklist,
  type Worklist,
  type WorklistFilter,
  type WorklistItem,
  type WorklistScope,
  type WorklistWalk,
} from "./worklist.queries";
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

// The cuts through the queue. `all` first because it is the default view.
const FILTERS: readonly WorklistFilter[] = [
  "all",
  "customer_waiting",
  "leads",
  "deals_at_risk",
  "meetings",
  "tasks",
  "decisions",
  "system",
];

/** The dial's name in the address. One spelling, read and written. */
export const WORKLIST_FILTER_PARAM = "filter";

/**
 * The lane the address asks for, or the default.
 *
 * An unknown value reads as `all` rather than as an error: the vocabulary grows
 * on the server, and a pasted link naming a lane this build has not learnt
 * should show the day rather than an empty screen or a crash.
 */
export function worklistFilterFrom(
  params: ReadonlyMap<string, string>,
): WorklistFilter {
  const asked = params.get(WORKLIST_FILTER_PARAM);
  return FILTERS.find((value) => value === asked) ?? "all";
}

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

// One row.
//
// The rank number leads because the whole promise of the page is an order; the
// reason line under the title is what makes that order checkable rather than
// something to trust.
function WorklistHeader({
  day,
  loaded,
  scope,
  filter,
  onScope,
  onFilter,
  owner,
  onOwner,
}: Readonly<{
  day: Worklist;
  // How many rows are on screen, which grows as the reader pages.
  loaded: number;
  scope: WorklistScope;
  filter: WorklistFilter;
  owner: string;
  onScope: (next: WorklistScope) => void;
  onFilter: (next: WorklistFilter) => void;
  onOwner: (next: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const scopes = day.scope_options;
  const completeness = completenessText(day, filter, t, locale, loaded);
  return (
    <div className="worklist-header">
      {/* Five figures, ONE scope: the whole assembled day. All five come off
          `summary`, which the server counts over every candidate it weighed
          rather than over the page it cut — so the sentence stays still as the
          reader pages, and a total the browser derived a second way cannot
          disagree with the bands beside it. */}
      <p className="t-h2 worklist-lead">
        {t(
          // `in_play` is optional, and a server that does not send it has not
          // said there is none — it has said nothing. Printing 0 for silence
          // is the under-reporting this line must never do, so the sentence
          // without the figure is drawn instead.
          day.summary.in_play === undefined
            ? "worklist.summary.noMiddle"
            : "worklist.summary",
          {
            urgent: formatNumber(day.summary.urgent, locale),
            due: formatNumber(day.summary.due, locale),
            inPlay: formatNumber(day.summary.in_play ?? 0, locale),
            lower: formatNumber(day.summary.lower_priority, locale),
            total: formatNumber(day.summary.total, locale),
          },
        )}
      </p>
      {/* Drawn only when there is a choice: a rep who can see only their own
          work is never offered a switch that would refuse when pressed. */}
      {scopes.length > 1 && owner === "" && (
        <SegmentedControl
          options={scopes}
          value={scope}
          onChange={onScope}
          label={t("worklist.scope.label")}
          labels={{
            mine: t("worklist.scope.mine"),
            unassigned: t("worklist.scope.unassigned"),
            team: t("worklist.scope.team"),
            all: t("worklist.scope.all"),
          }}
        />
      )}
      {/* Whose queue. Offered on the same tier the server admits a named owner
          on — `team` in the options means this reader reaches past themselves —
          so the control and the refusal cannot disagree. It replaces the scope
          switch rather than sitting beside it: a named owner outranks the scope
          word, and the pair is a 422. */}
      {scopes.includes("team") && (
        <OwnerPicker owner={owner} onOwner={onOwner} />
      )}
      <FilterPills
        pills={FILTERS.map((value) => ({
          value,
          label: t(`worklist.filter.${value}` as const),
          count: pillCount(day, value),
        }))}
        value={filter}
        onChange={onFilter}
        label={t("worklist.filter.label")}
      />
      {/* What the page is NOT showing. Drawn only when there is a difference to
          report: on a day the queue carries whole, "12 of 12" is noise. */}
      {completeness !== null && (
        <p className="t-caption worklist-completeness">{completeness}</p>
      )}
    </div>
  );
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
const NOTHING_IN_HAND = "\u0000none";

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
  // a person further down whose context is worth standing open, and skipping to
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
function WorklistBody({
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
  // The pane belongs to the DAY, so only a row in the day can fill it. A review
  // row selected here would draw its context beside the Today panel while the
  // highlighted row sat in the panel below — the two halves of one answer, a
  // screen apart, with nothing joining them.
  const selected = rowInHand(sellerWork(queue), selectedId);
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
      {/* What the day is WORTH. A SIBLING of the queue rather than part of the
          header, which is what lets the phone put the work first.

          On a wide screen it reads above the queue, where a figure describing
          the whole day belongs. On a phone the four cards stack into a 338px
          block, and with the title and controls above them the first row's
          verb landed 972px down an 844px screen — a rep opened their morning
          and had to scroll before they could do anything. The stylesheet moves
          this below the queue under 720px; the DOM order stays as it reads,
          so a screen reader still meets the day's figures before its rows. */}
      <WorklistReadings day={day} onLane={onFilter} />
      {/* Who on the team is carrying what.
          Offered on the SAME tier the server admits the board on, read off
          scope_options so the control and the refusal cannot disagree — the
          rule the owner picker beside it already keeps.
          Drawn only on the reader's OWN day. On a named person's queue the
          reader has already chosen who to look at, and a board above it would
          offer them the choice they just made. */}
      {/* WHAT is going wrong, above WHO is carrying what. A lead opens this
          page for the first question — the board answers the second, and
          answering it first asks them to infer the trouble from three counts
          per teammate. Same tier and same condition as the board: both are the
          lead's read of a team, and a rep is refused both. */}
      {owner === "" && day.scope_options.includes("team") && (
        <TeamExceptionsPanel
          enabled={day.scope_options.includes("team")}
          onOwner={onOwner}
        />
      )}
      {owner === "" && day.scope_options.includes("team") && (
        <TeamBoard
          onOwner={onOwner}
          onUnassigned={() => onScope("unassigned")}
        />
      )}
      {/* The verbs a lead has over somebody else's day. Drawn only on a named
          person's queue: on the reader's own there is nobody to coach. */}
      {owner !== "" && <CoachControl owner={owner} />}
      {/* What the queue is NOT showing. Beside the team board because it is the
          same reader's question — a lead asking whether the day their team sees
          is the day their team has — and on the same tier for the same reason.

          On the reader's OWN day only. The endpoint takes no owner and no
          scope: it derives its subject from the authenticated principal, so
          wherever the queue beside it is about somebody else, this panel is
          still answering about the reader. "412 hidden from you" stood on a
          page headed with a colleague's name and read as THEIR backlog — on
          the one surface whose whole job is to say what a queue is hiding.

          BOTH ways of leaving your own day are guarded, because there are two
          and they are reached by different controls. `owner` is the drill-down
          into a named colleague; `scope` is the picker beside it, and the team
          board's own "show me the unowned pile" moves the scope while leaving
          the owner empty. Guarding the drill-down alone left the same wrong
          figure standing under the unassigned and team queues.

          Answering it FOR a colleague is a different feature needing a
          different endpoint. Until that exists, saying nothing beats saying the
          wrong person's number under their name. */}
      {owner === "" && scope === "mine" && (
        <HiddenBacklogPanel enabled={day.scope_options.includes("team")} />
      )}
      {/* What has moved since the reader started paging. An offer to refresh
          rather than a fault: the day on screen is correct, it is simply no
          longer complete. */}
      <WalkNotice walk={walk} onRefresh={onRefresh} />
      {/* A day cannot read as clear while something that would have filled it
          was never read. This is the surface speaking about ITSELF, which is
          what Callout is for. */}
      {missing.length > 0 && (
        <Callout tone="warn">
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
        <p className="t-body worklist-clear">
          {missing.length > 0
            ? t("worklist.clearOfWhatWasRead")
            : t("worklist.clear")}
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
          pane={
            selected && hasPane(selected) ? (
              <WorklistPane item={selected} />
            ) : null
          }
          label={t("worklist.pane.title")}
          queue={
            <Panel title={t("worklist.queue")}>
              {/* The headings come from the SERVER's band list, in its draw
                  order, rather than from the rows — which is the only way a
                  band holding nothing can say so. Ranks are still counted over
                  the whole queue, so a row's number is its place on the page
                  and not its place within its heading. */}
              {bandSections(day, today).map((section) =>
                section.items.length === 0 ? (
                  canReportEmptyBands(hasMore) && (
                    <div key={section.band} className="worklist-queue-band">
                      <Eyebrow as="h3" className="worklist-band">
                        {t(`worklist.band.${section.band}` as const)}
                      </Eyebrow>
                      {/* Said, not left blank. A heading with nothing under it
                          reads as a page that failed to draw. */}
                      <p className="t-body worklist-band-clear">
                        {t(`worklist.bandClear.${section.band}` as const)}
                      </p>
                    </div>
                  )
                ) : (
                  <div key={section.band} className="worklist-queue-band">
                    <Eyebrow as="h3" className="worklist-band">
                      {t(`worklist.band.${section.band}` as const)}
                    </Eyebrow>
                    <QueueRows items={section.items} {...rowProps} />
                  </div>
                ),
              )}
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
      <ReviewPanel items={review} shortfall={reviewMissing} rows={rowProps} />
      {/* LAST, and folded shut. A reader opens this page to find what to do
          next; what is already finished answers a different question — worth
          having, and not worth leading with. It is also the only panel here
          that asks for nothing: the receipt is why the acts above it are safe
          to take. */}
      <HandledForYouPanel />
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
    <Panel title={t("worklist.review")}>
      <QueueRows items={items} {...rows} />
      {/* What this panel is NOT showing. It has no cursor of its own — review
          rows arrive as a side effect of paging the day — so a reader with an
          approval past the page cut sees a panel that looks complete and
          nothing that says otherwise. The day's own total is the denominator,
          never drawn bare: it counts every candidate the read weighed, so
          alone it would claim rows this panel does not hold. */}
      {shortfall && (
        <p className="t-caption worklist-completeness">
          {t("worklist.review.partial", {
            loaded: formatNumber(shortfall.loaded, locale),
            total: formatNumber(shortfall.total, locale),
          })}
        </p>
      )}
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
}: Readonly<{ queue: ReactNode; pane: ReactNode; label: string }>) {
  if (!pane) {
    return <>{queue}</>;
  }
  return (
    <PageZones
      shape="aside"
      mainClassName="worklist-main"
      aside={pane}
      asideLabel={label}
      main={queue}
    />
  );
}

// The Worklist screen.
export function WorklistScreen({
  opensOn,
}: Readonly<{
  // What the address asked for, from `#/worklist/<segment>`: a user id opens
  // that person's queue, and the literal "unassigned" opens the unowned pile.
  // Both are doors a team board row needs — a row that could only reach this
  // page would ask the reader to pick the same thing a second time.
  //
  // It SEEDS the dials and nothing more. They stay state afterwards and the
  // address does not follow them, for the reason the dials give below: an
  // address carrying one of four would describe a fraction of what is on screen.
  opensOn?: string;
}> = {}) {
  const t = useT();
  // The dials are state rather than a stored preference: a scope is a question
  // about right now, and a remembered one would answer a different question
  // than the reader asked on their next visit.
  const [scope, setScope] = useState<WorklistScope>(
    opensOn === UNASSIGNED ? UNASSIGNED : "mine",
  );
  // The one dial of the four that lives in the ADDRESS, and the reason is a
  // figure on another screen: Home's readings each count one of these lanes,
  // and a reading that names a set is the way into it — which it cannot be
  // unless the lane is nameable. `?filter=` is the query half, which
  // `routeIdentity` ignores by design, so this does not disturb the
  // `#/worklist/<owner>` remount that applies `opensOn`. It also makes a
  // narrowed queue a link somebody can paste, which is what the address is for.
  //
  // Scope, owner and the selected row stay state. Moving all four is still its
  // own change; this moves the one that another surface has to be able to say.
  const [params, setParams] = useUrlParams();
  const filter = worklistFilterFrom(params);
  const setFilter = (next: WorklistFilter) => {
    const query = new Map(params);
    // "all" is the default, so it is spelled by the parameter's ABSENCE — an
    // address carrying `?filter=all` describes the same view as one carrying
    // nothing and would be a second spelling of it.
    if (next === "all") {
      query.delete(WORKLIST_FILTER_PARAM);
    } else {
      query.set(WORKLIST_FILTER_PARAM, next);
    }
    setParams(query);
  };
  // Whose queue, when it is not the reader's own. Empty means their own day,
  // which is what every seat sees and the only thing most seats may ask for.
  const [owner, setOwner] = useState(
    opensOn && opensOn !== UNASSIGNED ? opensOn : "",
  );
  // Which row the context pane is about. Local state, not the address: the
  // page's other dials are state too, and putting one of the four in the URL
  // would make the address describe a fraction of what the reader is looking
  // at. Moving them all there is its own change.
  const [selectedId, setSelectedId] = useState("");
  const [openEmail, setOpenEmail] = useOpenEmail();
  // Changing a dial drops the selection. A row chosen under one question is
  // not a row the reader chose under the next one, and keeping the id means a
  // row that comes back — a filter switched away and back, a snooze that lifts
  // — re-opens its pane with nobody having asked it to.
  const answerWith =
    <T,>(set: (next: T) => void) =>
    (next: T) => {
      setSelectedId("");
      set(next);
    };
  const day = useWorklist(scope, filter, owner === "" ? undefined : owner);
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
            day={first}
            walk={walk}
            // Refreshing starts a NEW walk, which is what brings in the work
            // that arrived behind the reader. Refetching the query is exactly
            // that: the first page is fetched without a cursor, so the server
            // freezes a fresh snapshot over today's rows.
            onRefresh={() => void day.refetch()}
            queue={queue}
            scope={scope}
            filter={filter}
            owner={owner}
            selectedId={selectedId}
            onScope={answerWith(setScope)}
            onFilter={answerWith(setFilter)}
            onOwner={answerWith(setOwner)}
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
      <OpenEmailDrawer
        activityId={openEmail}
        zone={viewerZone()}
        onClose={() => setOpenEmail(null)}
      />
    </div>
  );
}

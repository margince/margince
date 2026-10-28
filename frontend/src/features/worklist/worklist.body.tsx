import type { ReactNode } from "react";
import { rosterOwnerNaming } from "../../components/entityref";
import { Badge, Button } from "../../design-system/atoms";
import { PageZones } from "../../design-system/pagezones";
import { Panel } from "../../design-system/panel";
import { useT } from "../../i18n";
import { useMemberNames } from "../../lib/membernames";
import {
  bandSections,
  canReportEmptyBands,
  unbandedRows,
} from "./worklist.bands";
import { TeamBoard } from "./worklist.board";
import { useWorklistPicks, WorklistBulkBar } from "./worklist.bulkdone";
import { clearSentence } from "./worklist.clear";
import {
  reviewShortfall,
  reviewWork,
  sellerWork,
} from "./worklist.destinations";
import { TeamExceptionsPanel } from "./worklist.exceptions";
import { HandledForYouPanel } from "./worklist.handled";
import { WorklistHeader } from "./worklist.header";
import { HiddenBacklogPanel } from "./worklist.hidden";
import { CoachControl } from "./worklist.manager";
import { hasPane, WorklistPane } from "./worklist.pane";
import type {
  Worklist,
  WorklistFilter,
  WorklistItem,
  WorklistScope,
  WorklistWalk,
} from "./worklist.queries";
import {
  NOTHING_IN_HAND,
  QueueRows,
  ReviewPanel,
  type RowContext,
  rowInHand,
} from "./worklist.queue";
import { QueueBand } from "./worklist.queuebands";
import { WorklistReadings } from "./worklist.readings";
import { OptionalRelationships } from "./worklist.relationships";
import { rowIdentity } from "./worklist.rowidentity";
import { DayUnread, dayPartlyRead } from "./worklist.unread";
import { LoadMoreOfTheDay, WalkNotice } from "./worklist.walknotice";
import { represented } from "./worklistunits";

export function WorklistBody({
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
  // What has moved since this walk started, from the latest page. The two
  // figures cover the whole walk and are recomputed on every page, so page
  // one's copy goes stale once a reader pages.
  walk: WorklistWalk | undefined;
  onRefresh: () => void;
  hasMore: boolean;
  loadingMore: boolean;
  moreFailed: boolean;
  onMore: () => void;
}>) {
  const t = useT();
  // Whose day this is, named by id, asking nothing on the reader's own day
  // (`owner` is then ""). `rosterOwnerNaming` answers null for nobody and for
  // an unnamed id alike, which is the one wording this sentence needs.
  const colleague = rosterOwnerNaming(useMemberNames(owner ? [owner] : []))(
    owner,
  );
  // Default context follows seller work. An explicit choice may also name a
  // review row, whose record context must remain reachable from the queue.
  const selected = rowInHand(
    selectedId === "" ? sellerWork(queue) : queue,
    selectedId,
  );
  // The day cut into the two jobs it holds. The server decides which, in
  // `destination`, and the counts above the queue use the same field. A split
  // derived here from `source` or `category` would drift from those figures.
  const today = sellerWork(queue);
  const review = reviewWork(queue);
  // How much review work the day holds that this panel has not loaded, in
  // items: `buckets.review` counts a folded group as its members.
  const reviewItems = represented(review);
  const reviewMissing = reviewShortfall(
    reviewItems.items,
    day.summary.buckets?.review,
    reviewItems.floor,
    filter,
  );
  const selection = useWorklistPicks(queue, `${scope}/${filter}/${owner}`);
  const openOwnerDay = (next: string) => {
    onFilter("all");
    onOwner(next);
  };

  const rowProps: RowContext = {
    // Numbered within the panel each row is drawn in, not across the day.
    //
    // A rank over the whole queue is correct but reads as a fault. The split
    // puts 1, 4, 7 in one panel and 2, 3, 5 in the other. A reader meeting a
    // list that starts at 4 and skips 5 cannot tell a correct number from a
    // broken one. A rank answers "where in this list", and the day's own order
    // is what put the rows in these lists.
    positions: new Map([
      ...sellerWork(queue).map((item, at) => [rowIdentity(item), at] as const),
      ...reviewWork(queue).map((item, at) => [rowIdentity(item), at] as const),
    ]),
    owner,
    // The resolved selection, not the raw state. The pane falls back to the
    // first row when the reader has chosen nothing. A highlight reading the
    // state alone would mark no row while the pane describes one.
    selectedId: selected ? rowIdentity(selected) : "",
    onSelect,
    onOpenEmail,
    onFilter,
    picks: selection.picks,
  };
  return (
    <>
      {/* Two columns where the page has the width. The lanes (the day's
          figures, whose day it is, and the cuts through it) stand beside the
          day and stay put while it scrolls. A reader ten rows down changes the
          cut without going back to the top. The columns wrapper is what the
          page's own width is measured against (a box cannot answer a query
          about itself). Under the fold all three wrappers dissolve (worklist.css,
          `display: contents`), and the blocks take the one column in the order
          the phone rules give them. */}
      <div className="worklist-columns">
        <div className="worklist-lanes">
          <WorklistHeader
            day={day}
            loaded={queue}
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
          {/* The reason a lead opened somebody else's day, at the head of that
          day in every state. Placed below, the block would move as its own form
          opened, and a lead would read three panels of that morning first. Only
          on a named queue. */}
          {owner !== "" && <CoachControl owner={owner} name={colleague} />}
          {/* What has moved since the reader started paging. An offer to refresh
          rather than a fault: the day on screen is correct, it is simply no
          longer complete. */}
          <WalkNotice walk={walk} onRefresh={onRefresh} />
          {/* A day cannot read as clear while something that would have filled it
          was never read. */}
          <DayUnread day={day} />
          <WorklistBulkBar selection={selection} />
          {queue.length === 0 ? (
            // One line, not a panel. No card is drawn to report a zero.
            //
            // One line rather than the four per-band ones below. Those exist
            // because a reader whose Now band is empty cannot otherwise tell it
            // from a page that starts lower. That question needs a page to ask
            // it. A wholly clear day has nothing to tell apart, and four empty
            // headings say less than one sentence that says so once.
            //
            // Under the Tasks pill the sentence names its horizon. This queue is
            // today's: the server takes open tasks due before the installation's
            // midnight, so a task due tomorrow is absent. "Nothing is waiting on
            // you" read as "you have no work" to a rep who could see three tasks
            // on the company beside it. The page offered nothing to reconcile them.
            <p className="t-body worklist-clear">
              {clearSentence(
                dayPartlyRead(day, queue),
                filter,
                colleague,
                t,
                hasMore,
              )}
            </p>
          ) : (
            // The queue, and beside it what the selected row is about.
            //
            // PageZones is used only where there is a pane. Its aside shape
            // reserves a 7fr/3fr grid whatever the aside holds. Wrapping an
            // unselected page would leave the queue at seventy per cent width.
            // The empty third beside it reads as a pane which failed to load.
            //
            // hasPane is asked before the element is made. A component returning
            // null is still an element, and an element still gets the aside column
            // and its landmark. The rule lives beside the component that obeys it.
            <PageZonesWhenPaned
              active={selectedId !== ""}
              onClose={() => onSelect(NOTHING_IN_HAND)}
              pane={
                // Not in the drawer, the one surface the pane is withheld from.
                // The pane says whom a row is about, when they last wrote and
                // when we did. The row says all three itself, on every surface.
                // On the queue's own page the column beside it is free, and the
                // pane adds the record's own reading. In a drawer it spends a
                // third of a narrow list repeating the line above it.
                !embedded && selected && hasPane(selected) ? (
                  <WorklistPane item={selected} />
                ) : null
              }
              label={t("worklist.pane.title")}
              queue={
                <Panel
                  // Indigo, the band the Brief's Focus panel wears and a
                  // record's "what needs you today" pane before it. These rows
                  // are the agent's reading of the day: what it ranked and what
                  // it prepared. Indigo is the one claim the product makes about
                  // who did that. The same panel is drawn on this page and in
                  // the queue drawer, so the claim reads the same in both.
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
                </Panel>
              }
            />
          )}
          {/* What is not the seller's to execute, below their day rather than
          inside it.
          A duplicate pair, a stopped mailbox and an approval somebody owes are
          three different jobs, and none of them is the next call to make. Drawn
          in the queue they compete with it: a rep scanning for their next
          customer steps over the product's own housekeeping to find one.
          Below, and never hidden: this work is somebody's, and a screen that
          swallowed it would be the reason it went undone. */}
          <ReviewPanel
            items={review}
            shortfall={reviewMissing}
            more={hasMore}
            changed={Boolean(walk?.changed_since_snapshot)}
            rows={rowProps}
          />
          {/* One way on for the whole day, below both panels it fills. */}
          {hasMore && (
            <LoadMoreOfTheDay
              pending={loadingMore}
              failed={moreFailed}
              onMore={onMore}
            />
          )}
          {/* Team oversight belongs to the explicitly selected wider scope. */}
          {owner === "" &&
            scope !== "mine" &&
            scope !== "unassigned" &&
            day.scope_options.includes("team") && (
              <TeamExceptionsPanel
                enabled={day.scope_options.includes("team")}
                onOwner={openOwnerDay}
              />
            )}
          {owner === "" &&
            scope !== "mine" &&
            scope !== "unassigned" &&
            day.scope_options.includes("team") && (
              <TeamBoard
                onOwner={openOwnerDay}
                onUnassigned={() => {
                  onFilter("all");
                  onScope("unassigned");
                }}
              />
            )}
          {/* This diagnostic counts all readable history, not personal obligations. */}
          {owner === "" && scope === "all" && (
            <HiddenBacklogPanel
              enabled={day.scope_options.includes("team")}
              onOpenEmail={onOpenEmail}
            />
          )}
          {/* Last, and open. A reader opens this page to find what to do next.
          What is already finished answers a different question: worth having,
          and not worth leading with, which is what its position says. It is
          also the only panel here that asks for nothing. The receipt is why the
          acts above it are safe to take, and a receipt folded shut is one
          nobody checks. */}
          <OptionalRelationships day={day} owner={owner} />
          <HandledForYouPanel />
        </div>
      </div>
    </>
  );
}

// The queue, with a pane beside it only where there is one to draw.
//
// A shape decision rather than a component. PageZones reserves its aside
// column unconditionally, and a page with nothing selected should read as it
// did before selection existed.
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

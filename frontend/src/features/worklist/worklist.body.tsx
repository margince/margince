import type { ReactNode } from "react";
import { rosterOwnerNaming } from "../../components/entityref";
import { Badge, Button } from "../../design-system/atoms";
import { PageZones } from "../../design-system/pagezones";
import { Panel } from "../../design-system/panel";
import { ScrollSentinel } from "../../design-system/scrollsentinel";
import { useT } from "../../i18n";
import { useMemberNames } from "../../lib/membernames";
import type { WorklistView } from "../../lib/worklistviews";
import {
  bandSections,
  canReportEmptyBands,
  unbandedRows,
} from "./worklist.bands";
import { TeamBoard } from "./worklist.board";
import { useWorklistPicks, WorklistBulkBar } from "./worklist.bulkdone";
import { clearSentence } from "./worklist.clear";
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
  type RowContext,
  rowInHand,
  ViewPanel,
} from "./worklist.queue";
import { QueueBand } from "./worklist.queuebands";
import { WorklistReadings } from "./worklist.readings";
import { OptionalRelationships } from "./worklist.relationships";
import { rowIdentity } from "./worklist.rowidentity";
import { DayUnread, dayPartlyRead } from "./worklist.unread";
import { UpcomingSection } from "./worklist.upcoming";
import { WalkNotice } from "./worklist.walknotice";

export function WorklistBody({
  embedded = false,
  day,
  walk,
  onRefresh,
  queue,
  scope,
  view,
  filter,
  owner,
  selectedId,
  onScope,
  onView,
  onFilter,
  onOwner,
  onSelect,
  onOpenEmail,
  cursor,
  loadingMore,
  moreFailed,
  onMore,
}: Readonly<{
  embedded?: boolean;
  day: Worklist;
  queue: readonly WorklistItem[];
  scope: WorklistScope;
  view: WorklistView;
  onView: (next: WorklistView) => void;
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
  // Where the next batch starts; absent once the view is wholly loaded.
  cursor: string | undefined;
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
  const hasMore = cursor !== undefined;
  // Only Today opens a row's context by default; another view opens it when
  // the reader picks a row.
  const selected = rowInHand(
    view === "today" || selectedId !== "" ? queue : [],
    selectedId,
  );
  const selection = useWorklistPicks(
    queue,
    `${scope}/${view}/${filter}/${owner}`,
  );
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
    positions: new Map(
      queue.map((item, at) => [rowIdentity(item), at] as const),
    ),
    owner,
    // The resolved selection, not the raw state. The pane falls back to the
    // first row when the reader has chosen nothing. A highlight reading the
    // state alone would mark no row while the pane describes one.
    selectedId: selected ? rowIdentity(selected) : "",
    onSelect,
    onOpenEmail,
    onView,
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
            view={view}
            filter={filter}
            owner={owner}
            onScope={onScope}
            onView={onView}
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
            <EmptyView
              view={view}
              rows={rowProps}
              sentence={clearSentence(
                dayPartlyRead(day, queue),
                filter,
                colleague,
                t,
                hasMore,
                view,
              )}
            />
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
                view !== "today" ? (
                  <ViewPanel view={view} items={queue} rows={rowProps} />
                ) : (
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
                    {bandSections(day, queue).map((section) => (
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
                    {unbandedRows(queue).length > 0 && (
                      <QueueRows items={unbandedRows(queue)} {...rowProps} />
                    )}
                  </Panel>
                )
              }
            />
          )}
          {/* The view loads its next batch as the reader nears its end. */}
          <ScrollSentinel
            list={`${scope}/${view}/${filter}/${owner}`}
            cursor={cursor}
            loading={loadingMore}
            failed={moreFailed}
            onLoadMore={onMore}
            end={queue.length > 0 ? t(`worklist.end.${view}`) : undefined}
          />
          {view === "today" && filter === "all" && (
            <UpcomingSection
              day={day}
              scope={scope}
              owner={owner}
              rows={rowProps}
            />
          )}
          <ScopePanels
            day={day}
            scope={scope}
            owner={owner}
            onOwnerDay={openOwnerDay}
            onUnassigned={() => {
              onFilter("all");
              onScope("unassigned");
            }}
            onOpenEmail={onOpenEmail}
          />
          {/* Last, and open. A reader opens this page to find what to do next.
          What is already finished answers a different question: worth having,
          and not worth leading with, which is what its position says. It is
          also the only panel here that asks for nothing. The receipt is why the
          acts above it are safe to take, and a receipt folded shut is one
          nobody checks. */}
          <OptionalRelationships day={day} owner={owner} scope={scope} />
          <HandledForYouPanel />
        </div>
      </div>
    </>
  );
}

// A view with no rows says so in one line. An empty Decisions view keeps its
// domain review launcher.
function EmptyView({
  view,
  rows,
  sentence,
}: Readonly<{ view: WorklistView; rows: RowContext; sentence: string }>) {
  return (
    <>
      <p className="t-body worklist-clear">{sentence}</p>
      {view !== "today" && <ViewPanel view={view} items={[]} rows={rows} />}
    </>
  );
}

// The panels a wider scope adds below the day. Team oversight shows on a team
// or all scope; the hidden-backlog diagnostic counts all readable history.
function ScopePanels({
  day,
  scope,
  owner,
  onOwnerDay,
  onUnassigned,
  onOpenEmail,
}: Readonly<{
  day: Worklist;
  scope: WorklistScope;
  owner: string;
  onOwnerDay: (next: string) => void;
  onUnassigned: () => void;
  onOpenEmail: (activityId: string) => void;
}>) {
  if (owner !== "") return null;
  const leads = day.scope_options.includes("team");
  const wide = scope !== "mine" && scope !== "unassigned" && leads;
  return (
    <>
      {wide && <TeamExceptionsPanel enabled={leads} onOwner={onOwnerDay} />}
      {wide && <TeamBoard onOwner={onOwnerDay} onUnassigned={onUnassigned} />}
      {scope === "all" && (
        <HiddenBacklogPanel enabled={leads} onOpenEmail={onOpenEmail} />
      )}
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

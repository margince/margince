import type { ReactNode } from "react";
import { useState } from "react";
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
import { TeamBoard } from "./worklist.board";
import {
  completenessText,
  pillCount,
  sourceUnavailableText,
} from "./worklist.copy";
import { FocusCard, focusOf } from "./worklist.focus";
import { HiddenBacklogPanel } from "./worklist.hidden";
import { CoachControl, OwnerPicker } from "./worklist.manager";
import { NextUp, nextUpOf } from "./worklist.nextup";
import { hasPane, WorklistPane } from "./worklist.pane";
import {
  loadedQueue,
  UNASSIGNED,
  useWorklist,
  type Worklist,
  type WorklistFilter,
  type WorklistItem,
  type WorklistScope,
} from "./worklist.queries";
import { WorklistReadings } from "./worklist.readings";
import { WorklistRow } from "./worklist.row";
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

// How much work the day holds, as opposed to how much one response carried.
//
// `summary.total` counts the rows in ONE page — it is computed after the page
// cut — so it is the wrong number to print beside a queue the reader can page
// through: it would stay at 25 while the list grew past it. `counts[].
// considered` is the figure taken before the cut, which is what "in all" means.
function dayTotal(day: Worklist): number {
  return day.counts.reduce((total, count) => total + count.considered, 0);
}

// Whether this row opens its band — the first banded row, or the first after a
// row of a DIFFERENT band.
//
// A row with no band opens nothing: an older server that does not send one
// leaves the page ungrouped rather than drawing a heading it cannot name.
//
// The comparison skips BACK over unbanded rows rather than looking only at the
// row immediately above. `band` is optional in the contract, so a queue may mix
// banded and unbanded rows — and comparing against an unbanded neighbour would
// read every banded row after one as opening its band again, drawing "Now"
// twice over one contiguous run.
function opensBand(queue: readonly WorklistItem[], index: number): boolean {
  const band = queue[index]?.band;
  if (!band) {
    return false;
  }
  for (let before = index - 1; before >= 0; before--) {
    const earlier = queue[before]?.band;
    if (earlier) {
      return earlier !== band;
    }
  }
  return true;
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
            // The DAY's candidates, not this page's rows. `summary.total`
            // counts what one response carried, so printing it beside a queue
            // the reader has paged past would shrink as they read further in.
            total: formatNumber(dayTotal(day), locale),
          },
        )}
      </p>
      {/* What the day is WORTH, above the controls that narrow it. The figures
          describe the whole day rather than the page or the filter, so they sit
          above both rather than beside the pills they would be confused with. */}
      <WorklistReadings day={day} />
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
        <p className="t-meta worklist-completeness">{completeness}</p>
      )}
    </div>
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
  hasMore: boolean;
  loadingMore: boolean;
  moreFailed: boolean;
  onMore: () => void;
}>) {
  const t = useT();
  const missing = day.sources_unavailable;
  const focus = focusOf(queue);
  // What follows it, bounded, in the server's own order. These rows may all be
  // one kind of work — worklist.nextup.tsx says why that is the ranking's to
  // fix rather than this page's.
  const next = nextUpOf(queue, focus);
  // The row the pane is about. Resolved from the identity rather than held as
  // the item itself: a refetch replaces every row object, and a held one would
  // go on describing a version of the day that is no longer on screen.
  const selected = queue.find((item) => rowIdentity(item) === selectedId);
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
      {/* Who on the team is carrying what.
          Offered on the SAME tier the server admits the board on, read off
          scope_options so the control and the refusal cannot disagree — the
          rule the owner picker beside it already keeps.
          Drawn only on the reader's OWN day. On a named person's queue the
          reader has already chosen who to look at, and a board above it would
          offer them the choice they just made. */}
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
          is the day their team has — and on the same tier for the same reason. */}
      <HiddenBacklogPanel enabled={day.scope_options.includes("team")} />
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
      {/* The one thing to do next, said rather than implied. The row stays in
          the queue below: removing it would make the rank numbers lie and the
          counts disagree with the page. */}
      {focus && <FocusCard item={focus} onOpenEmail={onOpenEmail} />}
      {/* And then? A finite list a reader can see the end of, so a morning has a
          shape rather than a backlog. Drawn only under a focus card: without
          one there is no "next", only the queue. */}
      {focus && <NextUp items={next} />}
      {queue.length === 0 ? (
        // One line, not a panel. No card is drawn to report a zero.
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
              <ol className="worklist-list">
                {queue.map((item, index) => (
                  <li key={rowIdentity(item)}>
                    {/* The heading, drawn where the band CHANGES. The server sends
                    the queue already sorted so each band is one contiguous run,
                    so a change is a boundary and never a second visit — which
                    is what lets a heading be drawn from the row rather than by
                    grouping the list into buckets the order would then fight. */}
                    {opensBand(queue, index) && (
                      <Eyebrow as="h3" className="worklist-band">
                        {t(
                          `worklist.band.${item.band ?? "keep_momentum"}` as const,
                        )}
                      </Eyebrow>
                    )}
                    <WorklistRow
                      item={item}
                      position={index + 1}
                      owner={owner}
                      selected={selectedId === rowIdentity(item)}
                      // Only where pressing it OPENS something. WorklistRow
                      // draws a plain number without this, which is what the
                      // Brief already relies on: a rank that toggles a pressed
                      // state and opens nothing teaches the reader that the
                      // page lies about what is pressable.
                      onSelect={
                        hasPane(item)
                          ? () =>
                              onSelect(
                                selectedId === rowIdentity(item)
                                  ? ""
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
    </>
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
  const [filter, setFilter] = useState<WorklistFilter>("all");
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

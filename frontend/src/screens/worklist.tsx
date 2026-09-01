import type { ReactNode } from "react";
import { useState } from "react";
import { Button, SegmentedControl } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Eyebrow } from "../design-system/eyebrow";
import { FilterPills } from "../design-system/filterpills";
import { PageZones } from "../design-system/pagezones";
import { Panel } from "../design-system/panel";
import { SurfaceState } from "../design-system/surfacestate";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import {
  completenessText,
  pillCount,
  sourceUnavailableText,
} from "./worklist.copy";
import { FocusCard, focusOf } from "./worklist.focus";
import { CoachControl, OwnerPicker } from "./worklist.manager";
import { hasPane, WorklistPane } from "./worklist.pane";
import {
  useWorklist,
  type Worklist,
  type WorklistFilter,
  type WorklistItem,
  type WorklistScope,
} from "./worklist.queries";
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
  scope,
  filter,
  onScope,
  onFilter,
  owner,
  onOwner,
}: Readonly<{
  day: Worklist;
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
  const completeness = completenessText(day, filter, t, locale);
  return (
    <div className="worklist-header">
      <p className="t-h2 worklist-lead">
        {t("worklist.summary", {
          urgent: formatNumber(day.summary.urgent, locale),
          due: formatNumber(day.summary.due, locale),
          lower: formatNumber(day.summary.lower_priority, locale),
        })}
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
        <p className="t-meta worklist-completeness">{completeness}</p>
      )}
    </div>
  );
}

// The day, drawn.
function WorklistBody({
  day,
  scope,
  filter,
  owner,
  selectedId,
  onScope,
  onFilter,
  onOwner,
  onSelect,
}: Readonly<{
  day: Worklist;
  scope: WorklistScope;
  filter: WorklistFilter;
  owner: string;
  selectedId: string;
  onScope: (next: WorklistScope) => void;
  onFilter: (next: WorklistFilter) => void;
  onOwner: (next: string) => void;
  onSelect: (next: string) => void;
}>) {
  const t = useT();
  const missing = day.sources_unavailable;
  const focus = focusOf(day.queue);
  // The row the pane is about. Resolved from the id rather than held as the
  // item itself: a refetch replaces every row object, and a held one would go
  // on describing a version of the day that is no longer on screen.
  const selected = day.queue.find((item) => item.id === selectedId);
  return (
    <>
      <WorklistHeader
        day={day}
        scope={scope}
        filter={filter}
        owner={owner}
        onScope={onScope}
        onFilter={onFilter}
        onOwner={onOwner}
      />
      {/* The verbs a lead has over somebody else's day. Drawn only on a named
          person's queue: on the reader's own there is nobody to coach. */}
      {owner !== "" && <CoachControl owner={owner} />}
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
      {focus && <FocusCard item={focus} />}
      {day.queue.length === 0 ? (
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
                {day.queue.map((item, index) => (
                  <li key={`${item.source}-${item.id}`}>
                    {/* The heading, drawn where the band CHANGES. The server sends
                    the queue already sorted so each band is one contiguous run,
                    so a change is a boundary and never a second visit — which
                    is what lets a heading be drawn from the row rather than by
                    grouping the list into buckets the order would then fight. */}
                    {opensBand(day.queue, index) && (
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
                      asOf={day.as_of}
                      selected={selectedId === item.id}
                      onSelect={() =>
                        onSelect(selectedId === item.id ? "" : item.id)
                      }
                      onReview={() => onFilter(reviewFilter(item))}
                    />
                  </li>
                ))}
              </ol>
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
export function WorklistScreen() {
  const t = useT();
  // The dials are state rather than a stored preference: a scope is a question
  // about right now, and a remembered one would answer a different question
  // than the reader asked on their next visit.
  const [scope, setScope] = useState<WorklistScope>("mine");
  const [filter, setFilter] = useState<WorklistFilter>("all");
  // Whose queue, when it is not the reader's own. Empty means their own day,
  // which is what every seat sees and the only thing most seats may ask for.
  const [owner, setOwner] = useState("");
  // Which row the context pane is about. Local state, not the address: the
  // page's other dials are state too, and putting one of the four in the URL
  // would make the address describe a fraction of what the reader is looking
  // at. Moving them all there is its own change.
  const [selectedId, setSelectedId] = useState("");
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
  const state = day.isPending ? "loading" : day.isError ? "failed" : "ready";
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
        {day.data && (
          <WorklistBody
            day={day.data}
            scope={scope}
            filter={filter}
            owner={owner}
            selectedId={selectedId}
            onScope={answerWith(setScope)}
            onFilter={answerWith(setFilter)}
            onOwner={answerWith(setOwner)}
            onSelect={setSelectedId}
          />
        )}
      </SurfaceState>
    </div>
  );
}

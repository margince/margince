import { useState } from "react";
import { Badge, SegmentedControl } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { FilterPills } from "../design-system/filterpills";
import { Panel, PanelRow } from "../design-system/panel";
import { SurfaceState } from "../design-system/surfacestate";
import { formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import {
  comparisonText,
  consequenceText,
  itemTitle,
  reasonText,
  subjectHref,
} from "./worklist.copy";
import {
  useWorklist,
  type Worklist,
  type WorklistFilter,
  type WorklistItem,
  type WorklistScope,
} from "./worklist.queries";
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

// One row.
//
// The rank number leads because the whole promise of the page is an order; the
// reason line under the title is what makes that order checkable rather than
// something to trust.
function WorklistRow({
  item,
  position,
}: Readonly<{ item: WorklistItem; position: number }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  const href = subjectHref(item);
  const title = itemTitle(item, t);
  const because = item.because
    .map((reason) => reasonText(reason, t, locale, zone))
    .join(" · ");
  const above = comparisonText(item.above_next, t, locale, zone);
  const consequence = consequenceText(item, t);
  return (
    <PanelRow className="worklist-row">
      <span className="t-caption worklist-rank" aria-hidden>
        {formatNumber(position, locale)}
      </span>
      <div className="worklist-row-text">
        <p className="t-body worklist-row-title">
          {href ? (
            <a className="worklist-row-link" href={href}>
              {title}
            </a>
          ) : (
            title
          )}
          <Badge>{t(`worklist.category.${item.category}` as const)}</Badge>
          {item.overdue && <Badge tone="danger">{t("worklist.overdue")}</Badge>}
        </p>
        {because && <p className="t-caption worklist-row-because">{because}</p>}
        {/* What it costs to do nothing. The question a queue exists to answer,
            and the one the lane feed had no field for. */}
        {consequence && (
          <p className="t-caption worklist-row-consequence">{consequence}</p>
        )}
        {/* Why this row beat the one below it. Absent on the last row, which
            has nothing below it to beat. */}
        {above && <p className="t-caption worklist-row-above">{above}</p>}
      </div>
    </PanelRow>
  );
}

// The day's figures, and the dials that narrow them.
function WorklistHeader({
  day,
  scope,
  filter,
  onScope,
  onFilter,
}: Readonly<{
  day: Worklist;
  scope: WorklistScope;
  filter: WorklistFilter;
  onScope: (next: WorklistScope) => void;
  onFilter: (next: WorklistFilter) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const scopes = day.scope_options;
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
      {scopes.length > 1 && (
        <SegmentedControl
          options={scopes}
          value={scope}
          onChange={onScope}
          label={t("worklist.scope.label")}
          labels={{
            mine: t("worklist.scope.mine"),
            team: t("worklist.scope.team"),
            all: t("worklist.scope.all"),
          }}
        />
      )}
      <FilterPills
        pills={FILTERS.map((value) => ({
          value,
          label: t(`worklist.filter.${value}` as const),
        }))}
        value={filter}
        onChange={onFilter}
        label={t("worklist.filter.label")}
      />
    </div>
  );
}

// The day, drawn.
function WorklistBody({
  day,
  scope,
  filter,
  onScope,
  onFilter,
}: Readonly<{
  day: Worklist;
  scope: WorklistScope;
  filter: WorklistFilter;
  onScope: (next: WorklistScope) => void;
  onFilter: (next: WorklistFilter) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const missing = day.sources_unavailable;
  return (
    <>
      <WorklistHeader
        day={day}
        scope={scope}
        filter={filter}
        onScope={onScope}
        onFilter={onFilter}
      />
      {/* A day cannot read as clear while something that would have filled it
          was never read. This is the surface speaking about ITSELF, which is
          what Callout is for. */}
      {missing.length > 0 && (
        <Callout tone="warn">
          {t("worklist.partial", {
            count: formatNumber(missing.length, locale),
          })}
        </Callout>
      )}
      {day.queue.length === 0 ? (
        // One line, not a panel. No card is drawn to report a zero.
        <p className="t-body worklist-clear">
          {missing.length > 0
            ? t("worklist.clearOfWhatWasRead")
            : t("worklist.clear")}
        </p>
      ) : (
        <Panel title={t("worklist.queue")}>
          {day.queue.map((item, index) => (
            <WorklistRow
              key={`${item.source}-${item.id}`}
              item={item}
              position={index + 1}
            />
          ))}
        </Panel>
      )}
    </>
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
  const day = useWorklist(scope, filter);
  const state = day.isPending ? "loading" : day.isError ? "failed" : "ready";
  return (
    <div className="wrap worklist">
      <SurfaceState
        label={t("worklist.title")}
        labelLevel="h3"
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
            onScope={setScope}
            onFilter={setFilter}
          />
        )}
      </SurfaceState>
    </div>
  );
}

import { useState } from "react";
import { Badge, SegmentedControl } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { FilterPills } from "../design-system/filterpills";
import { Panel, PanelRow } from "../design-system/panel";
import { SurfaceState } from "../design-system/surfacestate";
import { formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import { ApprovalRow } from "./approvalrow";
import {
  comparisonText,
  consequenceText,
  itemTitle,
  reasonText,
  sourceUnavailableText,
  subjectHref,
} from "./worklist.copy";
import {
  useApproval,
  useWorklist,
  type Worklist,
  type WorklistFilter,
  type WorklistItem,
  type WorklistScope,
  worklistKey,
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
    .filter((phrase): phrase is string => phrase !== null)
    .join(" · ");
  const above = comparisonText(item.above_next, t, locale, zone);
  const consequence = consequenceText(item, t);
  return (
    <PanelRow className="worklist-row">
      {/* The rank is the page's central claim, so it is readable rather than
          decorative: the list element carries the order for a screen reader and
          the number states it for everybody else. */}
      <span className="t-caption worklist-rank">
        {formatNumber(position, locale)}
      </span>
      <div className="worklist-row-text">
        <p className="t-body worklist-row-title">
          {href ? (
            <a className="entity-link" href={href}>
              {title}
            </a>
          ) : (
            title
          )}
          <Badge>{t(`worklist.category.${item.category}` as const)}</Badge>
          {item.overdue && <Badge tone="danger">{t("worklist.overdue")}</Badge>}
        </p>
        {item.batch?.sample && item.batch.sample.length > 0 && (
          // A group nobody can see into is a group nobody trusts, and an
          // untrusted group is worse than the pile it replaced.
          <p className="t-caption worklist-row-sample">
            {item.batch.sample.join(" · ")}
          </p>
        )}
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
      {item.batch ? <BatchVerb /> : <RowVerbs item={item} href={href} />}
      {decidable(item) && <RowDecision item={item} />}
    </PanelRow>
  );
}

// Whether this row is a decision a person answers HERE.
//
// The queue holds no authority of its own — the card below is the same one the
// record page draws, posting to the same endpoint. What the queue adds is that
// the decision is answerable where it was ranked, instead of sending a reader
// to a second screen to do what the row already described.
function decidable(item: WorklistItem): boolean {
  return item.actions.includes("decide") && item.source === "approval";
}

// The decision itself, fetched whole because a row cannot carry it.
function RowDecision({ item }: Readonly<{ item: WorklistItem }>) {
  const approval = useApproval(item.id, true);
  // A body with no `kind` is not a proposal this card can draw: the kind
  // chooses the label, the tool chip and the autonomy dot. Treated as a failed
  // read rather than rendered, because the alternative is a throw that takes
  // the whole day's page down over one malformed answer.
  const usable = approval.data?.kind ? approval.data : undefined;
  if (!usable) {
    return null;
  }
  return (
    <div className="worklist-row-decision">
      <ApprovalRow approval={usable} extraInvalidateKeys={[worklistKey]} />
    </div>
  );
}

// The way into a group.
//
// It narrows the queue to decisions rather than opening a screen of its own:
// that screen is its own piece of work, and a row whose only verb led nowhere
// would be worse than the pile it replaced. The filter is the door that exists.
function BatchVerb() {
  const t = useT();
  return (
    <div className="worklist-row-verbs">
      <a className="worklist-row-verb" href="#/worklist?filter=decisions">
        {t("worklist.verb.review_batch")}
      </a>
    </div>
  );
}

// What this row offers, as the item itself declares it.
//
// Every verb is a LINK to the surface that owns it rather than a mutation from
// here: this queue adds no authority of its own, so deciding an approval goes
// to the decision surface and merging a pair to the dedupe queue, exactly as
// they do from any other door. Rendering a button that acted here would be a
// second place for those rules to live.
//
// A verb whose destination this page cannot name draws nothing. A control that
// looks pressable and goes nowhere is worse than no control.
function RowVerbs({
  item,
  href,
}: Readonly<{ item: WorklistItem; href: string | undefined }>) {
  const t = useT();
  const drawn = new Set<string>();
  const verbs = item.actions.flatMap((action) => {
    const route = VERB_DESTINATION[action];
    if (!route) {
      // A verb this build cannot route draws nothing. A control that looks
      // pressable and goes nowhere is worse than no control.
      return [];
    }
    const destination = route(href);
    if (!destination) {
      return [];
    }
    // One control per DESTINATION. `complete` and `snooze` both open the
    // record this row is about, and two identical "Open" links side by side
    // ask the reader to choose between the same thing twice.
    const key = `${VERB_LABEL[action](t)}|${destination}`;
    if (drawn.has(key)) {
      return [];
    }
    drawn.add(key);
    return [{ action, destination }];
  });
  if (verbs.length === 0) {
    return null;
  }
  return (
    <div className="worklist-row-verbs">
      {verbs.map(({ action, destination }) => (
        <a key={action} className="worklist-row-verb" href={destination}>
          {VERB_LABEL[action](t)}
        </a>
      ))}
    </div>
  );
}

// Where each verb lives. A total map over the ones this page can route, so a
// verb the contract adds either gets a destination here or is not drawn —
// never a button that does nothing.
const VERB_DESTINATION: Partial<
  Record<
    WorklistItem["actions"][number],
    (href: string | undefined) => string | undefined
  >
> = {
  // `decide` and `merge` are deliberately absent: the surface that answers
  // them IS this page, so a link would send the reader where they already are.
  // They come back when the decision card is drawn inline, which is its own
  // piece of work.
  //
  // Everything routable is the record the row is about.
  open: (href) => href,
  complete: (href) => href,
  snooze: (href) => href,
  acknowledge: (href) => href,
};

// What each routable verb is called. Spelled as a map of functions rather than
// a composed key, so a verb the contract adds without copy here does not
// compile — which is the only way this cannot reach a reader as a raw word.
const VERB_LABEL: Record<
  WorklistItem["actions"][number],
  (t: ReturnType<typeof useT>) => string
> = {
  decide: (t) => t("worklist.verb.decide"),
  merge: (t) => t("worklist.verb.merge"),
  open: (t) => t("worklist.verb.open"),
  complete: (t) => t("worklist.verb.complete"),
  snooze: (t) => t("worklist.verb.snooze"),
  acknowledge: (t) => t("worklist.verb.acknowledge"),
  // The briefing queue's three verbs. Named here because the map is total over
  // the contract's actions — they route nowhere from this page yet, so
  // VERB_DESTINATION does not carry them and no control is drawn.
  act: (t) => t("worklist.verb.open"),
  dismiss: (t) => t("worklist.verb.open"),
  set_aside: (t) => t("worklist.verb.open"),
};

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
            sources: missing
              .map((source) => sourceUnavailableText(source, t))
              .join(", "),
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
          <ol className="worklist-list">
            {day.queue.map((item, index) => (
              <li key={`${item.source}-${item.id}`}>
                <WorklistRow item={item} position={index + 1} />
              </li>
            ))}
          </ol>
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
      {/* No label: the shell already heads this page, and a second "Worklist"
          would be announced twice and nest an h3 above the queue's own h2. */}
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
            onScope={setScope}
            onFilter={setFilter}
          />
        )}
      </SurfaceState>
    </div>
  );
}

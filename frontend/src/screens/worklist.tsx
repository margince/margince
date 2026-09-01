import { useState } from "react";
import { Badge, Button, SegmentedControl } from "../design-system/atoms";
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
  completenessText,
  consequenceText,
  dealFactsText,
  itemTitle,
  moveHref,
  pillCount,
  reasonText,
  rowHref,
  sourceUnavailableText,
} from "./worklist.copy";
import { CoachControl, OwnerPicker, ReassignControl } from "./worklist.manager";
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

// Which narrowing actually CONTAINS this group's members.
//
// Every group used to send the reader to `decisions`, which excludes system
// rows — so pressing Review on a broken automation filtered its own failures
// out of view and drew an empty page. A verb that hides what it promises to
// show is worse than no verb.
function reviewFilter(item: WorklistItem): WorklistFilter {
  return item.category === "system" ? "system" : "decisions";
}

// One row.
//
// The rank number leads because the whole promise of the page is an order; the
// reason line under the title is what makes that order checkable rather than
// something to trust.
function WorklistRow({
  item,
  position,
  owner,
  asOf,
  onReview,
}: Readonly<{
  item: WorklistItem;
  position: number;
  // Whose queue this row is on, empty for the reader's own. A row can only be
  // handed to somebody else from a page that is already about somebody else.
  owner: string;
  // When the server took this snapshot. The waiting_days tie-break's elapsed
  // days are computed against THIS, not the render's own wall clock — a cached
  // read rendered later, or a client clock that has drifted from the
  // server's, must not silently change what the row says about an order the
  // server already decided as of a fixed instant.
  asOf: string;
  onReview: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  const href = rowHref(item);
  const title = itemTitle(item, t, locale);
  const facts = dealFactsText(item, t, locale, zone);
  const because = item.because
    .map((reason) => reasonText(reason, t, locale, zone))
    .filter((phrase): phrase is string => phrase !== null)
    .join(" · ");
  const above = comparisonText(
    item.above_next,
    t,
    locale,
    zone,
    new Date(asOf),
  );
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
        {/* `detail` is not prose on every source: a relationship-decay row
            carries a bare day COUNT there (attention/render.go's lapsedItem,
            "the client writes 'quiet N days'"), which this row already says
            properly through `because`. Only the `notice` source's detail is
            a full sentence — the server sets it from the notice's own body,
            the deal's name included — so only that source renders it here
            rather than every source that happens to send one. */}
        {item.source === "notice" && item.detail && (
          <p className="t-caption worklist-row-detail">{item.detail}</p>
        )}
        {item.batch?.sample && item.batch.sample.length > 0 && (
          // A group nobody can see into is a group nobody trusts, and an
          // untrusted group is worse than the pile it replaced.
          <p className="t-caption worklist-row-sample">
            {item.batch.sample.join(" · ")}
          </p>
        )}
        {facts && <p className="t-caption worklist-row-facts">{facts}</p>}
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
      {item.batch ? (
        <BatchVerb onReview={onReview} />
      ) : (
        <RowVerbs item={item} href={href} move={moveHref(item)} />
      )}
      {/* Only a task carries an assignee, so only a task can be handed on. A
          group row stands for a pile and names no single activity to move. */}
      {owner !== "" && item.source === "task" && !item.batch && (
        <ReassignControl item={item} owner={owner} />
      )}
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
// would be worse than the pile it replaced.
//
// A button, not a link. The dials live in this screen's state today, so an
// address carrying `?filter=decisions` would be read by nobody and the control
// would do nothing — which is the defect it exists to avoid. Moving them into
// the URL is the right shape and is its own change.
function BatchVerb({ onReview }: Readonly<{ onReview: () => void }>) {
  const t = useT();
  return (
    <div className="worklist-row-verbs">
      <Button small onClick={onReview}>
        {t("worklist.verb.review_batch")}
      </Button>
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
  move,
}: Readonly<{
  item: WorklistItem;
  href: string | undefined;
  move: string | undefined;
}>) {
  const t = useT();
  const drawn = new Set<string>();
  type Verb = {
    action: WorklistItem["actions"][number];
    destination: string;
  };
  const verbs = item.actions.flatMap<Verb>((action) => {
    if (action === "decide") {
      const to = decideDestination(item, href);
      return to ? [{ action, destination: to }] : [];
    }
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
  if (verbs.length === 0 && !move) {
    return null;
  }
  return (
    <div className="worklist-row-verbs">
      {/* The step the product already worked out, offered where the reader is
          standing rather than on a screen they have to go and find. */}
      {move && (
        <a className="link-button" href={move}>
          {t("worklist.verb.draft_reply")}
        </a>
      )}
      {verbs.map(({ action, destination }) => (
        <a key={action} className="link-button" href={destination}>
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

// The one verb whose routing depends on the SOURCE rather than only the verb.
//
// `decide` is answered inline for an approval — the card is right there, so a
// link would send the reader where they already are. An introduction ask has no
// inline card: its four answers are the colleague's own, given on the contact's
// Network tab. Without this the ask row names somebody waiting and offers
// nothing at all, which is the worst of both.
function decideDestination(
  item: WorklistItem,
  href: string | undefined,
): string | undefined {
  return item.source === "introduction_request" ? href : undefined;
}

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
  onScope,
  onFilter,
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
  const missing = day.sources_unavailable;
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
                <WorklistRow
                  item={item}
                  position={index + 1}
                  owner={owner}
                  asOf={day.as_of}
                  onReview={() => onFilter(reviewFilter(item))}
                />
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
  // Whose queue, when it is not the reader's own. Empty means their own day,
  // which is what every seat sees and the only thing most seats may ask for.
  const [owner, setOwner] = useState("");
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
            onScope={setScope}
            onFilter={setFilter}
            onOwner={setOwner}
          />
        )}
      </SurfaceState>
    </div>
  );
}

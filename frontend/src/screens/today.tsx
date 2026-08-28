import {
  CalendarClock,
  CheckSquare,
  GitMerge,
  Handshake,
  Scale,
  ShieldAlert,
  Sparkles,
  Sunrise,
  TrendingDown,
  UserMinus,
} from "lucide-react";
import { useState } from "react";
import { Badge, Button, EmptyState } from "../design-system/atoms";
import { Panel, PanelBody, PanelRow } from "../design-system/panel";
import { SurfaceState } from "../design-system/surfacestate";
import { formatNumber } from "../format/format";
import { useNow } from "../format/now";
import { viewerZone } from "../format/timezone";
import { type Locale, useLocale, useT } from "../i18n";
import { itemDetail, itemTitle } from "./attentionitemcopy";
import { subjectHref } from "./attentionsubject";
import { BriefQueueItem } from "./briefqueue";
import {
  useBriefItemMark,
  useHomeDeals,
  useMorningBrief,
} from "./home.queries";
import { snoozedDueAt, useTaskUpdate } from "./taskactions";
import { FocusLane } from "./today.focus";
import {
  type Attention,
  type AttentionItem,
  type AttentionLane,
  attentionKey,
  useAttention,
} from "./today.queries";
import "./today.css";

// Today — the one surface that answers "what needs me?".
//
// Five producers used to ask that question from four screens, so answering it
// meant visiting all four and adding up. This page is the sum, and its shape is
// the answer's shape: what only a person can decide, what is already agreed,
// and what ran without asking.
//
// The order is not negotiable and is not a preference. Decisions lead because
// they are the only things here that cannot proceed without the reader.
// Receipts come last because they are finished — they are owed to the reader as
// a fact, and a finished act placed above an open one would be asking for
// attention it does not need.

// What a lane is for, in the reader's own language, plus the icon that carries
// it in a glance. Copy is looked up by the caller, never spelled in a primitive.
type LaneShape = Readonly<{
  title: string;
  empty: string;
  withheld: string;
  icon: typeof CheckSquare;
}>;

// The lead line: the largest true thing about the day, written here rather than
// on the server because the server has no language. A reader who reads only
// this line should still know whether to worry.
function leadLine(
  day: Attention,
  t: ReturnType<typeof useT>,
  locale: Locale,
): string {
  // A day with a lane missing from it is not a day this line may summarise:
  // "your day is clear" and "part of it is hidden from you" cannot both be on
  // one page, and the reader would believe the reassuring one.
  if ((day.lanes_omitted ?? []).length > 0) {
    return t("day.lead.partial");
  }
  // Meetings lead every other lane, because a meeting is the one thing on this
  // page that happens whether or not the reader acts. A decision waits; an
  // appointment at eleven does not.
  const booked = day.counts.meetings ?? 0;
  if (booked > 0) {
    return t("day.lead.meetings", { count: formatNumber(booked, locale) });
  }
  const decisions = day.counts.needs_you;
  if (decisions === 1) {
    return t("day.lead.oneDecision");
  }
  if (decisions > 1) {
    return t("day.lead.decisions", { count: formatNumber(decisions, locale) });
  }
  if (day.counts.planned > 0) {
    return t("day.lead.plannedOnly", {
      count: formatNumber(day.counts.planned, locale),
    });
  }
  // Above the briefing lane and below planned work, which is what a promise
  // IS: something the reader already agreed to, and the strongest claim on the
  // day short of a decision waiting on them. A briefing item only suggests
  // where to start; a promise is owed to somebody.
  const promises = day.counts.commitments ?? 0;
  if (promises > 0) {
    return t("day.lead.promises", { count: formatNumber(promises, locale) });
  }
  return quietLead(day, t, locale);
}

// What the line says once nothing is WAITING on the reader — no decision, no
// promise due, no meeting. Split from the branches above because those answer
// "what is being asked of you" and these answer "what is worth knowing anyway",
// and one function holding both was past the complexity the linter allows.
function quietLead(
  day: Attention,
  t: ReturnType<typeof useT>,
  locale: Locale,
): string {
  // A drifting deal is money leaving on its own — nobody is waiting on the
  // reader for it, which is exactly why it needs saying.
  // Above the drifting deals, because it is worse news: a rep already DECIDED
  // these, was told they worked, and they did not.
  const failed = day.counts.did_not_run ?? 0;
  if (failed > 0) {
    return t("day.lead.didNotRun", { count: formatNumber(failed, locale) });
  }
  const drifting = day.counts.at_risk ?? 0;
  if (drifting > 0) {
    return t("day.lead.atRisk", { count: formatNumber(drifting, locale) });
  }
  // A lapsed relationship is the same shape of news as a drifting deal:
  // nobody is waiting on the reader for it, and it is the thing that goes
  // unnoticed precisely because nobody is. Counted here rather than left to
  // the check below, because that check only catches a lane that was never
  // READ — a lane read and holding three contacts would still have printed
  // "clear" above them.
  const lapsed = day.counts.relationship_decay ?? 0;
  if (lapsed > 0) {
    return t("day.lead.decay", { count: formatNumber(lapsed, locale) });
  }
  // Before "clear", because the briefing lane is on this page: a line reading
  // "your day is clear" above two items the night picked out is the one thing
  // this line exists to prevent. It sits below decisions and planned work,
  // which are things that wait on the reader, where a briefing item only
  // suggests where to start.
  if (day.counts.this_morning > 0) {
    return t("day.lead.morningOnly", {
      count: formatNumber(day.counts.this_morning, locale),
    });
  }
  if (day.done_for_you && day.done_for_you.length > 0) {
    return t("day.lead.ranOvernight");
  }
  // "Clear" is the ONE line an absent lane can falsify, so it is the one that
  // checks. Every branch above states something this page measured; "clear"
  // states that nothing was found ANYWHERE, and a feed that never read the
  // claims has not looked where promises live. The weaker line says only what
  // is true — nothing is waiting among the lanes this page did read.
  if (
    day.counts.commitments === undefined ||
    day.counts.at_risk === undefined ||
    day.counts.relationship_decay === undefined ||
    day.counts.did_not_run === undefined ||
    day.counts.meetings === undefined
  ) {
    return t("day.lead.clearOfWhatWasRead");
  }
  return t("day.lead.clear");
}

// One row of the two REPORTING lanes — today's agreed work, and what already
// ran. Neither carries a decision: a task is completed or pushed, and a receipt
// is read. The lane that asks a person something does not use this row at all,
// because a decision does not fit on one.
//
// A row reaches the record it is about in one of two ways, and which one is
// decided by whether the row carries verbs. A row with no verbs is a single
// press target filling the row, which is what `interactive` is for. A row with
// Done and Tomorrow on it is not: those draw their own hover, and a fill behind
// them would claim a hit area the row does not have — so there the NAME is the
// link and the verbs keep their own targets.
function AttentionRow({
  item,
  onComplete,
  onSnooze,
  completing,
}: Readonly<{
  item: AttentionItem;
  onComplete: (id: string) => void;
  onSnooze: (id: string, dueAt: string) => void;
  completing: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  const detail = itemDetail(item, t, locale, zone);
  const href = subjectHref(item);
  const actionable = item.actions.includes("complete");
  const title = itemTitle(item, t);
  // The name is its own link only on a row that ALREADY carries verbs. On a
  // verb-less row the whole row is the link, and a second anchor inside it
  // would be a target within a target.
  const linkedTitle = href && actionable;
  const text = (
    <div className="today-row-text">
      <p className="t-body today-row-title">
        {linkedTitle ? (
          <a className="today-row-link" href={href}>
            {title}
          </a>
        ) : (
          title
        )}
      </p>
      {detail && <p className="t-caption today-row-detail">{detail}</p>}
    </div>
  );
  const verbs = (
    <div className="today-row-verbs">
      {item.overdue && <Badge tone="danger">{t("day.overdue")}</Badge>}
      {actionable && (
        <>
          {/* Not now, rather than not at all: a task pushed a day is still
              agreed work, and a queue whose only verbs are "done" and
              nothing teaches a reader to leave it open forever. */}
          {item.actions.includes("snooze") && item.due_at && (
            <Button
              small
              onClick={() => onSnooze(item.id, item.due_at ?? "")}
              disabled={completing}
            >
              {t("day.snooze")}
            </Button>
          )}
          <Button
            small
            onClick={() => onComplete(item.id)}
            disabled={completing}
          >
            {t("day.complete")}
          </Button>
        </>
      )}
    </div>
  );
  if (href && !actionable) {
    return (
      <PanelRow interactive>
        <a className="today-row today-row-whole" href={href}>
          {text}
          {verbs}
        </a>
      </PanelRow>
    );
  }
  return (
    <PanelRow className="today-row">
      {text}
      {verbs}
    </PanelRow>
  );
}

// One lane. `tone="accent"` marks the ONE panel that asks for a move — the
// decisions — so a reader scanning the page finds it before reading a word.
// Two tinted panels would be no lead at all, which is why the other two are
// plain.
// An OPTIONAL lane: one the server may not send at all.
//
// Absent means this installation does not read what the lane holds; empty means
// it read and found nothing. The two draw differently — nothing at all versus a
// quiet plate — so the check belongs in one place rather than repeated per lane,
// where the third copy is what pushed this screen past the complexity bar.
function OptionalLane({
  items,
  shape,
  omitted,
  lane,
  total,
  tone,
  onComplete,
  onSnooze,
  completing,
}: Readonly<{
  items: readonly AttentionItem[] | undefined;
  shape: LaneShape;
  omitted: readonly AttentionLane[];
  lane: AttentionLane;
  total: number;
  tone?: "accent" | "warn";
  onComplete: (id: string) => void;
  onSnooze: (id: string, dueAt: string) => void;
  completing: boolean;
}>) {
  const withheld = omitted.includes(lane);
  if (items === undefined && !withheld) {
    return null;
  }
  return (
    <Lane
      shape={shape}
      items={items ?? []}
      withheld={withheld}
      total={total}
      tone={tone}
      onComplete={onComplete}
      onSnooze={onSnooze}
      completing={completing}
    />
  );
}

function Lane({
  shape,
  items,
  withheld,
  total,
  tone,
  onComplete,
  onSnooze,
  completing,
}: Readonly<{
  shape: LaneShape;
  items: readonly AttentionItem[];
  // The reader may not read what this lane holds. Drawn in place of the rows,
  // because an empty lane and a hidden one look identical and only one of them
  // means "nothing here".
  withheld: boolean;
  // What the lane HOLDS, which is not what it shows: the lane is bounded so it
  // stays finishable, and a badge counting the page would tell a reader with
  // forty decisions that they have nine.
  total: number;
  // `warn` is the lead whose FINDING is the bad news — a deal nobody has
  // touched in months. It is not a second accent: the decisions lane is the
  // one panel asking for a MOVE, and this one reports something going wrong.
  tone?: "accent" | "warn";
  onComplete: (id: string) => void;
  onSnooze: (id: string, dueAt: string) => void;
  completing: boolean;
}>) {
  const { locale } = useLocale();
  const Icon = shape.icon;
  return (
    <Panel
      title={
        <span className="today-lane-title">
          <Icon size={16} aria-hidden />
          {shape.title}
        </span>
      }
      titleAction={
        total > 0 ? <Badge>{formatNumber(total, locale)}</Badge> : undefined
      }
      tone={tone}
    >
      {withheld ? (
        <PanelBody>
          <EmptyState>{shape.withheld}</EmptyState>
        </PanelBody>
      ) : items.length === 0 ? (
        <PanelBody>
          <EmptyState>{shape.empty}</EmptyState>
        </PanelBody>
      ) : (
        items.map((item) => (
          <AttentionRow
            key={`${item.source}-${item.id}`}
            item={item}
            onComplete={onComplete}
            onSnooze={onSnooze}
            completing={completing}
          />
        ))
      )}
    </Panel>
  );
}

// The overnight brief's lane: what the night thought the day was for.
//
// It draws the same card Home does, through the same connected component, so
// there is one answer to "what does a brief item look like and what happens
// when you press its buttons".
//
// The lane's ITEMS come from the feed and their CONTENT from the brief's own
// read, which is two requests for one lane and is deliberate. The alternative
// is copying the ranking payload — five factors, the composite, the evidence —
// through the attention contract as well, and then two wires carry the same
// numbers and can disagree. The feed says WHICH entries are still waiting; the
// brief says what each one is.
//
// Plain rather than `tone="accent"`: the decisions lane below is the one panel
// that asks for a move, and a second tinted panel would leave the page with no
// lead at all. A briefing item is a suggestion about where to start.
function MorningLane({
  items,
  withheld,
  total,
}: Readonly<{
  items: readonly AttentionItem[];
  withheld: boolean;
  total: number;
}>) {
  const t = useT();
  // Withheld or empty is answered from the FEED alone, before the brief and the
  // deals reads are mounted at all. Those two exist to draw cards, so asking for
  // them when the feed has already said there are none is two requests for a
  // panel that will show a sentence — and on a withheld lane one of them is a
  // 403 the reader was never going to see the answer to.
  if (withheld) {
    return (
      <MorningPanel total={total}>
        <PanelBody>
          <EmptyState>{t("day.lane.withheld")}</EmptyState>
        </PanelBody>
      </MorningPanel>
    );
  }
  if (items.length === 0) {
    return (
      <MorningPanel total={total}>
        <PanelBody>
          <EmptyState>{t("day.thisMorning.empty")}</EmptyState>
        </PanelBody>
      </MorningPanel>
    );
  }
  return <MorningCards items={items} total={total} />;
}

// The lane's chrome, shared by the three readings so the heading, the icon and
// the badge cannot come to differ between them.
function MorningPanel({
  total,
  children,
}: Readonly<{ total: number; children: React.ReactNode }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <Panel
      title={
        <span className="today-lane-title">
          <Sunrise size={16} aria-hidden />
          {t("day.thisMorning")}
        </span>
      }
      titleAction={
        total > 0 ? <Badge>{formatNumber(total, locale)}</Badge> : undefined
      }
    >
      {children}
    </Panel>
  );
}

// The lane with entries in it: the feed's ids joined to the brief's own items.
function MorningCards({
  items,
  total,
}: Readonly<{ items: readonly AttentionItem[]; total: number }>) {
  const t = useT();
  // Pinned rather than ticking: nothing this lane DRAWS depends on the clock,
  // and the instant is read only after a click, to work out when a set-aside
  // item comes back. A one-second interval here would re-render every card on
  // the lane sixty times a minute to change nothing.
  const nowMs = useNow(0);
  const brief = useMorningBrief();
  const deals = useHomeDeals();
  const mark = useBriefItemMark();
  // The feed decides which entries are in the lane and in what order; the
  // brief's own read supplies each one's content. An id the brief has not
  // caught up with is dropped rather than drawn empty — half a card is worse
  // than one card fewer.
  //
  // An item the brief already reports as answered leaves immediately, without
  // waiting for the feed to agree. The mark patches the brief cache straight
  // away and the feed catches up on its own refetch, so trusting the feed alone
  // would leave a just-answered card sitting in a lane whose whole rule is that
  // it holds only unanswered work — visibly settled, for as long as the network
  // takes.
  const ordered = items.flatMap((row) => {
    const found = (brief.data?.items ?? []).find((item) => item.id === row.id);
    return found && found.state === "new" ? [found] : [];
  });
  // Named by the feed but not yet drawable — which is not the same as answered.
  // An item the reader just answered is legitimately gone from the lane and the
  // quiet plate is the honest thing to show once the last one goes; an item the
  // brief has simply not delivered is still coming.
  const undrawable = items.filter(
    (row) => !(brief.data?.items ?? []).some((item) => item.id === row.id),
  );
  if (undrawable.length > 0 && ordered.length === 0) {
    // The feed named entries this lane cannot draw yet. That is NOT a quiet
    // morning, and saying so is the whole difference: the two reads settle
    // independently, so a lane that showed the quiet plate here would tell a
    // rep the night found nothing on a morning it found work.
    return (
      <MorningPanel total={total}>
        <PanelBody>
          <SurfaceState
            state={brief.isError ? "failed" : "loading"}
            loadingLabel={t("day.loading")}
            emptyLabel={t("day.thisMorning.empty")}
            detail={{ onRetry: () => void brief.refetch() }}
          >
            {null}
          </SurfaceState>
        </PanelBody>
      </MorningPanel>
    );
  }
  if (ordered.length === 0) {
    return (
      <MorningPanel total={total}>
        <PanelBody>
          <EmptyState>{t("day.thisMorning.empty")}</EmptyState>
        </PanelBody>
      </MorningPanel>
    );
  }
  return (
    <MorningPanel total={total}>
      <PanelBody className="today-morning-list">
        {ordered.map((item) => (
          <BriefQueueItem
            key={item.id}
            item={item}
            deals={deals.data?.rows ?? []}
            nowMs={nowMs}
            mark={mark}
          />
        ))}
      </PanelBody>
    </MorningPanel>
  );
}

// The day, assembled.
export function TodayScreen() {
  const t = useT();
  const day = useAttention();
  // The SAME mutation the task queue and the record page use, handed every key
  // that must forget what it cached. `["tasks"]` is the task queue's own key
  // and is easy to miss: without it, completing something here leaves it
  // sitting open on the Tasks screen until that query goes stale by itself.
  const complete = useTaskUpdate([attentionKey, ["tasks"], ["activities"]]);
  // Read once per render so every lane on the page agrees what "now" is.
  useNow(60_000);

  // Derived from THIS query's own states rather than through the composite
  // helper: that one answers per-section withholding inside a shared payload,
  // and here one read carries the whole page. Loading and failed are told
  // apart, because a page that called itself broken while its data was still
  // arriving would be wrong on every slow connection.
  const state = day.isPending ? "loading" : day.isError ? "failed" : "ready";
  return (
    <div className="today">
      <SurfaceState
        label={t("day.title")}
        labelLevel="h3"
        state={state}
        emptyLabel={t("day.lead.clear")}
        loadingLabel={t("day.loading")}
      >
        {day.data && <TodayLanes day={day.data} complete={complete} />}
      </SurfaceState>
    </div>
  );
}

function TodayLanes({
  day,
  complete,
}: Readonly<{
  day: Attention;
  complete: ReturnType<typeof useTaskUpdate>;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const needsYou = day.needs_you ?? [];
  const planned = day.planned ?? [];
  // Absent means this installation serves no commitments lane; empty means the
  // rep owes nothing today. The two draw differently, so they stay apart.
  const commitments = day.commitments;
  // Same absent-versus-empty rule: no lane at all when nothing reads deals.
  const atRisk = day.at_risk;
  // Decisions this reader approved whose released work then failed. Warn-toned
  // whenever it holds anything: every row is a promise the product broke.
  const failed = day.did_not_run;
  // Data-subject requests with running legal clocks. Only a DSR admin is
  // served this lane at all; warn-toned whenever it holds anything, because
  // every row is a deadline the law set.
  const requests = day.dsr;
  const requestsTone = (requests ?? []).length > 0 ? "warn" : undefined;
  const failedTone = (failed ?? []).length > 0 ? "warn" : undefined;
  const lapsed = day.relationship_decay;
  // Tinted only when the lane HAS a finding: a warn-toned panel drawn over "no
  // deal is drifting" would dress good news as bad.
  const driftingTone = (atRisk ?? []).length > 0 ? "warn" : undefined;
  const meetings = day.meetings;
  const done = day.done_for_you ?? [];
  const omitted = day.lanes_omitted ?? [];
  // How many decisions this reader has answered since the page opened, and
  // which ones they have pushed to the back of the queue.
  //
  // Deferral is held HERE rather than sent, because "not now" is a fact about
  // this sitting and not about the decision: it must not follow the reader to
  // another device or survive until tomorrow, and there is no server state that
  // would mean that. A decided item leaves the queue on its own — the mutation
  // invalidates the read — so nothing has to remember those.
  const [decided, setDecided] = useState(0);
  const [deferred, setDeferred] = useState<readonly string[]>([]);
  const onDecided = () => setDecided((n) => n + 1);
  // Deferred items go to the BACK, never away: a reader who put three things
  // off still has three things to do, and a queue that dropped them would
  // report a clear day that is not one.
  const queue = [
    ...needsYou.filter((item) => !deferred.includes(item.id)),
    ...needsYou.filter((item) => deferred.includes(item.id)),
  ];
  // Defers the card the reader is LOOKING at, which is the head of the queue
  // and not the head of the lane. The two are the same only until the first
  // deferral; after that the lane's head has moved to the back, so deferring it
  // again is a no-op and every later press of "later" does nothing.
  const onSkip = () =>
    setDeferred((ids) =>
      queue[0] && !ids.includes(queue[0].id) ? [...ids, queue[0].id] : ids,
    );
  const onComplete = (id: string) =>
    complete.mutate({ id, body: { is_done: true } });
  // One day later, through the same helper the task queue snoozes with, so the
  // two surfaces cannot come to mean different things by the word.
  const onSnooze = (id: string, dueAt: string) => {
    const next = snoozedDueAt(dueAt);
    if (next) {
      complete.mutate({ id, body: { due_at: next } });
    }
  };
  return (
    <>
      <p className="t-h2 today-lead">{leadLine(day, t, locale)}</p>
      <MorningLane
        items={day.this_morning ?? []}
        withheld={omitted.includes("this_morning")}
        total={day.counts.this_morning}
      />
      <OptionalLane
        items={meetings}
        shape={{
          title: t("day.meetings"),
          empty: t("day.meetings.empty"),
          withheld: t("day.lane.withheld"),
          icon: CalendarClock,
        }}
        omitted={omitted}
        lane="meetings"
        total={day.counts.meetings ?? 0}
        onComplete={onComplete}
        onSnooze={onSnooze}
        completing={complete.isPending}
      />
      <FocusLane
        items={queue}
        total={day.counts.needs_you}
        decided={decided}
        withheld={omitted.includes("needs_you")}
        onDecided={onDecided}
        onSkip={onSkip}
      />
      <Lane
        shape={{
          title: t("day.planned"),
          empty: t("day.planned.empty"),
          withheld: t("day.lane.withheld"),
          icon: CheckSquare,
        }}
        items={planned}
        withheld={omitted.includes("planned")}
        total={day.counts.planned}
        onComplete={onComplete}
        onSnooze={onSnooze}
        completing={complete.isPending}
      />
      <OptionalLane
        items={atRisk}
        shape={{
          title: t("day.atRisk"),
          empty: t("day.atRisk.empty"),
          withheld: t("day.lane.withheld"),
          icon: TrendingDown,
        }}
        omitted={omitted}
        lane="at_risk"
        total={day.counts.at_risk ?? 0}
        tone={driftingTone}
        onComplete={onComplete}
        onSnooze={onSnooze}
        completing={complete.isPending}
      />
      <OptionalLane
        items={requests}
        shape={{
          title: t("day.dsr"),
          empty: t("day.dsr.empty"),
          withheld: t("day.lane.withheld"),
          icon: Scale,
        }}
        omitted={omitted}
        lane="dsr"
        total={day.counts.dsr ?? 0}
        tone={requestsTone}
        onComplete={onComplete}
        onSnooze={onSnooze}
        completing={complete.isPending}
      />
      <OptionalLane
        items={failed}
        shape={{
          title: t("day.didNotRun"),
          empty: t("day.didNotRun.empty"),
          withheld: t("day.lane.withheld"),
          icon: ShieldAlert,
        }}
        omitted={omitted}
        lane="did_not_run"
        total={day.counts.did_not_run ?? 0}
        tone={failedTone}
        onComplete={onComplete}
        onSnooze={onSnooze}
        completing={complete.isPending}
      />
      <OptionalLane
        items={lapsed}
        shape={{
          title: t("day.decay"),
          empty: t("day.decay.empty"),
          withheld: t("day.lane.withheld"),
          icon: UserMinus,
        }}
        omitted={omitted}
        lane="relationship_decay"
        total={day.counts.relationship_decay ?? 0}
        onComplete={onComplete}
        onSnooze={onSnooze}
        completing={complete.isPending}
      />
      <OptionalLane
        items={commitments}
        shape={{
          title: t("day.commitments"),
          empty: t("day.commitments.empty"),
          withheld: t("day.lane.withheld"),
          icon: Handshake,
        }}
        omitted={omitted}
        lane="commitments"
        total={day.counts.commitments ?? 0}
        onComplete={onComplete}
        onSnooze={onSnooze}
        completing={complete.isPending}
      />
      <Lane
        shape={{
          title: t("day.done"),
          empty: t("day.done.empty"),
          withheld: t("day.lane.withheld"),
          icon: Sparkles,
        }}
        items={done}
        withheld={omitted.includes("done_for_you")}
        total={done.length}
        onComplete={onComplete}
        onSnooze={onSnooze}
        completing={complete.isPending}
      />
      {day.counts.duplicates_open != null && day.counts.duplicates_open > 0 && (
        <p className="t-caption today-foot">
          <GitMerge size={14} aria-hidden />
          {t("day.duplicatesOpen", {
            count: formatNumber(day.counts.duplicates_open, locale),
          })}
        </p>
      )}
    </>
  );
}

import {
  CheckSquare,
  GitMerge,
  Handshake,
  Sparkles,
  Sunrise,
} from "lucide-react";
import { useState } from "react";
import { Badge, Button, EmptyState } from "../design-system/atoms";
import { Panel, PanelBody, PanelRow } from "../design-system/panel";
import { SurfaceState } from "../design-system/surfacestate";
import { formatDateTime, formatNumber } from "../format/format";
import { useNow } from "../format/now";
import { viewerZone } from "../format/timezone";
import { type Locale, useLocale, useT } from "../i18n";
import { approvalKindLabel } from "./approvalkind";
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
  if (day.counts.commitments === undefined) {
    return t("day.lead.clearOfWhatWasRead");
  }
  return t("day.lead.clear");
}

// One item's headline.
//
// The server sends a sentence when it HAS one — an approval summary is composed
// at staging time out of the proposal's own facts. It deliberately sends none
// for a duplicate pair, because that sentence would have to be invented and the
// server has no language to invent it in. So the client writes that one, in the
// reader's.
function itemTitle(item: AttentionItem, t: ReturnType<typeof useT>): string {
  if (item.title) {
    return item.title;
  }
  if (item.source === "dedupe_candidate") {
    return t(`day.duplicate.${duplicateNoun(item.kind)}` as const);
  }
  return item.kind ? approvalKindLabel(item.kind, t) : t("day.item.untitled");
}

// Which noun a duplicate pair is about. An unrecognised entity type falls back
// to the generic line rather than printing the wire word at a reader.
function duplicateNoun(kind: string | undefined): "person" | "org" | "lead" {
  if (kind === "organization") {
    return "org";
  }
  if (kind === "lead") {
    return "lead";
  }
  return "person";
}

// The supporting line under a headline: how sure the detector was, or when this
// is due, or when it happened. At most one — a card that stacked all three
// would make the reader read three things to learn one.
function itemDetail(
  item: AttentionItem,
  t: ReturnType<typeof useT>,
  locale: Locale,
  zone: string,
): string | null {
  // A promise is the one item whose supporting line carries TWO facts, and it
  // needs both: the words it was read from are what make the claim checkable,
  // and the deadline is why it is on today's page at all. Showing only the
  // quote would drop the date the lane is ordered by.
  if (item.source === "conversation_claim" && item.detail && item.due_at) {
    return t("day.commitment.detail", {
      quote: item.detail,
      due: formatDateTime(item.due_at, locale, zone),
    });
  }
  if (item.detail) {
    return item.detail;
  }
  if (item.confidence != null) {
    return t("day.match", {
      percent: formatNumber(Math.round(item.confidence * 100), locale),
    });
  }
  if (item.occurred_at) {
    return formatDateTime(item.occurred_at, locale, zone);
  }
  if (item.due_at) {
    return formatDateTime(item.due_at, locale, zone);
  }
  return null;
}

// One row of the two REPORTING lanes — today's agreed work, and what already
// ran. Neither carries a decision: a task is completed or pushed, and a receipt
// is read. The lane that asks a person something does not use this row at all,
// because a decision does not fit on one.
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
  return (
    <PanelRow className="today-row">
      <div className="today-row-text">
        <p className="t-body today-row-title">{itemTitle(item, t)}</p>
        {detail && <p className="t-caption today-row-detail">{detail}</p>}
      </div>
      <div className="today-row-verbs">
        {item.overdue && <Badge tone="danger">{t("day.overdue")}</Badge>}
        {item.actions.includes("complete") && (
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
    </PanelRow>
  );
}

// One lane. `tone="accent"` marks the ONE panel that asks for a move — the
// decisions — so a reader scanning the page finds it before reading a word.
// Two tinted panels would be no lead at all, which is why the other two are
// plain.
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
  tone?: "accent";
  onComplete: (id: string) => void;
  onSnooze: (id: string, dueAt: string) => void;
  completing: boolean;
}>) {
  const Icon = shape.icon;
  return (
    <Panel
      title={
        <span className="today-lane-title">
          <Icon size={16} aria-hidden />
          {shape.title}
        </span>
      }
      titleAction={total > 0 ? <Badge>{total}</Badge> : undefined}
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
  return (
    <Panel
      title={
        <span className="today-lane-title">
          <Sunrise size={16} aria-hidden />
          {t("day.thisMorning")}
        </span>
      }
      titleAction={total > 0 ? <Badge>{total}</Badge> : undefined}
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
            deals={deals.data ?? []}
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
  const onSkip = () =>
    setDeferred((ids) =>
      needsYou[0] && !ids.includes(needsYou[0].id)
        ? [...ids, needsYou[0].id]
        : ids,
    );
  // Deferred items go to the BACK, never away: a reader who put three things
  // off still has three things to do, and a queue that dropped them would
  // report a clear day that is not one.
  const queue = [
    ...needsYou.filter((item) => !deferred.includes(item.id)),
    ...needsYou.filter((item) => deferred.includes(item.id)),
  ];
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
      {(commitments !== undefined || omitted.includes("commitments")) && (
        <Lane
          shape={{
            title: t("day.commitments"),
            empty: t("day.commitments.empty"),
            withheld: t("day.lane.withheld"),
            icon: Handshake,
          }}
          items={commitments ?? []}
          withheld={omitted.includes("commitments")}
          total={day.counts.commitments ?? 0}
          onComplete={onComplete}
          onSnooze={onSnooze}
          completing={complete.isPending}
        />
      )}
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

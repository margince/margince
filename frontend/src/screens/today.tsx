import { CheckSquare, GitMerge, Sparkles } from "lucide-react";
import { useState } from "react";
import { Badge, Button, EmptyState } from "../design-system/atoms";
import { Panel, PanelBody, PanelRow } from "../design-system/panel";
import { SurfaceState } from "../design-system/surfacestate";
import { formatDateTime, formatNumber } from "../format/format";
import { useNow } from "../format/now";
import { viewerZone } from "../format/timezone";
import { type Locale, useLocale, useT } from "../i18n";
import { approvalKindLabel } from "./approvalkind";
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
  if (day.done_for_you && day.done_for_you.length > 0) {
    return t("day.lead.ranOvernight");
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

import { CheckSquare, GitMerge, Layers, Sparkles } from "lucide-react";
import { navigate } from "../app/router";
import { Badge, Button, EmptyState } from "../design-system/atoms";
import { Panel, PanelBody, PanelRow } from "../design-system/panel";
import { SurfaceState } from "../design-system/surfacestate";
import { formatDateTime } from "../format/format";
import { useNow } from "../format/now";
import { viewerZone } from "../format/timezone";
import { type Locale, useLocale, useT } from "../i18n";
import { approvalKindLabel } from "./approvalkind";
import { snoozedDueAt, useTaskUpdate } from "./taskactions";
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
  icon: typeof Layers;
}>;

// The lead line: the largest true thing about the day, written here rather than
// on the server because the server has no language. A reader who reads only
// this line should still know whether to worry.
function leadLine(day: Attention, t: ReturnType<typeof useT>): string {
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
    return t("day.lead.decisions", { count: decisions });
  }
  if (day.counts.planned > 0) {
    return t("day.lead.plannedOnly", { count: day.counts.planned });
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
    return t("day.match", { percent: Math.round(item.confidence * 100) });
  }
  if (item.occurred_at) {
    return formatDateTime(item.occurred_at, locale, zone);
  }
  if (item.due_at) {
    return formatDateTime(item.due_at, locale, zone);
  }
  return null;
}

// Where a card's verb takes the reader.
//
// Every route here belongs to the record that owns the item. This surface holds
// no decision of its own, so a click hands the reader to the place where the
// rules for that decision already live — which is also why a reader who lands
// there sees the same answer they would have reached from the record page.
function openItem(item: AttentionItem): void {
  switch (item.source) {
    case "dedupe_candidate":
      navigate({ screen: "dedupe" });
      return;
    case "task":
      navigate({ screen: "tasks" });
      return;
    default:
      // A receipt is already decided, so it lives on the queue's Decided tab.
      // Sending a reader to Pending to look for it would be sending them to
      // the one list it is guaranteed not to be in.
      navigate({
        screen: "inbox",
        id: item.actions.includes("open") ? "decided" : undefined,
      });
  }
}

// Which word a row's button carries.
//
// Read off the server's own `actions` in order of consequence, never by
// elimination. The first version labelled anything that was not a task
// "Decide", which put a Decide button on every RECEIPT — a finished act
// offering to be decided again, which is the one thing this lane exists not to
// do. It shipped past a test that only checked `actions`, and the screenshot is
// what caught it.
function verbLabel(
  item: AttentionItem,
): "day.merge" | "day.decide" | "day.open" {
  if (item.actions.includes("merge")) {
    return "day.merge";
  }
  if (item.actions.includes("decide")) {
    return "day.decide";
  }
  return "day.open";
}

// One row. The verb it offers is the one the server said it has: a receipt gets
// `open` and nothing else, so a finished act cannot be re-decided from here.
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
  const canComplete = item.actions.includes("complete");
  return (
    <PanelRow className="today-row">
      <div className="today-row-text">
        <p className="t-body today-row-title">{itemTitle(item, t)}</p>
        {detail && <p className="t-caption today-row-detail">{detail}</p>}
      </div>
      <div className="today-row-verbs">
        {item.overdue && <Badge tone="danger">{t("day.overdue")}</Badge>}
        {canComplete ? (
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
        ) : (
          <Button
            small
            variant={item.actions.includes("open") ? undefined : "primary"}
            onClick={() => openItem(item)}
          >
            {t(verbLabel(item))}
          </Button>
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
  const needsYou = day.needs_you ?? [];
  const planned = day.planned ?? [];
  const done = day.done_for_you ?? [];
  const omitted = day.lanes_omitted ?? [];
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
      <p className="t-h2 today-lead">{leadLine(day, t)}</p>
      <Lane
        shape={{
          title: t("day.needsYou"),
          empty: t("day.needsYou.empty"),
          withheld: t("day.lane.withheld"),
          icon: Layers,
        }}
        items={needsYou}
        withheld={omitted.includes("needs_you")}
        total={day.counts.needs_you}
        tone="accent"
        onComplete={onComplete}
        onSnooze={onSnooze}
        completing={complete.isPending}
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
          {t("day.duplicatesOpen", { count: day.counts.duplicates_open })}
        </p>
      )}
    </>
  );
}

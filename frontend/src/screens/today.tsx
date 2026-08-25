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
import { useTaskUpdate } from "./taskactions";
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
  icon: typeof Layers;
}>;

// The lead line: the largest true thing about the day, written here rather than
// on the server because the server has no language. A reader who reads only
// this line should still know whether to worry.
function leadLine(day: Attention, t: ReturnType<typeof useT>): string {
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
      navigate({ screen: "inbox" });
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
  completing,
}: Readonly<{
  item: AttentionItem;
  onComplete: (id: string) => void;
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
          <Button
            small
            onClick={() => onComplete(item.id)}
            disabled={completing}
          >
            {t("day.complete")}
          </Button>
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
  tone,
  onComplete,
  completing,
}: Readonly<{
  shape: LaneShape;
  items: readonly AttentionItem[];
  tone?: "accent";
  onComplete: (id: string) => void;
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
      titleAction={items.length > 0 ? <Badge>{items.length}</Badge> : undefined}
      tone={tone}
    >
      {items.length === 0 ? (
        <PanelBody>
          <EmptyState>{shape.empty}</EmptyState>
        </PanelBody>
      ) : (
        items.map((item) => (
          <AttentionRow
            key={`${item.source}-${item.id}`}
            item={item}
            onComplete={onComplete}
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
  // The SAME mutation the task queue and the record page use, handed the keys
  // this surface needs refreshed. A second PATCH of the same activity would be
  // a second place for a task's write rules to live.
  const complete = useTaskUpdate([attentionKey, ["activities"]]);
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
  return (
    <>
      <p className="t-h2 today-lead">{leadLine(day, t)}</p>
      {omitted.length > 0 && (
        // Said out loud rather than left as a gap. A lane withheld looks
        // exactly like a lane that is empty, and a reader told their day is
        // clear when the server simply could not see it has been misled by
        // the surface they trust most.
        <p className="t-caption today-withheld">{t("day.withheld")}</p>
      )}
      <Lane
        shape={{
          title: t("day.needsYou"),
          empty: t("day.needsYou.empty"),
          icon: Layers,
        }}
        items={needsYou}
        tone="accent"
        onComplete={onComplete}
        completing={complete.isPending}
      />
      <Lane
        shape={{
          title: t("day.planned"),
          empty: t("day.planned.empty"),
          icon: CheckSquare,
        }}
        items={planned}
        onComplete={onComplete}
        completing={complete.isPending}
      />
      <Lane
        shape={{
          title: t("day.done"),
          empty: t("day.done.empty"),
          icon: Sparkles,
        }}
        items={done}
        onComplete={onComplete}
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

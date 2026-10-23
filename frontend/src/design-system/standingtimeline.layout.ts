// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

const DAY_MS = 86_400_000;
const WEEKLY_SPAN_MS = 42 * DAY_MS;

/** Past this share of the axis a label hangs to the left of its dot, so the
 *  last event's words stay on the canvas. */
export const RIGHT_ALIGN_AFTER = 92;

/** Label rows under the spine; a label with no room on any is not drawn. */
export const LABEL_ROWS = 3;

/** The most labels one axis carries; the other events are dots. */
export const MAX_LABELS = 6;

/** Dots closer than this share one label, its most telling event's. */
export const CLUSTER_PX = 40;

/** A mark's chip is this wide with air between neighbours; marks closer than
 *  this share one chip. */
export const MARK_PX = 28;

/** The room kept either side of the Today line for its pill, so no chip sits
 *  under it or on the line. */
export const TODAY_CLEAR_PX = 44;

// The width of one character of a label's title and of its sub-line, and
// the room kept clear between two labels on a row.
const TITLE_CHAR_PX = 7.4;
const SUB_CHAR_PX = 6.4;
const LABEL_GAP_PX = 16;

/** The widest a label's words run, in pixels; a longer title or sub-line is
 *  cut with an ellipsis, so one long name cannot take a row to itself. */
export const LABEL_MAX_PX = 224;
// A lead line's own clearance from a label it must not cross.
const LEAD_CLEAR_PX = 6;
// The most arrangements one layout weighs.
const SEARCH_BUDGET = 4000;

export type TimelineRange = Readonly<{ start: number; end: number }>;

export type TimelineTick = Readonly<{ at: number; pct: number }>;

/** Milliseconds since the epoch for an ISO date or instant; a date alone is
 *  read as its UTC midnight. */
export function instant(iso: string): number {
  return Date.parse(iso.length === 10 ? `${iso}T00:00:00Z` : iso);
}

/**
 * The span the axis covers. It runs from the first of the month the earliest
 * moment falls in to the last day of the month the latest falls in. The month
 * ticks then start and end the axis. A span under six weeks is cut to whole weeks
 * instead, since month ticks on one month say nothing.
 */
export function rangeOf(moments: readonly number[]): TimelineRange {
  const start = monthStart(Math.min(...moments));
  const last = Math.max(...moments);
  if (last - start < WEEKLY_SPAN_MS) {
    const weeks = Math.max(
      Math.ceil((last - start + DAY_MS) / (7 * DAY_MS)),
      4,
    );
    return { start, end: start + weeks * 7 * DAY_MS };
  }
  const after = new Date(monthStart(last));
  after.setUTCMonth(after.getUTCMonth() + 1);
  return { start, end: after.getTime() - DAY_MS };
}

/**
 * The axis for a record read at `today`: every event, and room after today
 * (a week, or a sixth of the time before it). The Today pill then never
 * stands on the axis's end.
 */
export function axisOf(
  today: number,
  moments: readonly number[],
): TimelineRange {
  const first = Math.min(today, ...moments);
  const room = Math.max(7 * DAY_MS, (today - first) / 6);
  return rangeOf([today, today + room, ...moments]);
}

/** Where a moment sits on the axis, as a percentage held to 0–100. */
export function pct(at: number, range: TimelineRange): number {
  const span = range.end - range.start;
  if (span <= 0) {
    return 0;
  }
  const share = ((at - range.start) / span) * 100;
  return Math.round(Math.min(Math.max(share, 0), 100) * 100) / 100;
}

/**
 * The grid lines: every month start inside the range, every other one past
 * seven months, or every seventh day on a range of weeks.
 */
export function ticksOf(range: TimelineRange): TimelineTick[] {
  const ticks: TimelineTick[] = [];
  if (isWeekly(range)) {
    for (let at = range.start; at <= range.end; at += 7 * DAY_MS) {
      ticks.push({ at, pct: pct(at, range) });
    }
    return ticks;
  }
  const step = range.end - range.start > 7 * 31 * DAY_MS ? 2 : 1;
  const cursor = new Date(monthStart(range.start));
  while (cursor.getTime() <= range.end) {
    ticks.push({ at: cursor.getTime(), pct: pct(cursor.getTime(), range) });
    cursor.setUTCMonth(cursor.getUTCMonth() + step);
  }
  return ticks;
}

const TICK_CHAR_PX = 6.4;
// A tick label stands this far from its line.
const TICK_INSET_PX = 8;

/**
 * Which grid-line labels are drawn: a label whose words would run into the
 * one drawn before it is left off, and its line stays. The first label in
 * axis order wins, so the axis's end (which hangs left of its line) is the
 * one that gives way.
 */
export function tickLabelsShown(
  ticks: readonly Readonly<{ pct: number; chars: number }>[],
  canvasPx: number,
): boolean[] {
  let lastEnd = Number.NEGATIVE_INFINITY;
  return ticks.map((tick) => {
    const at = (tick.pct / 100) * canvasPx;
    const width = tick.chars * TICK_CHAR_PX;
    const span: Span =
      tick.pct > RIGHT_ALIGN_AFTER
        ? [at - TICK_INSET_PX - width, at - TICK_INSET_PX]
        : [at + TICK_INSET_PX, at + TICK_INSET_PX + width];
    if (span[0] < lastEnd + TICK_INSET_PX) {
      return false;
    }
    lastEnd = span[1];
    return true;
  });
}

/** Whether the axis counts weeks rather than months. */
export function isWeekly(range: TimelineRange): boolean {
  return range.end - range.start <= WEEKLY_SPAN_MS;
}

/** Which events a reader most needs named, most first. */
const KIND_RANK: Readonly<Record<TimelineKind, number>> = {
  late: 0,
  last: 1,
  ahead: 2,
  end: 3,
  done: 4,
};

export type TimelineKind = "done" | "late" | "last" | "ahead" | "end";

export type TimelineItem = Readonly<{ pct: number; kind: TimelineKind }>;

/**
 * Dots that sit closer than `CLUSTER_PX` on a canvas `canvasPx` wide, as one
 * group each, in axis order. `lead` is the member a reader most needs named.
 * That is an overdue promise first, then the last contact, the next date
 * ahead, the horizon, and of the rest the latest.
 */
export function clusterEvents(
  items: readonly TimelineItem[],
  canvasPx: number,
): { lead: number; members: number[] }[] {
  const reach = (CLUSTER_PX / canvasPx) * 100;
  const groups: [number, ...number[]][] = [];
  items.forEach((item, index) => {
    const open = groups.at(-1);
    if (open && item.pct - (items[open[0]]?.pct ?? 0) <= reach) {
      open.push(index);
    } else {
      groups.push([index]);
    }
  });
  return groups.map((members) => {
    const [first, ...rest] = members;
    return {
      members,
      lead: rest.reduce(
        (best, index) => (outranks(items, index, best) ? index : best),
        first,
      ),
    };
  });
}

function outranks(
  items: readonly TimelineItem[],
  a: number,
  b: number,
): boolean {
  const kindA = items[a]?.kind ?? "done";
  const kindB = items[b]?.kind ?? "done";
  if (KIND_RANK[kindA] !== KIND_RANK[kindB]) {
    return KIND_RANK[kindA] < KIND_RANK[kindB];
  }
  // Ahead, the nearest date matters most; behind, the latest.
  return kindA === "ahead" ? a < b : a > b;
}

/** One label to place: its dot's share of the axis, its kind, and the length
 *  of its two lines in characters. */
export type LabelBox = Readonly<{
  pct: number;
  kind: TimelineKind;
  titleChars: number;
  subChars: number;
}>;

/** A label's row under the spine and whether it hangs left of its dot, or
 *  `null` for a label left off the axis, whose event stays a dot. */
export type LabelPlace = Readonly<{ row: number; hangsLeft: boolean }> | null;

type Span = readonly [number, number];

/** A label's width in pixels at the label type. */
export function labelWidthPx(label: LabelBox): number {
  const words = Math.max(
    label.titleChars * TITLE_CHAR_PX,
    label.subChars * SUB_CHAR_PX,
  );
  return Math.min(words, LABEL_MAX_PX) + LABEL_GAP_PX;
}

/** Where a label's words run, in pixels, reading right from its dot or
 *  hanging left of it. */
export function labelSpan(
  label: LabelBox,
  canvasPx: number,
  hangsLeft: boolean,
): Span {
  const at = (label.pct / 100) * canvasPx;
  const width = labelWidthPx(label);
  return hangsLeft ? [at - width, at] : [at, at + width];
}

type Option = Readonly<{ row: number; hangsLeft: boolean; span: Span }>;

/**
 * Where each label goes, labels in axis order. The `MAX_LABELS` a reader
 * most needs are the candidates. Each reads right from its dot, or hangs left
 * of it past `RIGHT_ALIGN_AFTER`, and either way stays on the canvas.
 * It takes one of `LABEL_ROWS` rows where it overlaps no label. Its lead line
 * crosses no label above it, and no lead line from below crosses it.
 * Of the arrangements that hold, the one that names the most telling events
 * wins. Ties go to the fewest labels turned against their reading side, then
 * the highest rows. A label with no room is left off.
 */
export function placeLabels(
  labels: readonly LabelBox[],
  canvasPx: number,
): LabelPlace[] {
  const order = labels
    .map((_, index) => index)
    .sort((a, b) =>
      outranks(labels, a, b) ? -1 : outranks(labels, b, a) ? 1 : 0,
    )
    .slice(0, MAX_LABELS);
  const options = order.map((index) => optionsFor(labels[index], canvasPx));
  // A placed label outweighs every label ranked under it put together.
  const weights = order.map((_, rank) => 2 ** (order.length - rank) * 1000);
  let best: { score: number; chosen: (Option | null)[] } = {
    score: -1,
    chosen: [],
  };
  const chosen: (Option | null)[] = [];
  // The search tries the reading side and the top row first, so the first
  // arrangements it meets are good ones. The budget keeps a crowded axis from
  // costing a frame.
  let budget = SEARCH_BUDGET;
  const search = (rank: number, score: number): void => {
    budget -= 1;
    if (budget < 0) {
      return;
    }
    if (rank === order.length) {
      if (score > best.score) {
        best = { score, chosen: [...chosen] };
      }
      return;
    }
    const ceiling = weights.slice(rank).reduce((sum, w) => sum + w, score);
    if (ceiling <= best.score) {
      return;
    }
    const at = ((labels[order[rank] ?? 0]?.pct ?? 0) / 100) * canvasPx;
    (options[rank] ?? []).forEach((option, cost) => {
      if (fits(chosen, labels, order, canvasPx, option, at)) {
        chosen.push(option);
        search(rank + 1, score + (weights[rank] ?? 0) - cost);
        chosen.pop();
      }
    });
    chosen.push(null);
    search(rank + 1, score);
    chosen.pop();
  };
  search(0, 0);
  const places: LabelPlace[] = labels.map(() => null);
  order.forEach((index, rank) => {
    const option = best.chosen[rank];
    places[index] = option
      ? { row: option.row, hangsLeft: option.hangsLeft }
      : null;
  });
  return places;
}

// A label's rows and sides, its reading side first, each kept on the canvas.
function optionsFor(label: LabelBox | undefined, canvasPx: number): Option[] {
  if (!label) {
    return [];
  }
  const reads = label.pct > RIGHT_ALIGN_AFTER;
  const sides = [reads, !reads].filter((hangsLeft) => {
    const span = labelSpan(label, canvasPx, hangsLeft);
    return span[0] >= -LABEL_GAP_PX && span[1] <= canvasPx + LABEL_GAP_PX;
  });
  return Array.from({ length: LABEL_ROWS }, (_, row) => row).flatMap((row) =>
    sides.map((hangsLeft) => ({
      row,
      hangsLeft,
      span: labelSpan(label, canvasPx, hangsLeft),
    })),
  );
}

function fits(
  chosen: readonly (Option | null)[],
  labels: readonly LabelBox[],
  order: readonly number[],
  canvasPx: number,
  option: Option,
  at: number,
): boolean {
  const lead: Span = [at - LEAD_CLEAR_PX, at + LEAD_CLEAR_PX];
  return chosen.every((other, rank) => {
    if (!other) {
      return true;
    }
    const otherAt = ((labels[order[rank] ?? 0]?.pct ?? 0) / 100) * canvasPx;
    const otherLead: Span = [otherAt - LEAD_CLEAR_PX, otherAt + LEAD_CLEAR_PX];
    return (
      // Two labels on one row.
      (other.row !== option.row || !overlaps(other.span, option.span)) &&
      // This label's lead line through a label above it.
      (other.row >= option.row || !overlaps(other.span, lead)) &&
      // A lower label's lead line through this label.
      (other.row <= option.row || !overlaps(option.span, otherLead))
    );
  });
}

function overlaps(a: Span, b: Span): boolean {
  return a[0] < b[1] && b[0] < a[1];
}

function monthStart(at: number): number {
  const day = new Date(at);
  return Date.UTC(day.getUTCFullYear(), day.getUTCMonth(), 1);
}

/** One chip on the mark lane: where it stands and which marks it holds, oldest
 *  first. */
export type MarkGroup = Readonly<{ px: number; members: number[] }>;

/**
 * Where the chips on the mark lane go, for marks given oldest first at `pcts`
 * (shares of the axis) on a canvas `canvasPx` wide with Today at `nowPct`.
 *
 * A chip never reaches past `TODAY_CLEAR_PX` of Today or over the axis's left
 * end. A mark that close to Today is held back to that line rather than drawn
 * under the pill. Marks held to one position, or closer than `MARK_PX` to the
 * group's first mark, share one chip. It stands where its oldest member does.
 * Every gap between two chips is at least `MARK_PX`.
 */
export function groupMarks(
  pcts: readonly number[],
  canvasPx: number,
  nowPct: number,
): MarkGroup[] {
  const lowest = MARK_PX / 2;
  const highest = Math.max(
    lowest,
    Math.min((nowPct / 100) * canvasPx - TODAY_CLEAR_PX, canvasPx - lowest),
  );
  const groups: { px: number; members: number[] }[] = [];
  pcts.forEach((share, index) => {
    const px = Math.min(Math.max((share / 100) * canvasPx, lowest), highest);
    const open = groups.at(-1);
    if (open && px - open.px < MARK_PX) {
      open.members.push(index);
    } else {
      groups.push({ px, members: [index] });
    }
  });
  return groups;
}

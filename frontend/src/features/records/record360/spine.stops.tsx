import { formatDayMonth } from "../../../format/format";
import type { SpineSource, Stop } from "./spine";
import { type Ctx, on, pastStops, silence } from "./spine.exchanges";

// The narrowest a stop may be drawn. A date is set on one line (broken across
// two, it reads as two moments). Any narrower and the dates run into one smear
// of digits. At the floor the track outgrows its card and the thread scrolls,
// as it does with more history than fits. Crushing the stops is not allowed.
const STOP_FLOOR = "6.5rem";

// How much of the axis a stop takes. The gap takes half again the stops either
// side of it, because on a horizontal axis the width is the waiting. Today's
// marker takes only its own label: it is a line, not a span of time.
export function todayColumn(stop: Stop): string {
  if (stop.tone === "now") {
    // Its label's width and not a share of the axis. `auto` reads the stop's
    // minimum contribution, which every stop declares as zero so a long subject
    // cannot widen the thread. With `auto`, a track wider than its card would
    // collapse the marker, and today would vanish from the axis it divides.
    return "max-content";
  }
  return stop.tone === "gap"
    ? `minmax(${STOP_FLOOR}, 1.5fr)`
    : `minmax(${STOP_FLOOR}, 1fr)`;
}

// The thread, oldest first: the conversations that have happened, the silence
// since the last of them, then what is dated ahead.
//
// One stop per conversation, not per message and not per row of the timeline.
// A reader arriving at a record they do not remember asks what was going on.
// One stop naming the newest email would answer with one line out of a story.
// Grouping keeps this a thread rather than the history tab. Six emails in two
// conversations are two stops, and the tab is where one message is read.
export function spineStops(view: SpineSource, ctx: Ctx): Stop[] {
  const stops: Stop[] = [...pastStops(view, ctx)];
  const waiting = silence(view, ctx);
  if (waiting) {
    stops.push(waiting);
  }
  for (const ahead of aheadStops(view, ctx)) {
    stops.push(ahead);
  }
  return withToday(stops, ctx);
}

// Today, marked on the axis.
//
// Not a stop: nothing happened today, and a stop would put an event on the
// thread that the record does not contain. It is a line through the axis.
// Everything left of it happened, and everything right of it is owed.
//
// It goes before the first stop that has not happened yet. A slipped
// commitment is dated in the past, so it stays on the left where its date puts
// it. With nothing ahead, the line closes the axis: the record runs up to
// today.
function withToday(stops: readonly Stop[], ctx: Ctx): Stop[] {
  if (stops.length === 0) {
    return [...stops];
  }
  const first = stops.findIndex((stop) => stop.tone === "ahead");
  const marker: Stop = {
    key: "today",
    tone: "now",
    // No date on the axis's own date row: the marker is not a moment the
    // record reached, it is where the reader is standing. Its word and its
    // date sit under the line instead, out of the row the record's own dates
    // share.
    when: "",
    title: (
      <>
        <span className="co-spine-now-word">{ctx.t("co.spine.today")}</span>
        {/* Day and month, no year. The marker names today, and the year of
            today is the one date on the axis a reader already knows. */}
        <span>{formatDayMonth(ctx.asOf, ctx.locale, ctx.zone)}</span>
      </>
    ),
  };
  if (first === -1) {
    return [...stops, marker];
  }
  return [...stops.slice(0, first), marker, ...stops.slice(first)];
}

// What is dated ahead: the commitments that carry a due date, then the money.
//
// A task already past its date is drawn as overdue rather than ahead. It is
// late, not coming, and sorting it into the future would hide it.
function aheadStops(view: SpineSource, ctx: Ctx): Stop[] {
  const stops: Stop[] = [];
  for (const step of (view.next_steps?.data ?? []).slice(0, 2)) {
    if (!step.due_at) {
      continue;
    }
    stops.push({
      key: `step-${step.activity_id}`,
      tone: step.overdue ? "overdue" : "ahead",
      when: on(step.due_at, ctx),
      title: step.subject,
      detail: step.overdue ? ctx.t("co.spine.overdue") : undefined,
    });
  }
  const close = ctx.commercial?.next_close_on;
  if (close) {
    stops.push({
      key: "close",
      tone: "ahead",
      when: on(close, ctx),
      title: ctx.t("co.spine.expectedClose"),
    });
  }
  return stops;
}

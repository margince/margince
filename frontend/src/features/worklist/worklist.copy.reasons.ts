import {
  activityTimeOfDay,
  formatActivityWhen,
  type OccurredPrecision,
} from "../../format/activitywhen";
import { middayInstant, sameCalendarDay } from "../../format/calendarday";
import {
  formatDate,
  formatDateTime,
  formatMoney,
  formatNumber,
} from "../../format/format";
import type { Locale } from "../../i18n";
import { translatePlural } from "../../i18n";
import type { T } from "./worklist.copy";
import { scheduledText } from "./worklist.held";
import type {
  WorklistItem,
  WorklistReason,
  WorklistValue,
} from "./worklist.queries";
import { daysValued, known } from "./worklist.reasoncensus";

// One comparator value, in the reader's notation.
//
// A magnitude in the wrong notation is a different number to the reader. A
// German reader reads "1.234", and the coerced form says something else.
function valueText(
  value: WorklistValue | undefined,
  locale: Locale,
  zone: string,
): string | null {
  if (!value) {
    return null;
  }
  switch (value.kind) {
    case "date":
      return value.date ? formatDateTime(value.date, locale, zone) : null;
    case "money":
      // Both halves or nothing. A euro sign on a figure that might be dong is
      // worse than no figure, and the scale is wrong without the currency. So
      // a value that names no currency says nothing at all.
      return value.minor == null || !value.currency
        ? null
        : formatMoney(value.minor, value.currency, locale);
    case "days":
    case "count": {
      const figure = value.days ?? value.count;
      return figure == null ? null : formatNumber(figure, locale);
    }
    case "level":
      return value.level == null ? null : formatNumber(value.level, locale);
    default:
      return null;
  }
}

// Reasons that read with an amount or a moment, needing no plural rule (day
// counts are the census's DAYS_VALUED_REASONS). A set, since a key composed for
// a reason whose sentence has nowhere to put an arrived value would not exist.
const VALUED_REASONS = {
  expected_revenue: true,
  material: true,
  below_material: true,
  // Moments, not figures: valueText renders a date in the reader's zone.
  response_due_soon: true,
  first_asked: true,
  meeting_booked: true,
} as const;

type ValuedReason = keyof typeof VALUED_REASONS;

function valued(kind: WorklistReason["kind"]): kind is ValuedReason {
  return kind in VALUED_REASONS;
}

/**
 * The reason kind that reads as a badge, not as a phrase in the line.
 *
 * "Not confirmed ready" is a state of the meeting, as `overdue` is of a deadline,
 * and not one weighed fact among several. In a `because` line it reads as
 * background; a rep scanning a rail of meetings needs it at a glance.
 *
 * So it is drawn once, as a badge, and left out of the phrase line. Said in
 * both places it would read as two separate findings about one meeting.
 */
const BADGED = "meeting_unprepared";

/** Whether the reader attends this meeting and has not confirmed they are ready. */
export function isUnprepared(item: WorklistItem): boolean {
  return (
    item.source === "meeting" && item.preparation?.state === "needs_review"
  );
}

/**
 * The reason kinds the when line already says.
 *
 * Each is the moment in a coarser register. Said with it, a task row reads "due
 * 06.07.2026, 15:00" over "due today", and an overdue one repeats its badge. A
 * meeting reads "starts 14:12" over "starting shortly". One clock, twice, reads
 * as two findings. The moment wins: it names the hour a rep is racing.
 *
 * A set, not one kind, because the duplication is structural. `overdue` and
 * `due_today` are the two arms of one `if` in the task lane. Both are guarded by
 * the `due_at` the moment is drawn from, so naming one names half a condition.
 * `meeting_soon` fires only where the meeting has a `due_at`, which is when its
 * when line is drawn, so it never adds anything.
 *
 * The other deadline reasons stay out of this set: `closing_soon`,
 * `response_overdue` and `response_due_soon`. Their sources (`deal_at_risk`,
 * `brief_item`, `lead_response`) get null from `whenKeyFor`. Nothing draws their
 * moment, so their phrase is the only place the fact is said.
 *
 * They drop only where the moment is drawn. A row whose `due_at` the when line
 * refuses still says its phrase, or the row would lose the fact. An approval's
 * lapse instant is such a `due_at`: a fact about staged work, not a rep's debt.
 */
const SAID_BY_THE_WHEN_LINE = new Set<string>([
  "due_today",
  "overdue",
  "meeting_soon",
]);

/** The reasons a row says in its phrase line: all but those said elsewhere. */
export function phrasedReasons(
  item: WorklistItem,
  whenDrawn: boolean,
): WorklistReason[] {
  return item.because.filter(
    (reason) =>
      reason.kind !== BADGED &&
      !["material", "below_material", "routine", "pinned"].includes(
        reason.kind,
      ) &&
      !(
        reason.kind === "expected_revenue" &&
        item.deal?.amount_minor != null &&
        item.deal.currency
      ) &&
      !(whenDrawn && SAID_BY_THE_WHEN_LINE.has(reason.kind)),
  );
}

// One fact behind an item's rank.
export function reasonText(
  reason: WorklistReason,
  t: T,
  locale: Locale,
  zone: string,
): string | null {
  if (!known(reason.kind)) {
    // A reason this build has no sentence for is DROPPED, not printed as its
    // own key. The row keeps its other reasons and says one thing less.
    return null;
  }
  const value = valueText(reason.value, locale, zone);
  if (
    value !== null &&
    daysValued(reason.kind) &&
    (reason.value?.days ?? reason.value?.count) != null
  ) {
    const base = `worklist.because.${reason.kind}.value` as const;
    const figure = reason.value?.days ?? reason.value?.count ?? 0;
    return translatePlural(locale, base, figure, { value });
  }
  if (value !== null && valued(reason.kind)) {
    return t(`worklist.because.${reason.kind}.value` as const, { value });
  }
  return t(`worklist.because.${reason.kind}` as const);
}

// The deal's own figures, in one line: what it is worth, and when it closes.
//
// The money is on the wire. Without this line, a €160,100 deal reads only as
// "no contact for 83 days".
export function dealFactsText(
  item: WorklistItem,
  t: T,
  locale: Locale,
  zone: string,
): string | null {
  const deal = item.deal;
  if (!deal) {
    return null;
  }
  const parts: string[] = [];
  if (deal.amount_minor != null && deal.currency) {
    parts.push(formatMoney(deal.amount_minor, deal.currency, locale));
  }
  if (deal.expected_close_date) {
    parts.push(
      t(
        deal.close_date_provisional
          ? "worklist.deal.provisional"
          : "worklist.deal.closes",
        {
          date: formatDate(
            middayInstant(deal.expected_close_date, zone),
            locale,
            zone,
          ),
        },
      ),
    );
  }
  if (deal.forecast_category === "omitted")
    parts.push(t("worklist.deal.omitted"));
  return parts.length > 0 ? parts.join(" · ") : null;
}

// Notices retain the original change date even when delivery happens later.
// A meeting's start uses the reader's clock; a held meeting and a task, the record's.
//
// Today's meeting shows the clock time alone: a rep reads it on that morning,
// and "today" is the frame they are in. Otherwise the date shows too, or a bare
// "09:00" two days out reads as this morning.
// The two moments use different clocks because they are different facts. A
// meeting starts at an instant the reader attends, so it uses their clock. A
// 09:00 Berlin call is 14:00 in Bangkok; saying 09:00 sends them five hours
// late. A task is due on a calendar day the team agreed, so it uses the
// record's clock. Two colleagues quoting one deadline quote the same day, as
// `dueInstant` mints it.
export function whenText(
  item: WorklistItem,
  t: T,
  locale: Locale,
  viewer: string,
  record: string,
  now: Date,
): string | null {
  if (item.source === "notice" && item.notice_origin)
    return formatDateTime(item.notice_origin.occurred_at, locale, viewer);
  const held = scheduledText(item, t, locale, record);
  if (held) return held;
  if (!item.due_at) {
    return item.source === "task" ? t("brief.task.undated") : null;
  }
  if (item.source === "weekly_commitment")
    return t("worklist.when.due", {
      when: formatDate(item.due_at, locale, record),
    });
  const key = whenKeyFor(item);
  if (key === null) {
    return null;
  }
  const zone = item.source === "task" ? record : viewer;
  // A meeting's `due_at` is its start, as exact as the meeting is; any other
  // row's is a deadline, which always has its time.
  const when =
    item.source === "meeting"
      ? momentText(item.due_at, item.occurred_precision, locale, zone, now)
      : momentText(item.due_at, "instant", locale, zone, now);
  return t(key, { when });
}

// Which sentence the moment goes in, or null for a row whose `due_at` is not a
// clock the reader is racing. An approval's `due_at` is when the proposal
// lapses (the contract says so). Drawn as "due", "this offer goes stale" reads
// as "you are late", which is untrue about the reader's own day.
function whenKeyFor(
  item: WorklistItem,
): "worklist.when.starts" | "worklist.when.due" | null {
  if (item.source === "meeting") {
    return "worklist.when.starts";
  }
  if (
    [
      "task",
      "conversation_claim",
      "weekly_commitment",
      "dsr",
      "notice_case",
    ].includes(item.source)
  ) {
    return "worklist.when.due";
  }
  return null;
}

// The moment itself: the time alone when it falls today, the date and time
// otherwise, and the date alone when no time is known.
function momentText(
  dueAt: string,
  precision: OccurredPrecision,
  locale: Locale,
  zone: string,
  now: Date,
): string {
  const clock = sameCalendarDay(new Date(dueAt), now, zone)
    ? activityTimeOfDay(dueAt, precision, locale, zone)
    : null;
  return clock ?? formatActivityWhen(dueAt, precision, locale, zone);
}

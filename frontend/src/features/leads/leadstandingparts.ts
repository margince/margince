// The readings of a lead's How it stands card, as plain data. They are the date under
// each rung, the score and its two rows, the figures strip and the next move.
// Every reading takes the page's one `asOf`, so a card read at one instant
// says one thing, and a test can fix that instant.

import type { components } from "../../api/schema";
import type { PeekContent } from "../../design-system/peek";
import type { ReadingTone } from "../../design-system/readingtone";
import type { StandingDim } from "../../design-system/standingdims";
import {
  type ActivityMoment,
  activityDay,
  activityDaysAway,
  activityHasBegun,
  activityIsAhead,
  type OccurredPrecision,
} from "../../format/activitywhen";
import {
  daysAgoText,
  formatDayMonth,
  formatDuration,
  formatNumber,
  formatSignedNumber,
} from "../../format/format";
import { daysPast } from "../../format/lateness";
import type { Locale, Translator } from "../../i18n";
import { routeHash } from "../../lib/router";
import { movesLastActivity } from "./lastactivity";
import { promoteEligible, scoreFactorLabel } from "./leadpresentation";
import { scoreTone } from "./leadpresentation.board";
import type { LadderOpenStep } from "./leads.stepper";
import { firstResponseClock } from "./leadstanding";

type Lead = components["schemas"]["Lead"];
type Activity = components["schemas"]["Activity"];
type ScoreFactor = components["schemas"]["LeadScoreFactor"];
type StatusChange = Pick<
  components["schemas"]["FieldHistoryEntry"],
  "new_value" | "changed_at"
>;

export type ReadingContext = Readonly<{
  t: Translator;
  locale: Locale;
  zone: string;
  // The record zone, which a date-only activity is read in.
  record: string;
  // The instant the page read the lead at.
  asOf: string;
}>;

const LADDER: readonly LadderOpenStep[] = ["new", "contacted", "engaged"];

// The factor names the server's lead scorer writes (`GET /leads/{id}/score`):
// who the lead is, and what they did. Manual signals are neither.
const FIT_FACTORS = new Set([
  "decision_maker_title",
  "high_intent_source",
  "low_intent_source",
]);
const ENGAGEMENT_FACTORS = new Set(["reply", "meeting_held", "meeting_booked"]);

export function isFitFactor(factor: string): boolean {
  return FIT_FACTORS.has(factor);
}

export function isEngagementFactor(factor: string): boolean {
  return ENGAGEMENT_FACTORS.has(factor);
}

/**
 * The day the lead entered each rung it has reached: New on its creation,
 * the others on the newest status change into them. A rung with no recorded
 * change gets no date rather than a guess. The engaged rung falls back on the
 * signal the system moved it on, which is that same moment.
 */
export function ladderDates(
  lead: Lead,
  changes: readonly StatusChange[],
  ctx: ReadingContext,
): Partial<Record<LadderOpenStep, string>> {
  const reached = LADDER.indexOf(lead.status as LadderOpenStep);
  const top = reached < 0 ? LADDER.length - 1 : reached;
  const dates: Partial<Record<LadderOpenStep, string>> = {};
  LADDER.slice(0, top + 1).forEach((step) => {
    const entered = enteredAt(lead, step, changes, ctx);
    if (!entered) {
      return;
    }
    const day = activityDay(
      entered.at,
      entered.precision,
      ctx.locale,
      entered.zone,
      formatDayMonth,
    );
    dates[step] =
      step === lead.status ? ctx.t("lead.ladder.enteredNow", { at: day }) : day;
  });
  return dates;
}

// A rung's date, the zone it is read in and its precision. The signal that
// engaged the lead is read on the record's clock, a day entry by its own date.
function enteredAt(
  lead: Lead,
  step: LadderOpenStep,
  changes: readonly StatusChange[],
  { zone, record }: ReadingContext,
):
  | Readonly<{ at: string; zone: string; precision: OccurredPrecision }>
  | undefined {
  if (step === "new") {
    return { at: lead.created_at, zone, precision: "instant" };
  }
  const newest = changes
    .filter((change) => change.new_value === step)
    .sort((a, b) => Date.parse(b.changed_at) - Date.parse(a.changed_at))[0];
  if (newest) {
    return { at: newest.changed_at, zone, precision: "instant" };
  }
  const evidence = lead.qualification_evidence;
  const signal = evidence?.occurred_at;
  return step === "engaged" && lead.status_set_by === "system" && signal
    ? {
        at: signal,
        zone: record,
        precision: evidence?.occurred_precision ?? "instant",
      }
    : undefined;
}

/**
 * The score the ring draws, and the card its word opens. Every signal is
 * listed with the points it put on the score, and who set it when a user did.
 */
export function leadScoreReading(
  lead: Lead,
  factors: readonly ScoreFactor[] | undefined,
  ctx: ReadingContext,
): Readonly<{
  score: number;
  label: string;
  tone: ReadingTone;
  peek: PeekContent;
}> {
  const { t, locale } = ctx;
  const label = t("standing.ring.score", {
    score: formatNumber(lead.score, locale),
  });
  const reason = lead.score_override_reason;
  return {
    score: lead.score,
    label,
    tone: scoreTone(lead.score) ?? "neutral",
    peek: {
      head: label,
      items: (factors ?? []).map((factor) => ({
        key: factor.factor,
        title: scoreFactorLabel(factor.factor, t),
        right: formatSignedNumber(Math.round(factor.points), locale),
      })),
      foot: reason ? t("lead.score.setByHand", { reason }) : undefined,
    },
  };
}

/**
 * Fit and Engagement beside the score ring: the score's own factors summed by
 * kind, so the rows explain the number rather than add to it. `factors` is
 * undefined while the breakdown is not in hand.
 */
export function leadDims(
  factors: readonly ScoreFactor[] | undefined,
  ctx: ReadingContext,
): StandingDim[] {
  const { t } = ctx;
  return [
    factorDim(
      "fit",
      t("lead.dims.fit"),
      t("lead.dims.fit.peek"),
      factors,
      isFitFactor,
      ctx,
    ),
    factorDim(
      "engagement",
      t("lead.dims.engagement"),
      t("lead.dims.engagement.peek"),
      factors,
      isEngagementFactor,
      ctx,
    ),
  ];
}

function factorDim(
  key: string,
  label: string,
  peekHead: string,
  factors: readonly ScoreFactor[] | undefined,
  belongs: (factor: string) => boolean,
  ctx: ReadingContext,
): StandingDim {
  const { t, locale } = ctx;
  if (!factors) {
    return { key, label, value: t("lead.dims.notRead"), tone: "neutral" };
  }
  const counted = factors.filter((factor) => belongs(factor.factor));
  if (counted.length === 0) {
    return { key, label, value: t("lead.dims.noneCounted"), tone: "neutral" };
  }
  const points = Math.round(
    counted.reduce((sum, factor) => sum + factor.points, 0),
  );
  return {
    key,
    label,
    value: formatSignedNumber(points, locale),
    sub: counted.map((factor) => scoreFactorLabel(factor.factor, t)).join(", "),
    tone: points > 0 ? "success" : points < 0 ? "warning" : "neutral",
    // Each signal with the points it put on the score, so the figure above is
    // the rows below added up.
    peek: {
      head: peekHead,
      items: counted.map((factor) => ({
        key: factor.factor,
        title: scoreFactorLabel(factor.factor, t),
        right: formatSignedNumber(Math.round(factor.points), locale),
      })),
    },
  };
}

export type LeadFigure = Readonly<{
  key: string;
  label: string;
  // Never empty: a figure the lead does not have says so in words.
  value: string;
  detail?: string;
  tone?: ReadingTone;
  // The record the figure names, when it names one.
  href?: string;
}>;

// The company figure opens the company once a human linked it, and says it is
// not linked while the name is only typed.
function companyFigureLink(
  lead: Lead,
  t: Translator,
): Pick<LeadFigure, "detail" | "href"> {
  if (lead.company_id) {
    return {
      detail: t("lead.figure.linked"),
      href: routeHash({ screen: "companies", id: lead.company_id }),
    };
  }
  return lead.company_name?.trim()
    ? { detail: t("lead.figure.notLinked") }
    : {};
}

/** The four cells under the timeline: first response, last touch, the next
 *  booked meeting, and the company the lead named. */
export function leadFigures(
  lead: Lead,
  activities: readonly Activity[],
  ctx: ReadingContext,
  // True once the page holds the lead's whole history.
  wholeHistory = false,
  // The linked company's own name, which a rename may have moved from the typed one.
  companyName: string | null | undefined = lead.company_name,
): LeadFigure[] {
  const { t } = ctx;
  return [
    firstResponseFigure(lead, ctx),
    lastTouchFigure(
      lastTouchAt(lead, activities, ctx.asOf, ctx.record, wholeHistory),
      ctx,
    ),
    meetingFigure(nextBookedMeeting(activities, ctx.asOf, ctx.record), ctx),
    {
      key: "company",
      label: t("lead.readings.company"),
      value: companyName?.trim() || t("lead.readings.noCompany"),
      ...companyFigureLink(lead, t),
    },
  ];
}

/**
 * When the lead was last touched, as of `asOf`: the newest row that moves last
 * activity by then. The stored field stands in only for a cut page with no
 * such row. It names no kind, so it stands once it has begun in `record`.
 */
function lastTouchAt(
  lead: Lead,
  activities: readonly Activity[],
  asOf: string,
  record: string,
  wholeHistory: boolean,
): ActivityMoment | undefined {
  const now = Date.parse(asOf);
  const happened = activities
    .filter((row) => movesLastActivity(row, now, record))
    .sort((a, b) => Date.parse(b.occurred_at) - Date.parse(a.occurred_at))[0];
  if (happened) {
    return { at: happened.occurred_at, precision: happened.occurred_precision };
  }
  const stored = lead.last_activity_at;
  const precision = lead.last_activity_precision ?? "instant";
  return !wholeHistory &&
    stored &&
    activityHasBegun(stored, precision, now, record)
    ? { at: stored, precision }
    : undefined;
}

function lastTouchFigure(
  touch: ActivityMoment | undefined,
  ctx: ReadingContext,
): LeadFigure {
  const { t, locale, zone, record } = ctx;
  if (touch === undefined) {
    return {
      key: "last-touch",
      label: t("lead.figure.lastTouch"),
      value: t("standing.noneYet"),
    };
  }
  const { at, precision } = touch;
  const now = Date.parse(ctx.asOf);
  return {
    key: "last-touch",
    label: t("lead.figure.lastTouch"),
    value: activityDay(at, precision, locale, zone, formatDayMonth),
    detail: daysAgoText(
      -activityDaysAway(at, precision, now, zone, record),
      t,
      locale,
    ),
  };
}

function firstResponseFigure(lead: Lead, ctx: ReadingContext): LeadFigure {
  const { t, locale, zone } = ctx;
  const label = t("lead.readings.firstResponse");
  if (lead.first_response_at) {
    const from = Date.parse(lead.routed_at ?? lead.created_at);
    const span = Date.parse(lead.first_response_at) - from;
    return {
      key: "first-response",
      label,
      value:
        span >= 0
          ? t("lead.figure.respondedIn", { span: formatDuration(span, locale) })
          : t("lead.readings.answered"),
      detail: t("lead.sla.answeredAt", {
        at: formatDayMonth(lead.first_response_at, locale, zone),
      }),
    };
  }
  const clock = firstResponseClock(lead);
  if (!clock) {
    return { key: "first-response", label, value: t("standing.noneYet") };
  }
  const breached = clock.state === "breached";
  const atRisk = clock.state === "at_risk";
  return {
    key: "first-response",
    label,
    value: breached
      ? t("lead.sla.breached")
      : atRisk
        ? t("lead.sla.atRisk")
        : t("lead.readings.owed"),
    detail: t(breached ? "lead.sla.overdueSince" : "lead.sla.dueBy", {
      at: formatDayMonth(clock.deadline, locale, zone),
    }),
    tone: breached ? "danger" : atRisk ? "warning" : undefined,
  };
}

function meetingFigure(
  meeting: Activity | undefined,
  ctx: ReadingContext,
): LeadFigure {
  const { t, locale, zone } = ctx;
  const label = t("lead.figure.nextMeeting");
  if (!meeting) {
    return { key: "meeting", label, value: t("lead.figure.noneBooked") };
  }
  return {
    key: "meeting",
    label,
    value: activityDay(
      meeting.occurred_at,
      meeting.occurred_precision,
      locale,
      zone,
      formatDayMonth,
    ),
    detail: meeting.subject?.trim() || t("lead.figure.booked"),
    tone: "success",
  };
}

/** The soonest meeting booked ahead of `asOf`, if the page holds one. */
export function nextBookedMeeting(
  activities: readonly Activity[],
  asOf: string,
  record: string,
): Activity | undefined {
  const now = Date.parse(asOf);
  return activities
    .filter(
      (row) =>
        row.kind === "meeting" &&
        row.meeting_status === "booked" &&
        activityIsAhead(row.occurred_at, row.occurred_precision, now, record),
    )
    .sort((a, b) => Date.parse(a.occurred_at) - Date.parse(b.occurred_at))[0];
}

export type LeadNext = Readonly<{
  title?: string;
  late?: Readonly<{ label: string; tone: ReadingTone }>;
  sub?: string;
  // The move is to qualify, so the bar carries the Qualify verb.
  qualify: boolean;
  // Open tasks beyond the one the bar names.
  more: number;
}>;

/**
 * The one move the lead asks for: qualify it once an engaged lead carries the
 * evidence qualifying rests on, else its next open task. A closed lead asks
 * for nothing.
 */
export function leadNext(lead: Lead, ctx: ReadingContext): LeadNext {
  const { t, locale, zone } = ctx;
  const tasks = lead.open_task_count ?? 0;
  if (lead.archived_at) {
    return { qualify: false, more: 0 };
  }
  const evidence = lead.qualification_evidence;
  if (lead.status === "engaged" && promoteEligible(lead) && evidence) {
    return {
      title: t("lead.next.ready"),
      late: {
        label: scoreFactorLabel(
          evidence.trigger === "inbound_reply" ? "reply" : evidence.trigger,
          t,
        ),
        tone: "success",
      },
      sub: t("lead.next.readyWhy"),
      qualify: true,
      more: tasks,
    };
  }
  if (!lead.next_task_subject) {
    return { qualify: false, more: tasks };
  }
  const due = lead.next_task_due_at;
  const late = due
    ? daysPast(Date.parse(due), Date.parse(ctx.asOf)).late
    : false;
  return {
    title: lead.next_task_subject,
    late: late ? { label: t("co.next.overdue"), tone: "danger" } : undefined,
    sub: due
      ? t("co.next.due", { when: formatDayMonth(due, locale, zone) })
      : t("co.next.undated"),
    qualify: false,
    more: Math.max(0, tasks - 1),
  };
}

// The lead's time on one axis, as events for `StandingTimeline`: when it
// arrived, our first answer, their newest message, and a meeting booked
// ahead. Read from the lead's own fields and its activity page.

import type { components } from "../../api/schema";
import type {
  TimelineEvent,
  TimelineGap,
} from "../../design-system/standingtimeline";
import {
  type ActivityMoment,
  activityDay,
  activityDaysAway,
  activityHasBegun,
} from "../../format/activitywhen";
import {
  formatDayMonth,
  formatDuration,
  formatNumber,
} from "../../format/format";
import { leadIdentityName } from "../../format/leadname";
import type { ReadingContext } from "./leadstandingparts";
import { nextBookedMeeting } from "./leadstandingparts";

type Lead = components["schemas"]["Lead"];
type Activity = components["schemas"]["Activity"];

const EXCHANGE_KINDS = new Set<Activity["kind"]>([
  "email",
  "call",
  "meeting",
  "message",
]);

// The newest row that went one way and had happened by `asOf`. A called-off
// meeting did not take place, so it is no contact.
function newestExchange(
  activities: readonly Activity[],
  direction: "inbound" | "outbound",
  asOf: number,
  record: string,
): Activity | undefined {
  return activities
    .filter(
      (row) =>
        row.direction === direction &&
        EXCHANGE_KINDS.has(row.kind) &&
        row.meeting_status !== "canceled" &&
        row.meeting_status !== "no_show" &&
        activityHasBegun(row.occurred_at, row.occurred_precision, asOf, record),
    )
    .sort((a, b) => Date.parse(b.occurred_at) - Date.parse(a.occurred_at))[0];
}

export type LeadTimeline = Readonly<{
  events: TimelineEvent[];
  gap?: TimelineGap;
}>;

export function leadTimeline(
  lead: Lead,
  activities: readonly Activity[],
  source: string,
  ctx: ReadingContext,
): LeadTimeline {
  const { t, locale, zone } = ctx;
  const asOf = Date.parse(ctx.asOf);
  const day = (at: string) => formatDayMonth(at, locale, zone);
  const name = leadIdentityName(lead) || t("lead.unnamed");
  const dayOf = (row: Activity) =>
    activityDay(
      row.occurred_at,
      row.occurred_precision,
      locale,
      zone,
      formatDayMonth,
    );
  const theirs = newestExchange(activities, "inbound", asOf, ctx.record);
  const ours = newestExchange(activities, "outbound", asOf, ctx.record);
  const events: TimelineEvent[] = [
    {
      key: "received",
      at: lead.created_at,
      title: source,
      sub: t("lead.timeline.received", { at: day(lead.created_at) }),
      kind: "done",
    },
  ];
  const answer = firstResponseEvent(lead, ctx);
  if (answer) {
    events.push(answer);
  }
  const theirsIsLast =
    theirs &&
    (!ours || Date.parse(theirs.occurred_at) >= Date.parse(ours.occurred_at));
  if (theirs) {
    events.push({
      key: `inbound-${theirs.id}`,
      at: theirs.occurred_at,
      title: t("standing.wrote", { name }),
      sub: theirsIsLast
        ? t("lead.timeline.lastContact", { at: dayOf(theirs) })
        : dayOf(theirs),
      kind: theirsIsLast ? "last" : "done",
      activityIds: [theirs.id],
    });
  }
  const meeting = nextBookedMeeting(activities, ctx.asOf, ctx.record);
  if (meeting) {
    events.push({
      key: `meeting-${meeting.id}`,
      at: meeting.occurred_at,
      title: meeting.subject?.trim() || t("lead.figure.nextMeeting"),
      sub: t("lead.timeline.booked", { at: dayOf(meeting) }),
      kind: "ahead",
    });
  }
  const newest = theirsIsLast ? theirs : ours;
  return {
    events,
    gap:
      newest &&
      silence(
        lead,
        { at: newest.occurred_at, precision: newest.occurred_precision },
        Boolean(theirsIsLast),
        ctx,
      ),
  };
}

// Our first answer, with how long the lead waited for it.
function firstResponseEvent(
  lead: Lead,
  ctx: ReadingContext,
): TimelineEvent | undefined {
  const at = lead.first_response_at;
  if (!at) {
    return undefined;
  }
  const day = formatDayMonth(at, ctx.locale, ctx.zone);
  const span = Date.parse(at) - Date.parse(lead.routed_at ?? lead.created_at);
  return {
    key: "first-response",
    at,
    title: ctx.t("lead.timeline.weReplied"),
    sub:
      span >= 0
        ? ctx.t("lead.timeline.repliedIn", {
            at: day,
            span: formatDuration(span, ctx.locale),
          })
        : day,
    kind: "done",
  };
}

// The days since the newest exchange, while the lead is open: their message
// last leaves the move with us, ours last leaves it with them. Under a day is
// no silence worth drawing.
function silence(
  lead: Lead,
  from: ActivityMoment,
  yourMove: boolean,
  ctx: ReadingContext,
): TimelineGap | undefined {
  const asOf = Date.parse(ctx.asOf);
  const days = -activityDaysAway(
    from.at,
    from.precision,
    asOf,
    ctx.zone,
    ctx.record,
  );
  if (lead.archived_at || days < 1) {
    return undefined;
  }
  return {
    from: from.at,
    title: ctx.t("contact.strip.days", {
      count: formatNumber(days, ctx.locale),
    }),
    sub: ctx.t(yourMove ? "lead.gap.yourMove" : "lead.gap.theirMove"),
  };
}

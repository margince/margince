import type { components } from "../../api/schema";
import type { ReadingTone } from "../../design-system/readingtone";
import type {
  TimelineEvent,
  TimelineGap,
} from "../../design-system/standingtimeline";
import { formatMoneyCompact, formatNumber } from "../../format/format";
import { calendarDaysBetween, formatDayMonth } from "../../format/format.dates";
import { daysPast } from "../../format/lateness";
import { formatMoneyOrWord } from "../../format/moneyword";
import { type Locale, type Translator, translatePlural } from "../../i18n";
import type { ContactTab } from "../../lib/contacttab";
import { dealRoleLabel } from "../../lib/recordlabels";
import { omitted } from "./contact360";
import { contactFirstName } from "./contactfirstname";
import { awaitsOurReply } from "./contactstanding";

type Contact360 = components["schemas"]["Contact360"];

/** How dates and counts are worded on this read. */
export type StandingWords = Readonly<{
  t: Translator;
  locale: Locale;
  zone: string;
}>;

/** One cell of the figures strip, and where pressing it leads. */
export type ContactFigure = Readonly<{
  key: string;
  label: string;
  value: string;
  detail?: string;
  tone: ReadingTone;
  opens: { tab: ContactTab } | { deal: string };
}>;

const CALLED_OFF = new Set(["canceled", "no_show"]);

// Done meetings past this many crowd the axis and stretch it back years.
const MEETINGS_DRAWN = 3;

function daysAgo(iso: string, view: Contact360, words: StandingWords) {
  const days = calendarDaysBetween(
    new Date(iso),
    new Date(view.as_of),
    words.zone,
  );
  if (days < 1) {
    return words.t("contact.strip.today");
  }
  return translatePlural(words.locale, "contact.standing.daysAgo", days, {
    count: formatNumber(days, words.locale),
  });
}

/** The four figures under the timeline. A withheld section says so in the
 *  cell rather than drawing a zero. */
export function contactFigures(
  view: Contact360,
  words: StandingWords,
): ContactFigure[] {
  return [
    lastReply(view, words),
    unanswered(view, words),
    openDeal(view, words),
    nextMeeting(view, words),
  ];
}

function lastReply(view: Contact360, words: StandingWords): ContactFigure {
  const { t, locale, zone } = words;
  const base = {
    key: "lastReply",
    label: t("contact.rail.lastReply"),
    opens: { tab: "timeline" },
  } as const;
  if (omitted(view, "last_touch")) {
    return { ...base, value: t("record.notShown"), tone: "neutral" };
  }
  const at = view.last_inbound_at;
  if (!at) {
    return { ...base, value: t("contact.strip.never"), tone: "neutral" };
  }
  return {
    ...base,
    value: formatDayMonth(at, locale, zone),
    detail: daysAgo(at, view, words),
    tone: awaitsOurReply(view) ? "warning" : "neutral",
  };
}

// The activities section is the newest page only, so this counts what it holds.
function unanswered(view: Contact360, words: StandingWords): ContactFigure {
  const { t, locale } = words;
  const base = {
    key: "unanswered",
    label: t("co.strip.unanswered"),
    opens: { tab: "timeline" },
  } as const;
  if (omitted(view, "activities")) {
    return { ...base, value: t("record.notShown"), tone: "neutral" };
  }
  const waiting = (view.activities?.data ?? []).filter(
    (row) => row.email_summary?.move === "needs_reply",
  );
  if (waiting.length === 0) {
    return { ...base, value: t("contact.standing.none"), tone: "neutral" };
  }
  return {
    ...base,
    value: translatePlural(locale, "contact.standing.emails", waiting.length, {
      count: formatNumber(waiting.length, locale),
    }),
    detail: waiting[0]?.subject ?? undefined,
    tone: "warning",
  };
}

function openDeal(view: Contact360, words: StandingWords): ContactFigure {
  const { t, locale } = words;
  const label = t("contact.commercial.openDeal");
  const commercial = view.commercial;
  const deal = commercial?.deal;
  if (!commercial || !deal) {
    return {
      key: "openDeal",
      label,
      value: commercial ? t("contact.standing.none") : t("record.notShown"),
      tone: "neutral",
      opens: { tab: "deals" },
    };
  }
  return {
    key: "openDeal",
    label,
    value: formatMoneyOrWord(
      deal.amount_minor,
      deal.currency,
      locale,
      t("co.strip.unpriced"),
      formatMoneyCompact,
    ),
    detail: [
      deal.title,
      commercial.role ? dealRoleLabel(commercial.role, t) : null,
    ]
      .filter(Boolean)
      .join(" · "),
    tone: "neutral",
    opens: { deal: deal.deal_id },
  };
}

function nextMeeting(view: Contact360, words: StandingWords): ContactFigure {
  const { t, locale, zone } = words;
  const base = {
    key: "nextMeeting",
    label: t("co.strip.nextMeeting"),
    opens: { tab: "meetings" },
  } as const;
  if (omitted(view, "next_meeting")) {
    return { ...base, value: t("record.notShown"), tone: "neutral" };
  }
  const meeting = view.next_meeting;
  if (!meeting) {
    return { ...base, value: t("co.strip.next.none"), tone: "neutral" };
  }
  return {
    ...base,
    value: formatDayMonth(meeting.starts_at, locale, zone),
    detail: meeting.subject ?? undefined,
    tone: "neutral",
  };
}

/** What the timeline draws: the meetings held, our promises with a date, the
 *  newest message from them, and the meeting booked ahead. */
export function contactTimelineEvents(
  view: Contact360,
  words: StandingWords,
): TimelineEvent[] {
  return [
    ...heldMeetings(view, words),
    ...datedPromises(view, words),
    ...lastWrote(view, words),
    ...meetingAhead(view, words),
  ];
}

function heldMeetings(view: Contact360, words: StandingWords): TimelineEvent[] {
  const { t, locale, zone } = words;
  const asOf = Date.parse(view.as_of);
  return (view.activities?.data ?? [])
    .filter(
      (row) =>
        row.kind === "meeting" &&
        !CALLED_OFF.has(row.meeting_status ?? "") &&
        Date.parse(row.occurred_at ?? "") <= asOf,
    )
    .sort((a, b) => Date.parse(b.occurred_at) - Date.parse(a.occurred_at))
    .slice(0, MEETINGS_DRAWN)
    .map((row) => ({
      key: `meeting-${row.id}`,
      at: row.occurred_at,
      title: row.subject ?? t("contact.standing.event.meeting"),
      sub: `${formatDayMonth(row.occurred_at, locale, zone)} · ${t("contact.standing.event.meetingKind")}`,
      kind: "done",
      activityIds: [row.id],
    }));
}

function datedPromises(
  view: Contact360,
  words: StandingWords,
): TimelineEvent[] {
  const { t, locale, zone } = words;
  const asOf = Date.parse(view.as_of);
  return (view.claims ?? []).flatMap((claim): TimelineEvent[] => {
    if (
      claim.kind !== "commitment_ours" ||
      claim.status !== "open" ||
      !claim.due_at
    ) {
      return [];
    }
    const late = daysPast(Date.parse(claim.due_at), asOf).late;
    const day = formatDayMonth(claim.due_at, locale, zone);
    return [
      {
        key: `promise-${claim.id}`,
        at: claim.due_at,
        title: t("contact.standing.event.promiseDue"),
        sub: late ? `${day} · ${t("co.next.overdue").toLowerCase()}` : day,
        kind: late ? "late" : "ahead",
      },
    ];
  });
}

function lastWrote(view: Contact360, words: StandingWords): TimelineEvent[] {
  const { t, locale, zone } = words;
  const at = view.last_inbound_at;
  if (!at || omitted(view, "last_touch")) {
    return [];
  }
  const row = (view.activities?.data ?? []).find(
    (candidate) => candidate.occurred_at === at,
  );
  return [
    {
      key: "last-inbound",
      at,
      title: t("standing.wrote", {
        name: contactFirstName(view.contact),
      }),
      sub: formatDayMonth(at, locale, zone),
      kind: "last",
      activityIds: row ? [row.id] : undefined,
    },
  ];
}

function meetingAhead(view: Contact360, words: StandingWords): TimelineEvent[] {
  const { t, locale, zone } = words;
  const meeting = view.next_meeting;
  if (!meeting) {
    return [];
  }
  return [
    {
      key: `ahead-${meeting.activity_id}`,
      at: meeting.starts_at,
      title: meeting.subject ?? t("contact.standing.event.meeting"),
      sub: formatDayMonth(meeting.starts_at, locale, zone),
      kind: "ahead",
    },
  ];
}

/** The silence since the newest message either way. Its sub says whose move
 *  it is, on the same rule as the head's "Your move". */
export function contactGap(
  view: Contact360,
  words: StandingWords,
): TimelineGap | undefined {
  if (omitted(view, "last_touch")) {
    return undefined;
  }
  const from = newest(view.last_inbound_at, view.last_outbound_at);
  if (!from) {
    return undefined;
  }
  const days = calendarDaysBetween(
    new Date(from),
    new Date(view.as_of),
    words.zone,
  );
  if (days < 1) {
    return undefined;
  }
  const { t, locale } = words;
  return {
    from,
    title: translatePlural(locale, "standing.days", days, {
      count: formatNumber(days, locale),
    }),
    sub: awaitsOurReply(view)
      ? t("standing.noReply")
      : t("co.strip.engagement.waiting_on_them"),
  };
}

function newest(
  a: string | null | undefined,
  b: string | null | undefined,
): string | undefined {
  if (!a || !b) {
    return a ?? b ?? undefined;
  }
  return Date.parse(a) >= Date.parse(b) ? a : b;
}

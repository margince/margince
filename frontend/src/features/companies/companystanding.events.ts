import type { components } from "../../api/schema";
import type {
  TimelineEvent,
  TimelineGap,
} from "../../design-system/standingtimeline";
import { activityIsAhead } from "../../format/activitywhen";
import { formatMoneyCompact, formatNumber } from "../../format/format";
import { calendarDaysBetween, formatDayMonth } from "../../format/format.dates";
import type { Locale, PluralTranslator, Translator } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { ENGAGEMENT_LABELS } from "./company360.strip";
import { upcomingMeeting } from "./companynextmeeting";
import { activityHeadline } from "./companyrecent";

type Company360 = components["schemas"]["Company360"];
type Activity = NonNullable<Company360["activities"]>["data"][number];

export type TimelineWords = Readonly<{
  t: Translator;
  plural: PluralTranslator;
  locale: Locale;
  zone: string;
}>;

// How many earlier exchanges the axis draws beside the last contact. More
// history than this lives on the History tab.
const PAST_EVENTS = 6;
// How many stops the axis keeps for them once the clusters are folded.
const PAST_STOPS = 3;
// Exchanges this close together share one stop: their labels would otherwise
// sit on top of each other along the axis.
const CLUSTER_MS = 7 * 24 * 60 * 60 * 1000;

const KIND_WORD: Record<Activity["kind"], MessageKey> = {
  email: "co.recent.kind.email",
  call: "co.recent.kind.call",
  meeting: "co.recent.kind.meeting",
  note: "co.recent.kind.note",
  task: "co.recent.kind.task",
  message: "co.recent.kind.message",
};

// A meeting that never took place is not something that happened here.
const DID_NOT_HAPPEN = new Set(["canceled", "no_show"]);

/**
 * The account's time for How it stands: what happened, the last contact,
 * what is owed or booked ahead, and the expected close. Read from the 360
 * alone, dated by `as_of`, never by the clock.
 */
export function companyTimeline(
  view: Company360,
  words: TimelineWords,
): { events: TimelineEvent[]; gap?: TimelineGap } {
  const day = (iso: string) => formatDayMonth(iso, words.locale, words.zone);
  const asOf = Date.parse(view.as_of);
  const last = view.last_contact;
  const rows = view.activities?.data ?? [];
  const past = rows
    .filter(
      (row) =>
        row.kind !== "task" &&
        row.id !== last?.activity_id &&
        !DID_NOT_HAPPEN.has(row.meeting_status ?? "") &&
        !activityIsAhead(
          row.occurred_at,
          row.occurred_precision,
          asOf,
          words.zone,
        ),
    )
    .sort((a, b) => Date.parse(b.occurred_at) - Date.parse(a.occurred_at))
    .slice(0, PAST_EVENTS);
  const happened = clusters(past)
    .slice(0, PAST_STOPS)
    .reverse()
    .flatMap((cluster) => pastStop(cluster, words, day));
  return {
    events: [
      ...happened,
      ...lastContact(view, rows, words, day),
      ...owedAhead(view, words, day),
    ],
    gap: silence(view, words),
  };
}

// Groups exchanges, newest first, into runs that share one week. A busy week
// is then one stop that counts its exchanges rather than a pile of labels.
function clusters(newestFirst: readonly Activity[]): Activity[][] {
  const runs: Activity[][] = [];
  let runStart: number | undefined;
  let current: Activity[] = [];
  for (const row of newestFirst) {
    const at = Date.parse(row.occurred_at);
    if (runStart !== undefined && runStart - at < CLUSTER_MS) {
      current.push(row);
      continue;
    }
    current = [row];
    runs.push(current);
    runStart = at;
  }
  return runs;
}

// One stop for a run: the exchange itself when it stands alone, else how many
// there were across which days.
function pastStop(
  run: readonly Activity[],
  words: TimelineWords,
  day: (iso: string) => string,
): TimelineEvent[] {
  const [newest] = run;
  const oldest = run.at(-1);
  if (!newest || !oldest) {
    return [];
  }
  const alone = run.length === 1;
  return [
    {
      key: `activity-${newest.id}`,
      at: newest.occurred_at,
      title: alone
        ? activityHeadline(newest, words.t)
        : words.t("co.spine.exchangeCount", {
            count: formatNumber(run.length, words.locale),
          }),
      sub: alone
        ? `${day(newest.occurred_at)} · ${words.t(KIND_WORD[newest.kind])}`
        : `${day(oldest.occurred_at)}–${day(newest.occurred_at)}`,
      kind: "done",
      activityIds: run.map((row) => row.id),
    },
  ];
}

function lastContact(
  view: Company360,
  rows: readonly Activity[],
  words: TimelineWords,
  day: (iso: string) => string,
): TimelineEvent[] {
  const last = view.last_contact;
  if (!last) {
    return [];
  }
  const row = rows.find((candidate) => candidate.id === last.activity_id);
  return [
    {
      key: "last-contact",
      at: last.at,
      title: row
        ? activityHeadline(row, words.t)
        : words.t("co.spine.lastSpoke"),
      sub: `${day(last.at)} · ${words.t(row ? "co.spine.lastSpoke" : KIND_WORD[last.kind])}`,
      kind: "last",
      activityIds: [last.activity_id],
    },
  ];
}

function owedAhead(
  view: Company360,
  words: TimelineWords,
  day: (iso: string) => string,
): TimelineEvent[] {
  const events: TimelineEvent[] = [];
  // The two the spine always drew: a longer list is the Tasks tab.
  for (const step of (view.next_steps?.data ?? []).slice(0, 2)) {
    if (step.due_at) {
      events.push({
        key: `step-${step.activity_id}`,
        at: step.due_at,
        title: step.subject,
        sub: step.overdue
          ? `${day(step.due_at)} · ${words.t("co.spine.overdue")}`
          : day(step.due_at),
        kind: step.overdue ? "late" : "ahead",
      });
    }
  }
  const meeting = upcomingMeeting(view, words.zone);
  if (meeting) {
    events.push({
      key: `meeting-${meeting.activity_id}`,
      at: meeting.starts_at,
      title: meeting.subject,
      sub: `${day(meeting.starts_at)} · ${words.t("co.recent.kind.meeting")}`,
      kind: "ahead",
    });
  }
  const close = view.state_strip?.commercial?.next_close_on;
  if (close) {
    const amount = closingAmount(view, close, words.locale);
    events.push({
      key: "close",
      at: close,
      title: words.t("co.spine.expectedClose"),
      sub: amount ? `${day(close)} · ${amount}` : day(close),
      kind: "end",
    });
  }
  return events;
}

// The amount of the one open deal that closes on the date the strip names.
// Two deals on that day have no single amount, so none is shown.
function closingAmount(
  view: Company360,
  close: string,
  locale: Locale,
): string | undefined {
  const closing = (view.deals?.data ?? []).filter(
    (deal) => deal.status === "open" && deal.expected_close_date === close,
  );
  const amount = closing.length === 1 ? closing[0]?.amount : undefined;
  return amount?.amount_minor != null && amount.currency
    ? formatMoneyCompact(amount.amount_minor, amount.currency, locale)
    : undefined;
}

// The wait since the last word, on the server's own reading of whose move it
// is. It runs from our last message when they owe a reply, from theirs when
// we do.
function silence(
  view: Company360,
  words: TimelineWords,
): TimelineGap | undefined {
  const state = view.state_strip?.engagement?.state;
  const from =
    state === "waiting_on_them"
      ? view.last_outbound_at
      : state === "waiting_on_us"
        ? view.last_inbound_at
        : undefined;
  if (!from || (state !== "waiting_on_them" && state !== "waiting_on_us")) {
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
  return {
    from,
    title: words.plural("standing.days", days, {
      count: formatNumber(days, words.locale),
    }),
    sub: words.t(ENGAGEMENT_LABELS[state]),
  };
}

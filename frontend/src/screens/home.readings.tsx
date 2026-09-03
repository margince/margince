// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { StatCard } from "../design-system/atoms";
import { StatStrip } from "../design-system/statstrip";
import { formatDateTime, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import {
  type Locale,
  type Translator,
  useLocale,
  usePlural,
  useT,
} from "../i18n";
import { isUnprepared } from "./worklist.copy";
import type { Worklist, WorklistItem } from "./worklist.queries";

// The day's readings, on one plate.
//
// FIVE slots, always. A reading nobody can take stays in the row and says what
// it has none of, because a strip is read ACROSS as one comparison: a row that
// shrank to four would let a reader take a missing question for an answered one.
// That is the opposite convention to the Worklist's own strip, which draws only
// what it can measure — and deliberately so. This is the page a rep opens to ask
// "what is my morning", and the questions it does not answer are part of the
// answer.
//
// Every figure comes from the ONE worklist answer the queue below is drawn from,
// so no second read can put a different number beside the same rows. Two of the five have
// no source at all today and say so in words rather than drawing a nought:
// promises are not tracked (the commitments lane is unwired) and quota targets
// were retired from the product.
//
// The floor caveat is the plate's own, through `StatStrip`'s `floor` slot — the
// row is read across as one statement, so a caveat belonging to one figure would
// invite the reading where the others are exact.

const MEETINGS = "meetings";
const LEADS = "leads";

export function HomeReadingsStrip({ day }: Readonly<{ day: Worklist }>) {
  const t = useT();
  const { locale } = useLocale();
  const readings = day.readings;
  const meetings = meetingsReading(day);
  const soonest = soonestLeadDeadline(day);
  return (
    <section className="home-readings" aria-label={t("home.readings.label")}>
      <StatStrip
        testId="home-readings"
        floor={
          readings.more_available ? t("home.readings.truncated") : undefined
        }
      >
        <StatCard
          numeric
          label={t("home.readings.waiting")}
          value={formatNumber(readings.buyer_replies, locale)}
          tone={readings.buyer_replies > 0 ? "warn" : undefined}
          // The basis line says what the figure was taken over, on every day. A
          // zero already reads as "none"; a line under it repeating that says
          // the same thing twice and drops the one fact it could add.
          detail={t("home.readings.waitingBasis")}
        />
        <MeetingsStat
          meetings={meetings.meetings}
          unready={meetings.unready}
          locale={locale}
          t={t}
        />
        <Untracked
          label={t("home.readings.promises")}
          detail={t("home.readings.promisesBasis")}
        />
        <LeadsStat
          leads={readings.prospecting}
          soonest={soonest}
          locale={locale}
          t={t}
        />
        <Untracked
          label={t("home.readings.quota")}
          detail={t("home.readings.quotaBasis")}
        />
      </StatStrip>
    </section>
  );
}

// How many meetings, and how many of them nothing is prepared for.
//
// Readiness is the fact that changes what a reader does before the first one
// starts, so a day with meetings and nothing unprepared says "all prepared"
// rather than leaving the line blank: the absence of a warning has to be
// readable as an answer, not as a gap.
function MeetingsStat({
  meetings,
  unready,
  locale,
  t,
}: Readonly<{
  meetings: number;
  // Null when the page carries fewer meetings than it counted, so no honest
  // readiness figure exists — NOT the same as zero unprepared.
  unready: number | null;
  locale: Locale;
  t: Translator;
}>) {
  const plural = usePlural();
  return (
    <StatCard
      numeric
      label={t("home.readings.meetings")}
      value={formatNumber(meetings, locale)}
      tone={unready !== null && unready > 0 ? "warn" : undefined}
      detail={meetingsDetail(meetings, unready, locale, t, plural)}
    />
  );
}

function meetingsDetail(
  meetings: number,
  unready: number | null,
  locale: Locale,
  t: Translator,
  plural: ReturnType<typeof usePlural>,
): string {
  if (unready === null) {
    return t("home.readings.prepUnknown");
  }
  if (unready > 0) {
    return plural("home.readings.needsPrep", unready, {
      count: formatNumber(unready, locale),
    });
  }
  // "All prepared" is a claim about meetings, and an empty day has none to make
  // it about. The basis line says what was looked at instead.
  return meetings === 0
    ? t("home.readings.meetingsBasis")
    : t("home.readings.prepared");
}

// How much new business is owed a first answer, and when the nearest one is due.
//
// The deadline is the fact that changes what a reader does before lunch, so it
// takes the same shape readiness takes on the meetings slot beside it: a second
// fact in the detail line, and NULL rather than a guess where the page cannot
// honestly compute one.
function LeadsStat({
  leads,
  soonest,
  locale,
  t,
}: Readonly<{
  leads: number;
  // Null when no honest nearest deadline exists — either nothing on the page
  // names one, or the leads read was cut short and an unshown lead could be
  // sooner than every one the reader can see.
  soonest: string | null;
  locale: Locale;
  t: Translator;
}>) {
  return (
    <StatCard
      numeric
      label={t("home.readings.leads")}
      value={formatNumber(leads, locale)}
      tone={leads > 0 ? "warn" : undefined}
      detail={
        soonest === null
          ? t("home.readings.leadsBasis")
          : t("home.readings.leadsDue", {
              value: formatDateTime(soonest, locale, viewerZone()),
            })
      }
    />
  );
}

// The nearest deadline among the lead rows the page is SHOWING, or none.
//
// None in two cases that are one rule: the page cannot see the whole lane, or no
// row on it names a moment. A cut read is the interesting one — an unshown lead
// could be due sooner than every one the reader can see, so naming the earliest
// visible deadline would state a "next" that is not next. The slot falls back to
// its plain basis line, which is what the meetings slot does with readiness for
// the same reason.
function soonestLeadDeadline(day: Worklist): string | null {
  const entry = day.counts.find((count) => count.category === LEADS);
  if (entry === undefined) {
    // No lead was read at all: nothing to be nearest, and nothing missing.
    return null;
  }
  if (entry.shown !== entry.considered || entry.more_available) {
    return null;
  }
  let soonest: string | null = null;
  for (const item of day.queue) {
    const at = item.category === LEADS ? replyDueAt(item) : undefined;
    if (at !== undefined && (soonest === null || at < soonest)) {
      soonest = at;
    }
  }
  return soonest;
}

// When this row says a reply is due, or nothing.
//
// The moment is read off the at-risk reason BY NAME rather than by taking
// whatever date the row carries. An overdue lead has already missed its moment,
// so it is not the next one due — and no test here can hold that distinction,
// because a breached lead's other reason (`waiting_days`) carries a DAYS value,
// which a filter reading "any date value" would skip anyway. The kind check is
// what keeps this right when a lead row grows a second date-valued reason, a
// first-contact date or a routing moment, that would otherwise read as a reply
// deadline.
function replyDueAt(item: WorklistItem): string | undefined {
  for (const because of item.because) {
    if (
      because.kind === "response_due_soon" &&
      because.value?.kind === "date"
    ) {
      return because.value.date;
    }
  }
  return undefined;
}

// The count slots call `StatCard` straight rather than through a shared tile
// helper. `worklist.readings.tsx` has a `CountStat` of its own, and lifting it
// was considered: it takes a fixed detail string, while these need a detail that
// changes with the figure and a tone that follows it. A helper carrying both
// would be `StatCard` with its own props spelled twice — a rename, not a shared
// answer. The primitive they genuinely share is `StatCard`, and both use it.

// A slot whose question the product cannot answer yet.
//
// It draws an em dash and says why underneath. A zero here would be a false
// answer — "you owe nobody a promise" is a claim, and nothing in the estate has
// standing to make it.
function Untracked({
  label,
  detail,
}: Readonly<{ label: string; detail: string }>) {
  // The em dash is not a translated string: it is the same mark in every
  // catalog, and `worklist.readings.tsx` spells an unpriced figure the same way.
  return <StatCard label={label} value="—" detail={detail} />;
}

// The meetings reading: how many stand behind the day, and how many of those
// nothing is prepared for — or that the second question could not be answered.
//
// The two figures come from DIFFERENT populations and that is the whole care
// here. `considered` counts every meeting read and ranked, before the fold and
// before the page cut; the readiness figure can only be counted off the rows the
// page actually carries. Divide one by the other and a day with ten meetings
// considered and three on the page reads "10 · 2 need prep", telling a rep eight
// meetings are ready when nothing checked them.
//
// So readiness is claimed ONLY when the page carries every meeting it counted.
// Short of that the slot says the page could not check them all, which is the
// same honesty the strip's untracked slots keep.
function meetingsReading(day: Worklist): {
  meetings: number;
  unready: number | null;
} {
  const entry = day.counts.find((count) => count.category === MEETINGS);
  // No entry at all means no meeting was read: a day of zero meetings, carried
  // whole. Treating that as unanswerable told a rep the page could not check
  // meetings it had already established there were none of.
  if (entry === undefined) {
    return { meetings: 0, unready: 0 };
  }
  const whole = entry.shown === entry.considered && !entry.more_available;
  return {
    meetings: entry.considered,
    unready: whole ? day.queue.filter(isUnprepared).length : null,
  };
}

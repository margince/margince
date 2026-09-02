// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { StatCard } from "../design-system/atoms";
import { StatStrip } from "../design-system/statstrip";
import { formatNumber } from "../format/format";
import {
  type Locale,
  type Translator,
  useLocale,
  usePlural,
  useT,
} from "../i18n";
import { isUnprepared } from "./worklist.copy";
import type { Worklist } from "./worklist.queries";

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

export function HomeReadingsStrip({ day }: Readonly<{ day: Worklist }>) {
  const t = useT();
  const { locale } = useLocale();
  const readings = day.readings;
  const meetings = meetingsReading(day);
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
        <StatCard
          numeric
          label={t("home.readings.leads")}
          value={formatNumber(readings.prospecting, locale)}
          tone={readings.prospecting > 0 ? "warn" : undefined}
          detail={t("home.readings.leadsBasis")}
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

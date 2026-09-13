// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { navigate } from "../app/router";
import { StatCard } from "../design-system/atoms";
import { StatStrip } from "../design-system/statstrip";
import { useTooltip } from "../design-system/tooltip";
import { formatDateTime, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import {
  type Locale,
  type Translator,
  useLocale,
  usePlural,
  useT,
} from "../i18n";
import { PipelineOutlook } from "./brief.pipelineoutlook";
import {
  boundedCategories,
  DECISIONS,
  decisionsBlocking,
  LEADS,
  MEETINGS,
} from "./brief.readings.honesty";
import { WORKLIST_FILTER_PARAM } from "./worklist";
import { isUnprepared } from "./worklist.copy";
import type {
  Worklist,
  WorklistFilter,
  WorklistItem,
} from "./worklist.queries";

// The day's readings, on one dense plate.
//
// FIVE slots, and every one of them answerable. The plate asks: what is urgent,
// what is on today's calendar, which leads are owed a reply, where the pipeline
// stands this quarter, and what is waiting on a decision.
//
// FOUR OF THE FIVE come from the ONE worklist answer the queue below is drawn
// from, so no second read can put a different number beside the same rows. The
// pipeline outlook is the exception and is read separately, through the same
// query key Analytics uses — one answer to "what is the pipeline worth",
// wherever it is asked.
//
// THE WHOLE CELL IS THE DOOR, AND EACH DOOR SAYS WHAT IT DOES. The cell is the
// control, so what a reader presses is the reading they are looking at. Its
// foot said "Open" on all five, which is one entry repeated five times in a
// screen reader's control list. Each now names its own action — REPLACING the
// generic word, never appending, which produced "Open Open pipeline".
//
// A BOUNDED READ IS A `+` ON THE FIGURES IT IS TRUE OF. The row used to carry a
// sentence saying a source hit its limit; the fact belongs on the number, and
// the cell's hover line says why. All four used to be marked from
// `WorklistReadings.more_available`, which is one flag for the whole answer —
// so a calendar read whole showed `3+` because an unrelated lane stopped, and a
// mark on the exact figures is one a reader learns to discount. Per slot is not
// guesswork: `counts` carries `more_available` PER CATEGORY, seeded from the
// bounded SOURCES through `categoryOfSource` (reach.go).

// Open the worklist on the lane a reading counted.
//
// Each figure in this strip IS one of the queue's filter pills counted, so the
// reading's door is that lane.
function openLane(filter: WorklistFilter): void {
  navigate({ screen: "worklist" }, new Map([[WORKLIST_FILTER_PARAM, filter]]));
}

/**
 * One reading: what it is, the figure, and what the figure rests on.
 *
 * `warn` is deliberately narrow. Neutral ink is the default for every figure on
 * the plate, because a row where four numbers are coloured is a traffic light
 * rather than a comparison. It is spent only where the reading counts something
 * that is BREACHING — somebody waiting, a promise going, a meeting starting
 * with nothing prepared — and never on a figure that is merely large.
 */
type Reading = Readonly<{
  label: string;
  /** The figure itself, so the slot can tell a floor of none from a floor. */
  count: number;
  basis: ReactNode;
  warn?: boolean;
  /** The source behind the figure was read to its bound: it is a floor. */
  floor?: boolean;
  /** The lane this reading counted, which is where its cell leads. */
  lane: WorklistFilter;
  /** What this reading's door says, so five doors are not five "Open"s. */
  openLabel: string;
  /**
   * This slot spans the whole day rather than naming one topic, so its door
   * stands even at zero.
   *
   * A TOPIC's zero has nothing behind it and its door is a trip: "no meetings
   * today" leads to an empty lane framed by every other slot's urgent count,
   * which reads as a broken filter. The spanning slot is not a topic — it is
   * the strip's way into the worklist at all, and a clear morning is exactly
   * when a reader wants to go and look.
   *
   * Declared by the caller rather than read off `lane`, so this component never
   * has to know which of its callers' values is the special one.
   */
  spans?: boolean;
}>;

/**
 * One reading, with the door in the card's own foot.
 *
 * The DOOR IS A WORD, not the cell. `StatCard onOpen` is the one spelling of
 * that control in the product and it stretches its own press target over the
 * whole tile, so the reading is still one thing to press — and a stat card's
 * appearance is a question for the card, not for the five screens that draw one.
 *
 * The lane goes in the QUERY rather than the path because `#/worklist/<owner>`
 * is already an address the team board navigates to, and `routeIdentity`
 * ignores the query half — so this narrows the view without remounting the
 * screen, and leaves an address somebody can paste.
 */
function LaneReading({
  label,
  count,
  basis,
  warn,
  floor,
  lane,
  openLabel,
  spans,
}: Reading) {
  const t = useT();
  const { locale } = useLocale();
  // A FLOOR OF NONE IS NOT A FLOOR. `0+` says "at least nothing", which is
  // true of every number there has ever been — so the mark goes on a figure
  // that counts something and nowhere else. A bounded read that found none of
  // a kind is a reading of zero, and the `+` was noise on it.
  const marked = floor === true && count > 0;
  // A DOOR INTO NOTHING IS NOT REASSURANCE, it is a trip. A topic's confirmed
  // zero has no rows behind it, so its door lands the reader in an empty lane
  // framed by the urgent counts of every other slot — which reads as a filter
  // they broke rather than as a morning with none of that kind.
  //
  // Three things keep a door, and each is a different reason:
  //   - a figure that counts something, which has rows to show;
  //   - a BOUNDED zero, because "none so far, and the read stopped early" is a
  //     question the worklist can still answer where "none" is not. `floor`
  //     tells those apart and is already on this slot for the `+` mark;
  //   - the SPANNING slot, which is the strip's way into the worklist at all.
  const openable = count > 0 || floor === true || spans === true;
  // The tip rides the whole CELL rather than the three characters that carry
  // the mark: a `+` that explains itself only to a pointer resting on it is a
  // mark most readers never read. Focus reaches it too — the card's own door is
  // inside this element, and a focus event bubbles.
  const floorTip = useTooltip<HTMLSpanElement>(t("brief.readings.floorTip"));

  const figure = formatNumber(count, locale);
  const card = (
    <StatCard
      label={label}
      value={marked ? `${figure}+` : figure}
      tone={warn ? "warn" : undefined}
      detail={basis}
      // The whole plate is read as ONE glance, so every slot on it keeps the
      // same air. At the tile's default floor the day's own work started below
      // the fold on a laptop.
      density="compact"
      onOpen={openable ? () => openLane(lane) : undefined}
      openLabel={openLabel}
    />
  );
  // A plain SPAN and nothing more where the figure is a floor: it carries the
  // tip and no behaviour of its own, because the card inside it already holds
  // the only control on the cell. An unmarked reading gets no wrapper at all —
  // an `aria-describedby` pointing at a tip nobody renders is a dangling
  // reference.
  return marked ? (
    <span ref={floorTip.ref} {...floorTip.trigger}>
      {card}
      {floorTip.tip}
    </span>
  ) : (
    card
  );
}

export function BriefReadingsStrip({ day }: Readonly<{ day: Worklist }>) {
  const t = useT();
  const { locale } = useLocale();
  const plural = usePlural();
  const readings = day.readings;
  const meetings = meetingsReading(day);
  const soonest = soonestLeadDeadline(day);
  const blocking = decisionsBlocking(day);
  // A lane that never ANSWERED is the case the per-category narrowing does not
  // reach: it travels in `sources_unavailable`, which names a source, and only
  // the server maps a source to its lane. Re-deriving that here would be a
  // second copy of it, so an unavailable lane marks the whole strip — which
  // over-marks rather than calling a figure exact over work nobody could see.
  const bounded = boundedCategories(day);
  const unread = day.sources_unavailable.length > 0;
  const floorOf = (category: string): boolean =>
    unread || bounded.has(category);
  return (
    <section className="brief-readings" aria-label={t("brief.readings.label")}>
      <StatStrip testId="brief-readings">
        <LaneReading
          label={t("brief.readings.urgent")}
          // The SUMMARY's own count, not one lane's. `urgent` is every row at
          // the top two levels — somebody waiting or a promise breaking — and
          // the morning's first question is how many of those there are.
          count={day.summary.urgent}
          warn={day.summary.urgent > 0}
          // The one slot that genuinely spans the day: `urgent` is every row at
          // the top two levels whatever lane raised it, so any bounded source
          // anywhere makes it a floor. This is what `more_available` is for.
          floor={readings.more_available}
          // The basis says what the figure was taken over, on every day. A zero
          // already reads as "none"; a line repeating that says the same thing
          // twice and drops the one fact it could add.
          basis={t("brief.readings.urgentBasis")}
          openLabel={t("brief.readings.openUrgent")}
          // ITS OWN LANE, not the whole queue. This figure counts levels 0 to
          // 2; opening `all` landed a reader who was sent by a 4 in a list of
          // thirty, with nothing saying which four it meant.
          lane="urgent"
          // The one slot that is not a topic: its door stands at zero, because
          // a clear morning is exactly when a reader goes to look for
          // themselves. Every other slot's zero is a dead end.
          spans
        />
        <LaneReading
          label={t("brief.readings.meetings")}
          count={meetings.meetings}
          // Readiness is the breach here: a meeting starting with nothing
          // prepared is the one fact on this slot a reader must act on before
          // it begins. The count of meetings itself is neither good nor bad.
          warn={meetings.unready !== null && meetings.unready > 0}
          floor={floorOf(MEETINGS)}
          basis={meetingsDetail(meetings, locale, t, plural)}
          openLabel={t("brief.readings.openMeetings")}
          lane="meetings"
        />
        <LaneReading
          label={t("brief.readings.leads")}
          count={readings.prospecting}
          floor={floorOf(LEADS)}
          // The deadline is the fact that changes what a reader does before
          // lunch, and NULL rather than a guess where the page cannot honestly
          // compute one.
          basis={
            soonest === null
              ? t("brief.readings.leadsBasis")
              : t("brief.readings.leadsDue", {
                  value: formatDateTime(soonest, locale, viewerZone()),
                })
          }
          openLabel={t("brief.readings.openLeads")}
          lane="leads"
        />
        <PipelineOutlook />
        <LaneReading
          label={t("brief.readings.decisions")}
          count={readings.review}
          floor={floorOf(DECISIONS)}
          // Only where something IS held up, and how much of it. Otherwise the
          // plain basis, which says what the figure was taken over and claims
          // nothing about who is waiting.
          basis={
            blocking === null || blocking === 0
              ? t("brief.readings.decisionsBasis")
              : plural("brief.readings.decisionsBlocking", blocking, {
                  count: formatNumber(blocking, locale),
                })
          }
          openLabel={t("brief.readings.openDecisions")}
          lane="decisions"
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
function meetingsDetail(
  reading: MeetingsReading,
  locale: Locale,
  t: Translator,
  plural: ReturnType<typeof usePlural>,
): string {
  const { meetings, unready } = reading;
  if (unready === null) {
    return t("brief.readings.prepUnknown");
  }
  if (unready > 0) {
    return plural("brief.readings.needsPrep", unready, {
      count: formatNumber(unready, locale),
    });
  }
  // "All prepared" is a claim about meetings, and an empty day has none to make
  // it about. The basis line says what was looked at instead.
  return meetings === 0
    ? t("brief.readings.meetingsBasis")
    : t("brief.readings.prepared");
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

type MeetingsReading = Readonly<{
  meetings: number;
  // Null when the page carries fewer meetings than it counted, so no honest
  // readiness figure exists — NOT the same as zero unprepared.
  unready: number | null;
}>;

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
function meetingsReading(day: Worklist): MeetingsReading {
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

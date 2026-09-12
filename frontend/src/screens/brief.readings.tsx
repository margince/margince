// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { useRecordZone } from "../app/recordzone";
import { navigate } from "../app/router";
import { StatCard } from "../design-system/atoms";
import { StatStrip } from "../design-system/statstrip";
import { useTooltip } from "../design-system/tooltip";
import { middayInstant } from "../format/calendarday";
import {
  formatDateAbbrev,
  formatDateTime,
  formatMoneyCompact,
  formatNumber,
} from "../format/format";
import { quarterLabel } from "../format/quarter";
import { viewerZone } from "../format/timezone";
import {
  type Locale,
  type Translator,
  useLocale,
  usePlural,
  useT,
} from "../i18n";
import { openAnalyticsSection } from "./analytics.address";
import { useAnalyticsContext } from "./analytics.context";
import { useForecastReadings } from "./forecast.queries";
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
// THE WHOLE CELL IS THE DOOR. It used to be a word: every slot drew an
// "Open →" line in its foot, which is a decorative row on a plate whose whole
// argument is that five readings are taken in at one glance — and five doors
// all reading "Open" were five identical rows in a screen reader's list. The
// cell is now the control, so what a reader presses is the reading they are
// looking at and what a screen reader announces is that reading's own words.
//
// A BOUNDED READ IS A `+`, NOT A SENTENCE. The row used to carry a line saying
// a source had been read to its limit, so every figure above it was a floor.
// The fact belongs ON the figures: `8+` says it where the number is, and the
// cell's hover line says why. `WorklistReadings.more_available` is ONE flag for
// the whole readings block by contract — "set once for the strip rather than
// per reading" — so it marks every figure it covers rather than being
// attributed to whichever slot a reader happens to suspect.

const MEETINGS = "meetings";
const LEADS = "leads";

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
function LaneReading({ label, count, basis, warn, floor, lane }: Reading) {
  const t = useT();
  const { locale } = useLocale();
  // A FLOOR OF NONE IS NOT A FLOOR. `0+` says "at least nothing", which is
  // true of every number there has ever been — so the mark goes on a figure
  // that counts something and nowhere else. A bounded read that found none of
  // a kind is a reading of zero, and the `+` was noise on it.
  const marked = floor === true && count > 0;
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
      onOpen={() => openLane(lane)}
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
  // ONE flag for the four figures read off this answer, which is what the
  // contract says it is. Splitting it per slot would attribute a bound to
  // whichever reading looked likeliest, over populations that differ.
  const floor = readings.more_available;
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
          floor={floor}
          // The basis says what the figure was taken over, on every day. A zero
          // already reads as "none"; a line repeating that says the same thing
          // twice and drops the one fact it could add.
          basis={t("brief.readings.urgentBasis")}
          lane="all"
        />
        <LaneReading
          label={t("brief.readings.meetings")}
          count={meetings.meetings}
          // Readiness is the breach here: a meeting starting with nothing
          // prepared is the one fact on this slot a reader must act on before
          // it begins. The count of meetings itself is neither good nor bad.
          warn={meetings.unready !== null && meetings.unready > 0}
          floor={floor}
          basis={meetingsDetail(meetings, locale, t, plural)}
          lane="meetings"
        />
        <LaneReading
          label={t("brief.readings.leads")}
          count={readings.prospecting}
          floor={floor}
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
          lane="leads"
        />
        <PipelineOutlook />
        <LaneReading
          label={t("brief.readings.decisions")}
          count={readings.review}
          floor={floor}
          basis={t("brief.readings.decisionsBasis")}
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

// Where the pipeline stands this quarter: what is open, and what it is worth
// weighted.
//
// TWO FIGURES, NEITHER OF THEM A TARGET. The card NEVER says on track,
// attainment, or gap. There is no authoritative target in this product to
// compare against — the quota table was dropped by founder decision — so any
// such word would be inventing the thing that was removed.
//
// THE PERIOD IS IN THE TITLE, NOT THE BASIS. The headline reads one quarter's
// open pipeline, and a reader who cannot see which quarter cannot reconcile it
// against the currency totals beside it. `Q3` is all a title has room for, so
// the full range — and, where the reading is the whole organization's rather
// than this reader's, whose pipeline it is — goes on the cell's hover line.
//
// READ THROUGH THE SAME KEY ANALYTICS USES. Two surfaces asking what the
// pipeline is worth must not get two answers, so this calls the shared hook
// rather than its own fetch.
function PipelineOutlook() {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  // The reader's OWN pipeline, under the scope the SERVER names for them.
  // `/analytics/context` answers `default_scope`, which is what Analytics
  // starts from too — asking for it here is what makes the shared query key
  // true rather than merely claimed, and it stops the client constructing a
  // scope it has no standing to name.
  const context = useAnalyticsContext();
  const readings = useForecastReadings(context.data?.default_scope);
  const data = readings.data;
  // The RECORD's zone, and midday rather than midnight. These are date-only
  // wire values: there is no instant in "2026-07-01" to localize, and read in
  // the viewer's clock west of UTC they print the day before — a quarter
  // labelled 30 Jun – 29 Sept. The period is a property of the installation's
  // calendar, the same for every colleague reading it.
  const range =
    data === undefined
      ? ""
      : `${formatDateAbbrev(middayInstant(data.period_start, recordZone), locale, recordZone)} – ${formatDateAbbrev(middayInstant(data.period_end, recordZone), locale, recordZone)}`;
  const tip = useTooltip<HTMLSpanElement>(
    data?.scope_kind === "workspace"
      ? t("brief.readings.pipelineTipWorkspace", { period: range })
      : range,
  );

  if (readings.isPending) {
    // KEEPS THE ROW'S SHAPE: a basis line, like every slot beside it. A card one
    // line shorter reflows the whole strip when the read lands, which is the
    // opposite of the property this plate's fixed slot count defends.
    return (
      <StatCard
        label={t("brief.readings.pipelinePlain")}
        value={t("brief.readings.pipelinePending")}
        detail={t("brief.readings.pipelineReading")}
        density="compact"
      />
    );
  }
  // The currency is what makes the money sayable, so its absence is read as an
  // unanswered question rather than as a figure. `base_currency` IS required of
  // this response, and that is exactly why the check is here: a 200 whose shape
  // is not the one the contract promises is another absent read, and reaching
  // into it for the currency threw — taking the whole of `#/brief` down to the
  // app's error boundary, where a reader sees no strip, no feed and no rail.
  if (readings.isError || !data?.base_currency) {
    // A read that did not land is not a pipeline of nothing, and the slot says
    // so in words: a glyph in one slot of a row read across as one statement
    // reads as a figure the plate failed to draw rather than as one nobody has.
    return (
      <StatCard
        label={t("brief.readings.pipelinePlain")}
        value={t("brief.readings.pipelineNoRead")}
        detail={t("brief.readings.pipelineUnread")}
        density="compact"
      />
    );
  }
  const quarter = quarterLabel(data.period_start);
  return (
    // The range and, where the figure is the whole organization's, whose
    // pipeline it is: a dense title has room for a quarter and nothing more.
    <span ref={tip.ref} {...tip.trigger}>
      <StatCard
        label={
          quarter === null
            ? t("brief.readings.pipelinePlain")
            : t("brief.readings.pipeline", { quarter })
        }
        // formatMoneyCompact, not the full amount: its own doc says a strip slot
        // has about 110px and a full euro figure wraps mid-number or clips.
        density="compact"
        value={formatMoneyCompact(data.open_minor, data.base_currency, locale)}
        // THE FORECAST SECTION, which is where this figure is read from: the
        // same query key, under the same scope the server named for this
        // reader. Not the deals list — the reading is a period's weighted
        // outlook, and a list of open deals is a different question that
        // happens to share a sum. `openAnalyticsSection` is the ONE spelling of
        // that move, shared with the tab strip, so a second address built here
        // could not disagree with it.
        onOpen={() => openAnalyticsSection("forecast")}
        // The weighted figure and how much of the population carries a price,
        // because they are read together: a weighted number over a partly
        // priced population is a floor, and a reader who cannot see the second
        // cannot judge the first.
        detail={t("brief.readings.pipelineBasis", {
          weighted: formatMoneyCompact(
            data.weighted_minor,
            data.base_currency,
            locale,
          ),
          priced: formatNumber(data.priced_count, locale),
        })}
      />
      {tip.tip}
    </span>
  );
}

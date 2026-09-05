// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { navigate } from "../app/router";
import { StatCard } from "../design-system/atoms";
import { StatStrip } from "../design-system/statstrip";
import {
  formatDateTime,
  formatMoneyCompact,
  formatMoneyOrAbsent,
  formatNumber,
} from "../format/format";
import { viewerZone } from "../format/timezone";
import {
  type Locale,
  type Translator,
  useLocale,
  usePlural,
  useT,
} from "../i18n";
import { useAnalyticsContext } from "./analytics.context";
import { useForecastReadings } from "./forecast.queries";
import { WORKLIST_FILTER_PARAM } from "./worklist";
import { isUnprepared } from "./worklist.copy";
import type {
  Worklist,
  WorklistFilter,
  WorklistItem,
} from "./worklist.queries";

// The day's readings, on one plate.
//
// FIVE slots, and every one of them answerable — which is what changed. Two of
// the five were placeholders drawing an em dash: promises, because the
// commitments lane is unwired, and quota pace, because targets were retired from
// the product by founder decision. A row where two slots say "not tracked" is
// not a comparison a reader can make; it is three figures and two apologies, and
// the apologies were permanent.
//
// The rule that kept them was that a strip is read ACROSS, so a shrinking row
// could let a reader take a missing question for an answered one. That rule
// holds for a question the product intends to answer LATER. It does not hold for
// one the product decided not to ask: a slot that will never fill is not a
// pending answer, and drawing it forever teaches a reader to skip the row.
//
// So the count is the same and the content is not. The plate asks: what is
// urgent, what needs preparing, which leads are owed an answer, where the
// pipeline stands, and what is waiting on a decision.
//
// FOUR OF THE FIVE come from the ONE worklist answer the queue below is drawn
// from, so no second read can put a different number beside the same rows. The
// pipeline outlook is the exception and is read separately, through the same
// query key Analytics uses — one answer to "what is the pipeline worth",
// wherever it is asked.
//
// The floor caveat is the plate's own, through `StatStrip`'s `floor` slot — the
// row is read across as one statement, so a caveat belonging to one figure would
// invite the reading where the others are exact.

const MEETINGS = "meetings";
const LEADS = "leads";

// Open the worklist on the lane a reading counted.
//
// Each figure in this strip IS one of the queue's filter pills counted, so the
// reading's door is that lane. It goes in the QUERY rather than the path because
// `#/worklist/<owner>` is already an address the team board navigates to, and
// `routeIdentity` ignores the query half — so this narrows the view without
// remounting the screen, and leaves an address somebody can paste.
function openLane(filter: WorklistFilter): void {
  navigate({ screen: "worklist" }, new Map([[WORKLIST_FILTER_PARAM, filter]]));
}

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
          label={t("home.readings.urgent")}
          value={formatNumber(day.summary.urgent, locale)}
          tone={day.summary.urgent > 0 ? "warn" : undefined}
          // The SUMMARY's own count, not one lane's. `urgent` is every row at
          // the top two levels — somebody waiting or a promise breaking — and
          // the morning's first question is how many of those there are, not
          // how many came from one producer.
          //
          // The basis line says what the figure was taken over, on every day. A
          // zero already reads as "none"; a line under it repeating that says
          // the same thing twice and drops the one fact it could add.
          detail={t("home.readings.urgentBasis")}
          openLabel={t("home.readings.openLane")}
          onOpen={() => openLane("all")}
        />
        <MeetingsStat
          meetings={meetings.meetings}
          unready={meetings.unready}
          locale={locale}
          t={t}
          onOpen={() => openLane("meetings")}
        />
        <LeadsStat
          leads={readings.prospecting}
          soonest={soonest}
          locale={locale}
          t={t}
          onOpen={() => openLane("leads")}
        />
        <PipelineOutlook />
        <StatCard
          numeric
          label={t("home.readings.decisions")}
          value={formatNumber(readings.review, locale)}
          tone={readings.review > 0 ? "warn" : undefined}
          detail={t("home.readings.decisionsBasis")}
          openLabel={t("home.readings.openLane")}
          onOpen={() => openLane("decisions")}
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
  onOpen,
}: Readonly<{
  onOpen: () => void;
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
      openLabel={t("home.readings.openLane")}
      onOpen={onOpen}
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
  onOpen,
}: Readonly<{
  onOpen: () => void;
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
      openLabel={t("home.readings.openLane")}
      onOpen={onOpen}
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

// Where the pipeline stands: what is open, and what it is worth weighted.
//
// TWO FIGURES, NEITHER OF THEM A TARGET. The slot this replaces said "Quota
// pace — no target is set", which was a permanent apology for a question the
// product decided not to ask. What a rep can actually be told is what the
// pipeline holds, and the honest version of that is both numbers: `open` is the
// face value of every open deal, `weighted` applies each deal's own probability.
// One without the other invites the reader to treat a face value as a forecast.
//
// The card NEVER says on track, attainment, or gap. There is no authoritative
// target in this product to compare against — the quota table was dropped by
// founder decision — so any such word would be inventing the thing that was
// removed.
//
// READ THROUGH THE SAME KEY ANALYTICS USES. Two surfaces asking what the
// pipeline is worth must not get two answers, so this calls the shared hook
// rather than its own fetch.
function PipelineOutlook() {
  const t = useT();
  const { locale } = useLocale();
  // The reader's OWN pipeline, under the scope the SERVER names for them.
  //
  // `/analytics/context` answers `default_scope`, which is what Analytics starts
  // from too: a rep's own records, a manager's managed teams, the workspace for
  // a reader whose lens reaches it. Asking for it here is what makes the shared
  // key true rather than merely claimed — an earlier version spelled the
  // omission as a client-built `managed_teams` scope, which keyed as
  // "managed_teams" while Analytics keyed the same rep's read as "owner:<id>",
  // so the two surfaces held two cache entries for one identical request.
  //
  // It also stops the client constructing a scope it has no standing to name.
  // `managed_teams` is documented as a server ANSWER, never a request, and
  // building one meant filling its required `label` with an empty string —
  // a placeholder where the code had nothing true to put.
  const context = useAnalyticsContext();
  const readings = useForecastReadings(context.data?.default_scope);

  if (readings.isPending) {
    // KEEPS THE ROW'S SHAPE: a detail line, like every slot beside it. A card
    // one line shorter reflows the whole strip when the read lands, which is the
    // opposite of the property this plate's fixed slot count defends.
    return (
      <StatCard
        label={t("home.readings.pipeline")}
        value="—"
        detail={t("home.readings.pipelineReading")}
      />
    );
  }
  // The currency is what makes the money sayable, so its absence is read as an
  // unanswered question rather than as a figure.
  //
  // `base_currency` IS required of this response, and that is exactly why the
  // check is here: a 200 whose shape is not the one the contract promises is
  // another absent read, and reaching into it for the currency threw — taking
  // the whole of `#/home` down to the app's error boundary, where a reader sees
  // no strip, no feed and no rail rather than one em dash. A server too old to
  // send it, a projection that lost it, or a proxy answering the route with
  // something else all arrive this way.
  if (readings.isError || !readings.data?.base_currency) {
    // A read that did not land is not a pipeline of nothing. The em dash says
    // the question went unanswered, which is what the retired slots said and
    // the one case where that spelling is still true.
    return (
      <StatCard
        label={t("home.readings.pipeline")}
        value="—"
        detail={t("home.readings.pipelineUnread")}
      />
    );
  }
  const data = readings.data;
  return (
    <StatCard
      label={
        data.scope_kind === "workspace"
          ? t("home.readings.pipelineWorkspace")
          : t("home.readings.pipeline")
      }
      // formatMoneyCompact, not the full amount: its own doc says a strip slot
      // has about 110px and a full euro figure wraps mid-number or clips. Every
      // other money StatCard in the tree uses it.
      value={formatMoneyCompact(data.open_minor, data.base_currency, locale)}
      // The weighted figure and the completeness in one line, because they are
      // read together: a weighted number over a partly priced population is a
      // floor, and a reader who cannot see the second cannot judge the first.
      detail={t("home.readings.pipelineBasis", {
        weighted: formatMoneyOrAbsent(
          data.weighted_minor,
          data.base_currency,
          locale,
        ),
        priced: formatNumber(data.priced_count, locale),
        eligible: formatNumber(data.eligible_count, locale),
      })}
    />
  );
}

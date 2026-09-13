// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Split out of brief.readings.tsx on the 500-line cap. It is a different
// reading from the ones in the strip beside it: those count work waiting in
// this reader's queue, and this one values the pipeline through the same
// analytics key Analytics itself reads.

import { useRecordZone } from "../app/recordzone";
import { StatCard } from "../design-system/atoms";
import { useTooltip } from "../design-system/tooltip";
import { middayInstant } from "../format/calendarday";
import {
  formatDateAbbrev,
  formatMoneyCompact,
  formatNumber,
} from "../format/format";
import { quarterLabel } from "../format/quarter";
import { useLocale, useT } from "../i18n";
import { openAnalyticsSection } from "./analytics.address";
import { useAnalyticsContext } from "./analytics.context";
import { useForecastReadings } from "./forecast.queries";

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
// the full range — and, where the reading is the whole company's rather
// than this reader's, whose pipeline it is — goes on the cell's hover line.
//
// READ THROUGH THE SAME KEY ANALYTICS USES. Two surfaces asking what the
// pipeline is worth must not get two answers, so this calls the shared hook
// rather than its own fetch.
export function PipelineOutlook() {
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
    // The range and, where the figure is the whole company's, whose
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
        openLabel={t("brief.readings.openPipeline")}
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

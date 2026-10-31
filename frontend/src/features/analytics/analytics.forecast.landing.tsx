import type { components } from "../../api/schema";
import { EmptyState, StatCard } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { formatMoneyCompact, formatNumber } from "../../format/format";
import { formatMoneyOrWord } from "../../format/moneyword";
import { type Locale, useT } from "../../i18n";

type Readings = components["schemas"]["ForecastReadings"];
type Landing = NonNullable<Readings["landing"]>;
type Sufficiency = NonNullable<Readings["sufficiency"]>;

// The central 80% of finishes under the chances in use. It says nothing about
// whether the chances are right, so it never brackets the landing in words.
export type ForecastRange = Readonly<{ lowMinor: number; highMinor: number }>;

// Both ends or neither: one end alone is not a range a reader can act on.
export function rangeOf(
  lowMinor: number | undefined,
  highMinor: number | undefined,
): ForecastRange | undefined {
  if (lowMinor === undefined || highMinor === undefined) return undefined;
  // Equal ends mean nothing is left to decide, and the landing already says it.
  if (lowMinor === highMinor) return undefined;
  return { lowMinor, highMinor };
}

// Basis points to whole percent, which is the only rounding this surface does.
//
// The server sends basis points so it chooses no rounding for its clients; a
// reader comparing coverage across periods wants whole numbers, and a tenth of
// a percent of coverage is precision nobody acts on.
const BASIS_POINTS_PER_PERCENT = 100;

// Where the period lands, and the two halves that make it.
//
// Drawn as a sentence and not only a figure, because the sum is the part a
// reader has to trust: "won plus still to come" says what was added, and a
// manager's call says that nothing was added at all.
export function LandingCard({
  landing,
  currency,
  locale,
  range,
  onOpenDeals,
}: Readonly<{
  landing: Landing;
  currency: string;
  locale: Locale;
  range?: ForecastRange;
  // The deals and chances the range was drawn from, where the page holds them.
  onOpenDeals?: () => void;
}>) {
  const t = useT();
  // Compact, because a slot of a strip is about a hundred points wide and a
  // full amount clips there — and a WORD where the pair cannot be said as money
  // at all, because a slot compared across a row must not answer with a glyph.
  const money = (minor: number) =>
    formatMoneyOrWord(
      minor,
      currency,
      locale,
      t("format.notForecast"),
      formatMoneyCompact,
    );

  // A call REPLACES the projection rather than adding to what is won, so the
  // two cases get different sentences. One sentence with a swapped number
  // would tell a reader the call was the remainder, which is the misreading
  // this whole shape exists to prevent.
  //
  // ONE label over both, with the measure leading the detail: "Landing" is the
  // reading, and which measure produced it qualifies the reading the way a
  // basis does. A label that changed with the measure made one figure read as
  // two different readings between two periods.
  const detail =
    landing.measure === "manager_call"
      ? t("forecast.landingFromCall", { won: money(landing.won_minor) })
      : t("forecast.landingFrom", {
          won: money(landing.won_minor),
          remaining: money(landing.remaining_minor),
        });

  return (
    <>
      <StatCard
        // The strip these two sit at the end of folds to rows on a phone, and
        // the fold is the ROW's: a slot that did not declare it would keep its
        // box while its neighbours lost theirs.
        narrow="row"
        label={t("forecast.landing")}
        value={money(landing.amount_minor)}
        // The range alone can take both of the detail's two lines in a slot of
        // three, so beside it the sum moves into the receipt.
        detail={
          range
            ? t("forecast.range", {
                low: money(range.lowMinor),
                high: money(range.highMinor),
              })
            : detail
        }
        basis={
          range && (
            <>
              <p>{detail}</p>
              <p>{t("forecast.rangeBasis")}</p>
            </>
          )
        }
        onOpen={range ? onOpenDeals : undefined}
      />

      {/* Its OWN heading, not the card's: the caveat sat under a repeat of
          `forecast.landing` and read as a second copy of the figure's label
          rather than as a remark about it. */}
      {landing.caveat && (
        <Callout
          tone="warning"
          kind="standing"
          title={t("forecast.landingCaveat")}
        >
          {t(`forecast.landing.caveat.${landing.caveat}`)}
        </Callout>
      )}
    </>
  );
}

// Beside a manager's call, what the deal records alone project. The call
// replaced it, so the two stand apart and are never added. Its own slot of the
// strip, because the strip lays out one column per child.
export function RecordsLandingCard({
  systemMinor,
  currency,
  locale,
}: Readonly<{ systemMinor: number; currency: string; locale: Locale }>) {
  const t = useT();
  return (
    <StatCard
      narrow="row"
      label={t("forecast.landingRecords")}
      value={formatMoneyOrWord(
        systemMinor,
        currency,
        locale,
        t("format.notForecast"),
        formatMoneyCompact,
      )}
      detail={t("forecast.landingRecordsDetail")}
    />
  );
}

// Whether the open deals support the reference landing.
//
// An absence renders as a SENTENCE and no figure. Drawing zeroes beside "no
// basis" would read as a fully covered book, which is the opposite of what an
// absence means.
export function SufficiencyCard({
  sufficiency,
  currency,
  locale,
}: Readonly<{ sufficiency: Sufficiency; currency: string; locale: Locale }>) {
  const t = useT();
  if (sufficiency.absent) {
    return (
      // It stands WHERE a stat card would, so it is the shaped instructional
      // empty state rather than a notice about a card that is not there.
      <EmptyState title={t("forecast.pipelineAbsentTitle")}>
        {t(`forecast.pipelineAbsent.${sufficiency.absent}`)}
      </EmptyState>
    );
  }

  // Every figure below travels together or not at all — the server sends the
  // basis with them, and an assessment carrying a need but no basis is a
  // number a reader cannot argue with.
  const needed = sufficiency.needed_open_minor;
  const current = sufficiency.current_open_minor;
  const reference = sufficiency.reference_landing_minor;
  const coverage = sufficiency.coverage_bp;
  if (
    sufficiency.basis === undefined ||
    needed === undefined ||
    current === undefined ||
    reference === undefined ||
    coverage === undefined
  ) {
    return null;
  }

  // Compact, because a slot of a strip is about a hundred points wide and a
  // full amount clips there — and a WORD where the pair cannot be said as money
  // at all, because a slot compared across a row must not answer with a glyph.
  const money = (minor: number) =>
    formatMoneyOrWord(
      minor,
      currency,
      locale,
      t("format.notForecast"),
      formatMoneyCompact,
    );
  const percent = Math.round(coverage / BASIS_POINTS_PER_PERCENT);

  return (
    <StatCard
      narrow="row"
      label={
        sufficiency.basis === "historical_median"
          ? t("reporting.historicalBenchmark")
          : t("forecast.pipelineNeeded")
      }
      value={money(needed)}
      // TWO lines and no more: the share, then the two figures it was drawn
      // from. The need itself is already the value above, and WHICH measure the
      // reference came from is the card's receipt — a fragment of it here cost
      // the line a third row, which the clamp then took away in German.
      detail={
        <>
          <span>
            {t("forecast.coverage", { percent: formatNumber(percent, locale) })}
          </span>
          <span>
            {t("forecast.pipelineNeededDetail", {
              open: money(current),
              landing: money(reference),
            })}
          </span>
        </>
      }
      basis={<p>{t(`forecast.pipelineBasisWhy.${sufficiency.basis}`)}</p>}
      // The bar is the share of what this needs that the open deals actually
      // hold, clamped at the track: a book at three times its requirement
      // would otherwise draw a bar three times the width of its own card.
      meter={{ filled: Math.min(current, needed), total: needed }}
    />
  );
}

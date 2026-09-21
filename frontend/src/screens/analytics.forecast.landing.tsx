import type { components } from "../api/schema";
import { EmptyState, StatCard } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { formatMoneyCompact, formatNumber } from "../format/format";
import { type Locale, type Translator, useT } from "../i18n";

type Readings = components["schemas"]["ForecastReadings"];
type Landing = NonNullable<Readings["landing"]>;
type Sufficiency = NonNullable<Readings["sufficiency"]>;

// Basis points to whole percent, which is the only rounding this surface does.
//
// The server sends basis points so it chooses no rounding for its clients; a
// reader comparing coverage across periods wants whole numbers, and a tenth of
// a percent of coverage is precision nobody acts on.
const BASIS_POINTS_PER_PERCENT = 100;

/**
 * A money reading for a forecast SLOT: compact, and a WORD where there is no
 * figure to compact.
 *
 * Compact because a slot of a strip is about a hundred points wide and a full
 * amount clips there — "€201,099.0" is a different number rather than a smaller
 * rendering of the right one. A word rather than the em dash `formatMoneyOrAbsent`
 * returns, because every stat card spells its emptiness: a glyph where a figure
 * belongs reads as a slot that failed to draw.
 *
 * Exported because the strip around these cards (analytics.forecast.tsx) draws
 * its own three slots from the same readings, and two slots of ONE row
 * formatting money two ways is the comparison the row exists to make coming
 * apart.
 */
export function slotMoney(
  minor: number | null | undefined,
  currency: string | null | undefined,
  locale: Locale,
  t: Translator,
): string {
  if (minor == null || !currency) {
    return t("format.notForecast");
  }
  return formatMoneyCompact(minor, currency, locale);
}

// Where the period lands, and the two halves that make it.
//
// Drawn as a sentence and not only a figure, because the sum is the part a
// reader has to trust: "won plus still to come" says what was added, and a
// manager's call says that nothing was added at all.
export function LandingCard({
  landing,
  currency,
  locale,
}: Readonly<{ landing: Landing; currency: string; locale: Locale }>) {
  const t = useT();
  const money = (minor: number) => slotMoney(minor, currency, locale, t);

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
        label={t("forecast.landing")}
        value={money(landing.amount_minor)}
        detail={detail}
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

  const money = (minor: number) => slotMoney(minor, currency, locale, t);
  const percent = Math.round(coverage / BASIS_POINTS_PER_PERCENT);

  return (
    <StatCard
      label={t("forecast.pipelineNeeded")}
      value={money(needed)}
      // TWO lines, not one joined sentence: the share and the two figures it
      // was drawn from are separate facts, and the need itself is already the
      // value above — repeating it in its own detail said one number twice.
      detail={
        <>
          <span>
            {t("forecast.coverage", { percent: formatNumber(percent, locale) })}
          </span>
          <span>
            {[
              t("forecast.pipelineNeededDetail", {
                open: money(current),
                landing: money(reference),
              }),
              // The measure the reference came from, as the fragment that
              // qualifies it. The whole sentence behind it is the card's
              // receipt, below.
              t(`forecast.pipelineBasis.${sufficiency.basis}`),
            ].join(" · ")}
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

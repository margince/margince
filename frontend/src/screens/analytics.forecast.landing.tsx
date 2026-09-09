import type { components } from "../api/schema";
import { EmptyState, StatCard } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { formatMoneyOrAbsent, formatNumber } from "../format/format";
import { type Locale, useT } from "../i18n";

type Readings = components["schemas"]["ForecastReadings"];
type Landing = NonNullable<Readings["landing"]>;
type Sufficiency = NonNullable<Readings["sufficiency"]>;

// Basis points to whole percent, which is the only rounding this surface does.
//
// The server sends basis points so it chooses no rounding for its clients; a
// reader comparing coverage across periods wants whole numbers, and a tenth of
// a percent of pipeline coverage is precision nobody acts on.
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
}: Readonly<{ landing: Landing; currency: string; locale: Locale }>) {
  const t = useT();
  const money = (minor: number) => formatMoneyOrAbsent(minor, currency, locale);

  // A call REPLACES the projection rather than adding to what is won, so the
  // two cases get different sentences. One sentence with a swapped number
  // would tell a reader the call was the remainder, which is the misreading
  // this whole shape exists to prevent.
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
        numeric
      />
      {/* Its OWN heading, not the card's: the caveat sat under a repeat of
          `forecast.landing` and read as a second copy of the figure's label
          rather than as a remark about it. */}
      {landing.caveat && (
        <Callout
          tone="warn"
          kind="standing"
          title={t("forecast.landingCaveat")}
        >
          {t(`forecast.landing.caveat.${landing.caveat}`)}
        </Callout>
      )}
    </>
  );
}

// Whether the open pipeline supports the reference landing.
//
// An absence renders as a SENTENCE and no figure. Drawing zeroes beside "no
// basis" would read as a fully covered pipeline, which is the opposite of what
// an absence means.
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

  const money = (minor: number) => formatMoneyOrAbsent(minor, currency, locale);
  const percent = Math.round(coverage / BASIS_POINTS_PER_PERCENT);

  return (
    <StatCard
      label={t("forecast.pipelineNeeded")}
      value={money(needed)}
      detail={[
        t("forecast.coverage", { percent: formatNumber(percent, locale) }),
        t("forecast.pipelineNeededDetail", {
          current: money(current),
          needed: money(needed),
          reference: money(reference),
        }),
        t(`forecast.pipelineBasis.${sufficiency.basis}`),
      ].join(" ")}
      // The bar is the share of what this needs that the pipeline actually
      // holds, clamped at the track: a book at three times its requirement
      // would otherwise draw a bar three times the width of its own card.
      meter={{ filled: Math.min(current, needed), total: needed }}
      numeric
    />
  );
}

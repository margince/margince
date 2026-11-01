import type { components } from "../../api/schema";
import { EmptyState, StatCard } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { formatMoneyCompact, formatNumber } from "../../format/format";
import { formatMoneyOrWord } from "../../format/moneyword";
import {
  type Locale,
  type Translator,
  useLocale,
  usePlural,
  useT,
} from "../../i18n";

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
  return { lowMinor, highMinor };
}

// Basis points to whole percent, which is the only rounding this surface does.
//
// The server sends basis points so it chooses no rounding for its clients; a
// reader comparing coverage across periods wants whole numbers, and a tenth of
// a percent of coverage is precision nobody acts on.
const BASIS_POINTS_PER_PERCENT = 100;

type Money = (minor: number) => string;

/**
 * The two lines that say what a landing counts: its basis first, then the
 * figure that stands beside it. A call replaces the projection and is never
 * added to what is won; a call beside a projection is named, not added.
 * Without a landing the basis is unknown and said so, never guessed.
 */
export function landingBasisLines(
  landing: Landing | undefined,
  callMinor: number | undefined,
  money: Money,
  t: Translator,
): Readonly<{ basis: string; beside?: string; warning: boolean }> {
  if (!landing) return { basis: t("forecast.basis.unknown"), warning: false };
  const basis = t(`forecast.basis.${landing.measure}`);
  if (landing.caveat === "call_below_actual")
    return {
      basis,
      beside: t("forecast.callBelowLine", { won: money(landing.won_minor) }),
      warning: true,
    };
  if (landing.caveat === "call_absent")
    return { basis, beside: t("forecast.callAbsentLine"), warning: false };
  if (landing.measure === "manager_call")
    return {
      basis,
      beside:
        landing.system_minor == null
          ? undefined
          : t("forecast.recordsProject", {
              amount: money(landing.system_minor),
            }),
      warning: false,
    };
  return {
    basis,
    beside:
      callMinor == null
        ? t("forecast.split", {
            won: money(landing.won_minor),
            remaining: money(landing.remaining_minor),
          })
        : t("forecast.callNotAdded", { call: money(callMinor) }),
    warning: false,
  };
}

// Where the period lands, labelled with what it counts. The modelled range is
// its own reading beside it, never this figure's uncertainty.
export function LandingCard({
  landing,
  currency,
  locale,
  callMinor,
}: Readonly<{
  landing: Landing;
  currency: string;
  locale: Locale;
  // A standing call beside a projection, stated and never added.
  callMinor?: number;
}>) {
  const t = useT();
  // Compact, because a slot is about a hundred points wide and a full amount
  // clips there. A word where the pair cannot be said as money.
  const money = (minor: number) =>
    formatMoneyOrWord(
      minor,
      currency,
      locale,
      t("format.notForecast"),
      formatMoneyCompact,
    );
  const lines = landingBasisLines(landing, callMinor, money, t);
  return (
    <>
      <StatCard
        // The strip folds to rows on a phone, and the fold is the ROW's: a
        // slot that did not declare it would keep its box.
        narrow="row"
        label={t("forecast.landing")}
        value={money(landing.amount_minor)}
        tone={lines.warning ? "warning" : undefined}
        detail={
          <>
            <span>{lines.basis}</span>
            {lines.beside && <span>{lines.beside}</span>}
          </>
        }
      />
      {/* Its own heading, not the card's, so the caveat reads as a remark
          about the figure rather than a second copy of its label. */}
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

/**
 * The modelled finish as a reading of its own: won money plus each priced
 * open deal won or lost by its chance. It names what it includes and leaves
 * out, and says when no range was drawn.
 */
export function ModelRangeCard({
  label,
  range,
  wonMinor,
  unpriced,
  fxMissing,
  money,
  onOpen,
}: Readonly<{
  label: string;
  range: ForecastRange | undefined;
  wonMinor?: number;
  unpriced: number;
  fxMissing: number;
  money: Money;
  onOpen?: () => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  if (!range)
    return (
      <StatCard
        narrow="row"
        label={label}
        value={t("forecast.model.none")}
        detail={t("forecast.model.noneDetail")}
      />
    );
  const left = [
    unpriced > 0 &&
      plural("forecast.model.unpriced", unpriced, {
        count: formatNumber(unpriced, locale),
      }),
    fxMissing > 0 &&
      plural("forecast.model.fxMissing", fxMissing, {
        count: formatNumber(fxMissing, locale),
      }),
  ].filter((line): line is string => typeof line === "string");
  return (
    <StatCard
      narrow="row"
      label={label}
      // Equal ends leave nothing to the model, so they read as one amount.
      value={
        range.lowMinor === range.highMinor
          ? money(range.lowMinor)
          : t("forecast.model.value", {
              low: money(range.lowMinor),
              high: money(range.highMinor),
            })
      }
      detail={
        <>
          <span>{t("forecast.model.detail")}</span>
          <span>
            {left[0] ??
              (wonMinor == null
                ? undefined
                : t("forecast.model.includes", { won: money(wonMinor) }))}
          </span>
        </>
      }
      basis={
        <>
          <p>{t("forecast.model.basis")}</p>
          {wonMinor != null && (
            <p>{t("forecast.model.includes", { won: money(wonMinor) })}</p>
          )}
          {left.map((line) => (
            <p key={line}>{line}</p>
          ))}
        </>
      }
      onOpen={onOpen}
    />
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

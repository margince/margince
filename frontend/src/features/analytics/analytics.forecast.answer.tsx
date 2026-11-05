import type { components } from "../../api/schema";
import { StatCard } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { Panel, PanelBody } from "../../design-system/panel";
import { SegmentBar } from "../../design-system/readings";
import { StatStrip } from "../../design-system/statstrip";
import {
  formatAnalyticsMoney,
  formatAnalyticsMoneyOrAbsent,
  formatAnalyticsNumber,
  formatDateAbbrev,
  formatPercent,
} from "../../format/format";
import { formatMoneyOrWord } from "../../format/moneyword";
import { type Locale, useT } from "../../i18n";
import {
  LandingCard,
  ModelRangeCard,
  RecordsLandingCard,
  rangeOf,
  SufficiencyCard,
} from "./analytics.forecast.landing";
import { QualifiedAnswer } from "./analytics.forecast.qualified";
import type { ForecastPeriod } from "./forecast.queries";
import "./analytics.css";

type Readings = components["schemas"]["ForecastReadings"];

// Compare the period forecast with won sales plus the Likely deals.
function callDetail(
  call: NonNullable<Readings["current_call"]>,
  readings: Readings,
  locale: Locale,
  t: ReturnType<typeof useT>,
): string {
  // The day the call was authored, in the period's own zone: a figure and the
  // date beside it are bucketed on one calendar.
  const date = formatDateAbbrev(call.created_at, locale, readings.timezone);
  const difference =
    call.amount_minor - (readings.won_minor + readings.evidence_minor);
  if (difference === 0) {
    return t("forecast.currentCallDetailEven", { date });
  }
  const gap = formatMoneyOrWord(
    Math.abs(difference),
    readings.base_currency,
    locale,
    t("format.notForecast"),
    formatAnalyticsMoney,
  );
  return t(
    difference > 0
      ? "forecast.currentCallDetailOver"
      : "forecast.currentCallDetailUnder",
    { date, gap },
  );
}

// The answer, in one sentence and then in three readings. A shared view has no
// deals panel, so it passes no `onOpenDeals`.
export function ForecastAnswer({
  readings,
  locale,
  period,
  landingUnrecorded = false,
  onOpenDeals,
  landing = true,
  density,
}: Readonly<{
  readings: Readings;
  locale: Locale;
  period: ForecastPeriod;
  // A snapshot frozen before its landing was kept: the basis is unknown.
  landingUnrecorded?: boolean;
  onOpenDeals?: () => void;
  // False when the page places the landing readings elsewhere, beside these.
  landing?: boolean;
  density?: "plate";
}>) {
  const t = useT();
  const currency = readings.base_currency;
  const money = (minor: number | null | undefined) =>
    formatAnalyticsMoneyOrAbsent(minor ?? null, currency, locale);
  const slot = (minor: number | null | undefined) =>
    formatMoneyOrWord(
      minor,
      currency,
      locale,
      t("format.notForecast"),
      formatAnalyticsMoney,
    );
  const call = readings.current_call;
  const threshold = formatPercent(readings.likely_threshold / 100, locale);

  return (
    <>
      {/* The screen's own answer, so it is the screen's own section: a Callout
          says something ABOUT a surface, and this IS the surface's content —
          the question as the panel's title and the answer as its body. */}
      <Panel title={t("forecast.question")}>
        <PanelBody>
          {/* The call and the supported figure, and the gap between them. A
              reader shown only one of the two has no way to tell whether the
              call is ahead of the evidence or behind it. */}
          <p>
            {call
              ? t("forecast.answerWithCall", {
                  call: money(call.amount_minor),
                  evidence: money(readings.won_minor + readings.evidence_minor),
                })
              : t("forecast.answerNoCall", {
                  evidence: money(readings.won_minor + readings.evidence_minor),
                })}
          </p>
          {/* The same answer as one shape: what is banked, what is Likely, and
              the call across them. The two are DISJOINT on the server, since
              Likely counts open deals only, so they are laid end to end. */}
          <SegmentBar
            label={t("forecast.makeup")}
            parts={[
              {
                key: "won",
                label: t("forecast.alreadyWon"),
                value: readings.won_minor,
                amount: slot(readings.won_minor),
              },
              {
                key: "evidence",
                label: t("forecast.likely"),
                value: readings.evidence_minor,
                amount: slot(readings.evidence_minor),
              },
            ]}
            marker={
              call
                ? {
                    key: "call",
                    label: t("forecast.currentCall"),
                    value: call.amount_minor,
                    amount: slot(call.amount_minor),
                  }
                : undefined
            }
          />
          {call && (
            <p className="t-caption">
              {callDetail(call, readings, locale, t)}
              {call.baseline === "unavailable" &&
                ` ${t("forecast.callHistoryUnavailable")}`}
            </p>
          )}
          <QualifiedAnswer readings={readings} money={money} />
        </PanelBody>
      </Panel>

      {/* The two readings of the open deals. Weighted is not a segment above:
          it counts the Likely deals too, at their probability. */}
      <StatStrip density={density}>
        <StatCard
          narrow="row"
          label={t("forecast.likely")}
          value={slot(readings.evidence_minor)}
          detail={t("forecast.likely.help", { threshold })}
        />
        <StatCard
          narrow="row"
          label={t("forecast.weighted")}
          value={slot(readings.weighted_minor)}
          detail={
            <>
              <span>{t("forecast.weighted.help")}</span>
              <span>
                {t("forecast.bySource", {
                  stage: money(readings.by_source.stage.weighted_minor),
                  human: money(readings.by_source.human.weighted_minor),
                  margince: money(readings.by_source.margince.weighted_minor),
                })}
              </span>
            </>
          }
        />
      </StatStrip>

      {/* An unpriced deal is a real deal contributing zero money, so the gap
          between eligible and priced is stated beside the total rather than
          left in the receipt alone. */}
      {readings.priced_count < readings.eligible_count && (
        <Callout
          tone="warning"
          kind="standing"
          title={t("forecast.partialTitle")}
        >
          {t("forecast.partial", {
            priced: formatAnalyticsNumber(readings.priced_count, locale),
            eligible: formatAnalyticsNumber(readings.eligible_count, locale),
          })}
        </Callout>
      )}

      {landing && (
        <ForecastLanding
          readings={readings}
          locale={locale}
          period={period}
          landingUnrecorded={landingUnrecorded}
          onOpenDeals={onOpenDeals}
          density={density}
        />
      )}
    </>
  );
}

/**
 * Where the period lands: the landing, the modelled range, what the records
 * say and the deal value still needed.
 */
export function ForecastLanding({
  readings,
  locale,
  period,
  landingUnrecorded = false,
  onOpenDeals,
  density,
  title,
}: Readonly<{
  readings: Readings;
  locale: Locale;
  period: ForecastPeriod;
  landingUnrecorded?: boolean;
  onOpenDeals?: () => void;
  density?: "plate";
  // Set where the readings stand as a section of their own, as in a rail.
  title?: string;
}>) {
  const t = useT();
  const currency = readings.base_currency;
  const slot = (minor: number | null | undefined) =>
    formatMoneyOrWord(
      minor,
      currency,
      locale,
      t("format.notForecast"),
      formatAnalyticsMoney,
    );
  const range = rangeOf(readings.range_low_minor, readings.range_high_minor);
  // Every card declares the narrow shape, since the fold is the strip's. A
  // managed-teams reading has neither landing nor sufficiency: summed over
  // separate books, neither would describe any of them. A share's snapshot may
  // predate the landing, so a range alone keeps the strip.
  if (!(readings.landing || readings.sufficiency || range || landingUnrecorded))
    return null;
  const strip = (
    <StatStrip density={density}>
      {readings.landing ? (
        <LandingCard
          landing={readings.landing}
          currency={currency}
          locale={locale}
          callMinor={
            readings.landing.measure === "manager_call"
              ? undefined
              : readings.current_call?.amount_minor
          }
        />
      ) : (
        landingUnrecorded && (
          <StatCard
            narrow="row"
            label={t("forecast.landing")}
            value={t("forecast.landingUnrecorded")}
            detail={t("forecast.basis.unknown")}
          />
        )
      )}
      {(readings.landing || range) && (
        <ModelRangeCard
          label={t(`forecast.model.${period}`)}
          range={range}
          wonMinor={readings.won_minor}
          unpriced={readings.eligible_count - readings.priced_count}
          fxMissing={readings.fx_missing_count}
          money={slot}
          onOpen={onOpenDeals}
        />
      )}
      {readings.landing?.system_minor != null && (
        <RecordsLandingCard
          systemMinor={readings.landing.system_minor}
          currency={currency}
          locale={locale}
        />
      )}
      {readings.sufficiency && (
        <SufficiencyCard
          sufficiency={readings.sufficiency}
          currency={currency}
          locale={locale}
        />
      )}
    </StatStrip>
  );
  return title ? (
    <Panel title={title} flatHead>
      {strip}
    </Panel>
  ) : (
    strip
  );
}

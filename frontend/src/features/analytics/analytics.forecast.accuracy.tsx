// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { FactList } from "../../design-system/factlist";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import {
  formatAnalyticsNumber,
  formatDate,
  formatMoneyWhole,
  formatPercent,
} from "../../format/format";
import { type Locale, useLocale, usePlural, useT } from "../../i18n";
import { STAGE_CHANCES_KEY } from "../../lib/activitykeys";
import { useCompanyZone } from "../../lib/companyzone";
import { unwrap } from "../../lib/problem";

type Accuracy = components["schemas"]["ForecastAccuracy"];

/** The forecast's record so far: what it said, against what happened. */
export function useForecastAccuracy() {
  return useQuery({
    queryKey: [...STAGE_CHANCES_KEY, "accuracy"],
    queryFn: async () => unwrap(await api.GET("/forecast/stage-chances")),
    select: (data) => data.accuracy,
  });
}

/**
 * How accurate the forecast has been. One part is the share of closed deals
 * that were won at each chance the forecast gave them. The other is what each
 * finished quarter was said to land at, against what it landed at.
 */
export function ForecastAccuracy() {
  const t = useT();
  const { locale } = useLocale();
  const accuracy = useForecastAccuracy();
  return (
    <QueryGate query={accuracy} pendingLabel={t("forecast.accuracy.title")}>
      {(data) => (
        <Panel title={t("forecast.accuracy.title")}>
          <PanelBody>
            {data ? (
              <AccuracyBody accuracy={data} locale={locale} />
            ) : (
              <PanelIntro>{t("forecast.accuracy.none")}</PanelIntro>
            )}
          </PanelBody>
        </Panel>
      )}
    </QueryGate>
  );
}

function AccuracyBody({
  accuracy,
  locale,
}: Readonly<{ accuracy: Accuracy; locale: Locale }>) {
  const t = useT();
  const plural = usePlural();
  const zone = useCompanyZone();
  const { calibration, landings } = accuracy;
  if (calibration.deals === 0 && landings.length === 0) {
    return <PanelIntro>{t("forecast.accuracy.none")}</PanelIntro>;
  }
  const percent = (value: number) => formatPercent(value / 100, locale);
  return (
    <>
      {calibration.bands.length > 0 && (
        <>
          <PanelIntro>
            {plural("forecast.accuracy.calibration", calibration.deals, {
              count: formatAnalyticsNumber(calibration.deals, locale),
              days: formatAnalyticsNumber(calibration.lead_days, locale),
            })}
          </PanelIntro>
          <FactList
            numeric
            facts={calibration.bands.map((band) => ({
              key: `band-${band.from_percent}`,
              term: `${percent(band.from_percent)}–${percent(band.to_percent)}`,
              value:
                band.mean_percent == null || band.won_percent == null
                  ? plural("forecast.accuracy.fewDeals", band.deals, {
                      count: formatAnalyticsNumber(band.deals, locale),
                    })
                  : plural("forecast.accuracy.band", band.deals, {
                      count: formatAnalyticsNumber(band.deals, locale),
                      mean: percent(band.mean_percent),
                      won: percent(band.won_percent),
                    }),
            }))}
          />
        </>
      )}
      {landings.length > 0 && (
        <>
          <PanelIntro>{t("forecast.accuracy.landings")}</PanelIntro>
          <FactList
            numeric
            facts={landings.map((landing) => ({
              key: landing.period_end,
              term: formatDate(landing.period_end, locale, zone),
              value: t("forecast.accuracy.landing", {
                said: formatMoneyWhole(
                  landing.said_minor,
                  landing.currency,
                  locale,
                ),
                landed: formatMoneyWhole(
                  landing.landed_minor,
                  landing.currency,
                  locale,
                ),
              }),
              note: t("forecast.accuracy.saidOn", {
                date: formatDate(landing.said_on, locale, zone),
              }),
            }))}
          />
        </>
      )}
    </>
  );
}

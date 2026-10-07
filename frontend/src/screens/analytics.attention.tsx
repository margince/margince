// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useCan } from "../app/capability";
import { Button } from "../design-system/atoms";
import { Panel, PanelRow } from "../design-system/panel";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import { openAnalyticsSection, type Section } from "./analytics.address";
import type { AnalyticsScope } from "./analytics.context";
import { useDataCoverage } from "./analytics.coverage";
import { useInputChecks } from "./analytics.forecast.review";
import { useForecastReadings } from "./forecast.queries";

export type AttentionItem = Readonly<{
  key: string;
  title: string;
  detail: string;
  action: string;
  section: Section;
}>;

// What stands between this reader and numbers they can trust, each with the
// door to where it is fixed. The panel is absent when nothing does, because an
// empty "nothing needs you" panel is one more thing to read past every day.
export function AnalyticsAttention({
  scope,
}: Readonly<{ scope: AnalyticsScope }>) {
  const items = useAttentionItems(scope);
  return <AttentionPanel items={items} />;
}

export function AttentionPanel({
  items,
}: Readonly<{ items: readonly AttentionItem[] }>) {
  const t = useT();
  if (items.length === 0) {
    return null;
  }
  return (
    <Panel title={t("analytics.attention")}>
      {items.map((item) => (
        <PanelRow key={item.key} className="analytics-attention-row">
          <div className="analytics-attention-text">
            <p className="analytics-attention-title">{item.title}</p>
            <p className="t-caption">{item.detail}</p>
          </div>
          <Button onClick={() => openAnalyticsSection(item.section)}>
            {item.action}
          </Button>
        </PanelRow>
      ))}
    </Panel>
  );
}

// Each source is read under the key its own section reads it by, so opening
// that section shows the same answer this list summarised. A source that fails
// to load drops its line instead of the panel: the section it links to states
// the failure, and a summary is no place to restate it.
function useAttentionItems(scope: AnalyticsScope): AttentionItem[] {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const canReadForecast = useCan("forecast", "read");
  const checks = useInputChecks(canReadForecast);
  // An undefined scope holds the read, which is what a reader without the
  // forecast grant needs: no request, rather than a refused one.
  const readings = useForecastReadings(
    canReadForecast ? scope : undefined,
    "quarter",
  );
  const coverage = useDataCoverage();
  const canReadCoverage = useCan("data_coverage", "read");

  // A disabled query still hands back what it cached, so a grant withdrawn
  // mid-session is checked here as well as at the request.
  const items: AttentionItem[] = [];
  const findings = (canReadForecast && checks.data) || [];
  if (findings.length > 0) {
    items.push({
      key: "checks",
      title: plural("analytics.attentionChecks", findings.length, {
        count: formatNumber(findings.length, locale),
      }),
      detail: t("analytics.attentionChecksDetail"),
      action: t("analytics.attentionChecksAction"),
      section: "forecast",
    });
  }
  const forecast = canReadForecast ? readings.data : undefined;
  if (forecast && forecast.priced_count < forecast.eligible_count) {
    items.push({
      key: "unpriced",
      title: t("analytics.attentionUnpriced", {
        priced: formatNumber(forecast.priced_count, locale),
        eligible: formatNumber(forecast.eligible_count, locale),
      }),
      detail: t("analytics.attentionUnpricedDetail"),
      action: t("analytics.attentionUnpricedAction"),
      section: "forecast",
    });
  }
  const unread =
    (canReadCoverage &&
      coverage.data?.sources.filter((source) => source.state !== "checked")) ||
    [];
  if (unread.length > 0) {
    items.push({
      key: "coverage",
      title: plural("analytics.attentionCoverage", unread.length, {
        count: formatNumber(unread.length, locale),
      }),
      detail: t("reporting.coverageAction"),
      action: t("analytics.attentionCoverageAction"),
      section: "coverage",
    });
  }
  return items;
}

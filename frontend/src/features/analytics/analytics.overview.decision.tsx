// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { formatAnalyticsNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { metricOf } from "./analytics.overview.readings";
import type { ReportingEvaluation } from "./reporting.model";

export function useForecastCoverage(evaluation: ReportingEvaluation) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const metric = metricOf(evaluation, "forecast_landing");
  const basis = metric?.forecast_basis;
  if (!basis || metric?.value == null || metric.coverage.withheld) return null;
  const unpriced = Math.max(0, basis.eligible_count - basis.priced_count);
  const parts = [
    unpriced > 0
      ? plural("analytics.decision.unpriced", unpriced, {
          count: formatAnalyticsNumber(unpriced, locale),
        })
      : undefined,
    basis.fx_missing_count > 0
      ? plural("analytics.decision.fx", basis.fx_missing_count, {
          count: formatAnalyticsNumber(basis.fx_missing_count, locale),
        })
      : undefined,
  ].filter((part) => part !== undefined);
  if (!parts.length) return null;
  return {
    included:
      basis.included_count == null
        ? undefined
        : t("analytics.decision.included", {
            included: formatAnalyticsNumber(basis.included_count, locale),
            eligible: formatAnalyticsNumber(basis.eligible_count, locale),
          }),
    summary:
      basis.included_count == null
        ? undefined
        : t("analytics.outlook.valuedCount", {
            included: formatAnalyticsNumber(basis.included_count, locale),
            eligible: formatAnalyticsNumber(basis.eligible_count, locale),
          }),
    excluded: t("analytics.decision.excluded", { parts: parts.join(", ") }),
  };
}

export function ForecastCoverage({
  evaluation,
}: Readonly<{ evaluation: ReportingEvaluation }>) {
  const coverage = useForecastCoverage(evaluation);
  if (!coverage) return null;
  return (
    <>
      {coverage.included && <span>{coverage.included}</span>}
      <span>{coverage.excluded}</span>
    </>
  );
}

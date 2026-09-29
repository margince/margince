import type { components } from "../api/schema";
import { formatMoney, formatNumber } from "../format/format";
import type { Locale, Translator } from "../i18n";

export type ReportingSelection = components["schemas"]["ReportingSelection"];
export type ReportingEvaluation = components["schemas"]["ReportingEvaluation"];
export type ReportingChart = components["schemas"]["ReportingChart"];
export type ReportingEvidenceRef =
  components["schemas"]["ReportingEvidenceRef"];
export type ReportingReport = components["schemas"]["ReportingReport"];
export type ReportingMetricID = components["schemas"]["ReportingMetricID"];
export type ReportingBlockKind = components["schemas"]["ReportingBlockKind"];
export type ReportingEdition = components["schemas"]["ReportingEdition"];

export function reportingQuery(selection: ReportingSelection) {
  return {
    scope_kind:
      selection.scope.kind === "managed_teams"
        ? undefined
        : selection.scope.kind,
    scope_id: selection.scope.id,
    pipeline_id: selection.pipeline_id,
    period: selection.period,
    start_at: selection.interval?.start_at,
    end_at: selection.interval?.end_at,
    target_basis: selection.target_basis,
    close_window: selection.close_window,
    metrics: selection.metrics,
    blocks: selection.blocks,
  };
}

export function reportingAmount(
  value: number | null | undefined,
  unit: string,
  currency: string,
  locale: Locale,
): string {
  if (value == null) return "—";
  if (reportingMoneyUnit(unit, currency))
    return currency ? formatMoney(value, currency, locale) : "—";
  return `${formatNumber(value, locale)}${unit === "percent" ? "%" : ""}`;
}

export function metricLabel(metric: ReportingMetricID, t: Translator): string {
  return t(`reporting.${metric}`);
}
export function blockLabel(block: ReportingBlockKind, t: Translator): string {
  return t(`reporting.${block}`);
}

export function reportingMoneyUnit(unit: string, currency: string): boolean {
  return unit === "money" || (/^[A-Z]{3}$/.test(unit) && unit === currency);
}

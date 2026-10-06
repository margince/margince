import type { components } from "../api/schema";
import {
  formatDateAbbrev,
  formatDateTime,
  formatMoney,
  formatMoneyCompact,
  formatNumber,
} from "../format/format";
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

export function editionLabel(
  edition: ReportingEdition,
  locale: Locale,
): string {
  const context = edition.evaluation.context;
  return `${reportingPeriodLabel(context.interval, context.timezone, locale)} · ${formatDateTime(edition.captured_at, locale, context.timezone)}`;
}

export function executionLabel(
  status: string | undefined,
  t: Translator,
): string {
  switch (status) {
    case "pending":
    case "running":
    case "succeeded":
    case "partial":
    case "failed":
    case "suspended":
    case "skipped":
      return t(`reporting.execution.${status}`);
    default:
      return "";
  }
}

export function reportingCompactAmount(
  value: number | null | undefined,
  unit: string,
  currency: string,
  locale: Locale,
): string {
  return value != null && reportingMoneyUnit(unit, currency)
    ? currency
      ? formatMoneyCompact(value, currency, locale)
      : "—"
    : reportingAmount(value, unit, currency, locale);
}

export function reportingPeriodLabel(
  interval: { start_at: string; end_at: string },
  zone: string,
  locale: Locale,
): string {
  return `${formatDateAbbrev(interval.start_at, locale, zone)} – ${formatDateAbbrev(new Date(Date.parse(interval.end_at) - 1).toISOString(), locale, zone)}`;
}

export type ReportAction = {
  kind: "duplicate" | "archive" | "freeze";
  report: ReportingReport;
  key: string;
  name?: string;
};
export function editionStatus(
  edition: ReportingEdition,
  t: Translator,
): string {
  if (edition.expired) return t("reporting.expired");
  if (edition.redacted) return t("reporting.redacted");
  if (edition.withheld) return t("reporting.withheld");
  return edition.name;
}

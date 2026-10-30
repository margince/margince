import { useQuery } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button, EmptyState } from "../../design-system/atoms";
import { type ISODate, isoOrBlank } from "../../design-system/dateinput";
import { ErrorLine } from "../../design-system/errorline";
import { FilterBar } from "../../design-system/filterbar";
import { QueryGate } from "../../design-system/querystates";
import { formatDateTime } from "../../format/format";
import { startOfDayInZone } from "../../format/timezone";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { usePipelines } from "../../lib/pipelines.queries";
import { problemCodeOf, unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { navigate } from "../../lib/router";
import { replaceDial, useUrlParams } from "../../lib/urlstate";
import { AnalyticsAttention } from "./analytics.attention";
import type { AnalyticsScope } from "./analytics.context";
import { ReportingCharts } from "./reporting.charts";
import { ReportingEvidenceDrawer } from "./reporting.evidence";
import { ReportingExportButton } from "./reporting.export";
import { REPORTING_PERIODS, ReportingFilters } from "./reporting.filters";
import {
  type ReportingEvaluation,
  type ReportingEvidenceRef,
  type ReportingSelection,
  reportingPeriodIsEmpty,
  reportingQuery,
} from "./reporting.model";
import { useReportingCatalog } from "./reporting.queries";
import { SaveReportingDialog } from "./reporting.save";
import { type ReportingView, VIEW_SHAPE, viewRequest } from "./reporting.views";

type Catalog = components["schemas"]["ReportingCatalog"];

// Draws the empty-scope notice from the population flag of the answer beside it.
type EmptyNotice = (holdsDeals: boolean | undefined) => ReactNode;

// One view of Performance: the tab that opened it names which. The catalog and
// the pipelines are read under the keys every other surface shares, so a tab
// change asks the server for the evaluation alone.
export function ReportingOverview({
  view,
  scope,
  scopeControl,
  emptyNotice,
}: Readonly<{
  view: ReportingView;
  scope: AnalyticsScope;
  scopeControl?: ReactNode;
  emptyNotice?: EmptyNotice;
}>) {
  const t = useT();
  const catalog = useReportingCatalog();
  const pipelines = usePipelines();
  return (
    <QueryGate query={catalog} pendingLabel={t("reporting.performance")}>
      {(catalog) => (
        <QueryGate query={pipelines} pendingLabel={t("reporting.pipeline")}>
          {(pipelines) => (
            <OverviewBody
              view={view}
              scope={scope}
              scopeControl={scopeControl}
              emptyNotice={emptyNotice}
              catalog={catalog}
              pipelines={pipelines}
            />
          )}
        </QueryGate>
      )}
    </QueryGate>
  );
}

function useOverviewFilters(
  pipelines: readonly components["schemas"]["Pipeline"][],
) {
  const [params] = useUrlParams();
  const from = params.get("from") ?? "";
  const through = params.get("through") ?? "";
  const start: ISODate | "" = isoOrBlank(from);
  const end: ISODate | "" = isoOrBlank(through);
  const setStart = (value: string) => replaceDial("from", value);
  const setEnd = (value: string) => replaceDial("through", value);
  const period =
    REPORTING_PERIODS.find((value) => value === params.get("period")) ??
    "this_month";
  const setPeriod = (value: string) => replaceDial("period", value);
  const defaultPipeline =
    pipelines.find((pipeline) => pipeline.is_default)?.id ??
    pipelines[0]?.id ??
    "";
  const pipelineId =
    params.get("pipeline") === "all"
      ? ""
      : (pipelines.find((pipeline) => pipeline.id === params.get("pipeline"))
          ?.id ?? defaultPipeline);
  const setPipelineId = (value: string) =>
    replaceDial("pipeline", value || "all");
  const targetBasis: ReportingSelection["target_basis"] =
    period === "this_quarter" ? "fiscal_quarter" : "month";
  const setTargetBasis = (value: string) => replaceDial("target", value);
  const closeWindow: ReportingSelection["close_window"] =
    params.get("close") === "fiscal_quarter" ? "fiscal_quarter" : "all_open";
  const setCloseWindow = (value: string) => replaceDial("close", value);
  return {
    start,
    end,
    setStart,
    setEnd,
    period,
    setPeriod,
    pipelineId,
    setPipelineId,
    targetBasis,
    setTargetBasis,
    closeWindow,
    setCloseWindow,
  };
}

function OverviewBody({
  view,
  scope,
  scopeControl,
  emptyNotice,
  catalog,
  pipelines,
}: Readonly<{
  view: ReportingView;
  scope: AnalyticsScope;
  scopeControl?: ReactNode;
  emptyNotice?: EmptyNotice;
  catalog: Catalog;
  pipelines: readonly components["schemas"]["Pipeline"][];
}>) {
  const t = useT();
  const zone = useRecordZone();
  const { locale } = useLocale();
  const {
    start,
    end,
    setStart,
    setEnd,
    period,
    setPeriod,
    pipelineId,
    setPipelineId,
    targetBasis,
    setTargetBasis,
    closeWindow,
    setCloseWindow,
  } = useOverviewFilters(pipelines);
  const canSave = useCanWrite("report_definition", "create");
  const [saving, setSaving] = useState(false);
  const [evidence, setEvidence] = useState<ReportingEvidenceRef | null>(null);
  const shape = VIEW_SHAPE[view];
  const { metrics, blocks } = viewRequest(view, catalog, period);
  const selection: ReportingSelection = {
    scope: { kind: scope.kind, id: scope.id ?? undefined, label: scope.label },
    pipeline_id: shape.dealFilters ? pipelineId || undefined : undefined,
    period,
    interval: customInterval(period, start, end, zone),
    target_basis: targetBasis,
    // The contract asks for a close window on every selection. Only deal
    // figures read it, so other views keep the default and never the address's.
    close_window: shape.dealFilters ? closeWindow : "all_open",
    metrics,
    blocks,
  };
  const validDates = validDateRange(period, start, end);
  // A seat whose catalog holds none of the view's metrics has nothing to ask.
  const validPeriod = validDates && metrics.length > 0;
  const query = useQuery({
    enabled: validPeriod,
    queryKey: ["reporting-evaluation", selection],
    queryFn: async () => {
      return unwrap(
        await api.GET("/analytics/evaluate", {
          params: { query: reportingQuery(selection) },
        }),
      );
    },
  });
  const contextKey = JSON.stringify(selection);
  const invalidSelection =
    problemCodeOf(query.error) === "reporting_interval_invalid";
  return (
    <>
      <FilterBar
        label={t("analytics.filters")}
        actions={
          <>
            {query.data && <ReportingExportButton evaluation={query.data} />}
            {canSave && (
              <Button
                onClick={() => setSaving(true)}
                disabled={!query.isSuccess}
              >
                {t("reporting.save")}
              </Button>
            )}
          </>
        }
        caption={
          validDates
            ? resultsThrough(query.data, locale, t)
            : t("reporting.chooseDates")
        }
      >
        {scopeControl}
        <ReportingFilters
          selection={selection}
          showScope={false}
          showCloseWindow={shape.dealFilters}
          showPipeline={shape.dealFilters}
          showTargets={false}
          dates={{
            from: start,
            through: end,
            onChange: (from, through) => {
              setStart(from);
              setEnd(through);
              setEvidence(null);
            },
          }}
          onChange={(next) => {
            if (next.period !== period) {
              setPeriod(next.period);
              setTargetBasis(
                next.period === "this_quarter" ? "fiscal_quarter" : "month",
              );
            }
            if (shape.dealFilters && next.pipeline_id !== selection.pipeline_id)
              setPipelineId(next.pipeline_id ?? "");
            if (next.target_basis !== targetBasis)
              setTargetBasis(next.target_basis);
            if (next.close_window !== closeWindow)
              setCloseWindow(next.close_window);
            setEvidence(null);
          }}
        />
      </FilterBar>
      {metrics.length === 0 && (
        <EmptyState>{t("reporting.noViews")}</EmptyState>
      )}
      {invalidSelection && <ErrorLine error={query.error} />}
      {/* Its sources are its own, so a failed or unasked evaluation does not
          take the list down with it. */}
      {(!validPeriod || query.isError) && <AnalyticsAttention scope={scope} />}
      {validPeriod && !invalidSelection && (
        <QueryGate query={query} pendingLabel={t("reporting.performance")}>
          {(evaluation) => (
            <>
              {emptyNotice?.(evaluation.context.scope_holds_deals)}
              <ReportingCharts
                evaluation={evaluation}
                afterSummary={<AnalyticsAttention scope={scope} />}
                onEvidence={setEvidence}
              />
              {evidence && (
                <ReportingEvidenceDrawer
                  key={`${contextKey}:${JSON.stringify(evidence)}`}
                  evaluation={evaluation}
                  reference={evidence}
                  onClose={() => setEvidence(null)}
                />
              )}
              {saving && (
                <SaveReportingDialog
                  selection={evaluation.selection}
                  onClose={() => setSaving(false)}
                  onSaved={(report) => {
                    setSaving(false);
                    navigate({
                      screen: "analytics",
                      id: "reports",
                      id2: report.id,
                    });
                  }}
                />
              )}
            </>
          )}
        </QueryGate>
      )}
    </>
  );
}

function customInterval(
  period: string,
  start: ISODate | "",
  end: ISODate | "",
  zone: string,
): ReportingSelection["interval"] {
  if (period !== "custom" || !start || !end) return undefined;
  return {
    start_at: startOfDayInZone(start, zone),
    end_at: startOfDayInZone(end, zone, 1),
  };
}

function validDateRange(period: string, start: string, end: string): boolean {
  return period !== "custom" || !!(start && end);
}

// "Results through" only while the period is still running: a closed period
// ends where it says it does, and the line would only repeat it. A period read
// at its first instant holds no time, which is said rather than drawn as zeros.
function resultsThrough(
  evaluation: ReportingEvaluation | undefined,
  locale: Locale,
  t: Translator,
): string | undefined {
  if (
    !evaluation ||
    evaluation.context.interval.end_at !== evaluation.context.evaluated_at
  ) {
    return undefined;
  }
  if (reportingPeriodIsEmpty(evaluation.context.interval)) {
    return t("reporting.periodNotStarted", {
      at: formatDateTime(
        evaluation.context.interval.start_at,
        locale,
        evaluation.context.timezone,
      ),
    });
  }
  return t("reporting.resultsThrough", {
    at: formatDateTime(
      evaluation.context.interval.end_at,
      locale,
      evaluation.context.timezone,
    ),
  });
}

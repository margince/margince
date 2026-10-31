import "./reporting.css";
import { useQueryClient } from "@tanstack/react-query";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { formatDateTime, formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { cohortOutcomeLabel } from "./reporting.cohort";
import { EvidenceFrame } from "./reporting.evidenceframe";
import { EvidenceRecords } from "./reporting.evidencerecords";
import { breakdownLabel } from "./reporting.grouplabel";
import {
  type ReportingEvaluation,
  type ReportingEvidenceRef,
  reportingPeriodLabel,
} from "./reporting.model";
import { movementLabel } from "./reporting.movement";
import {
  reloadReportingFigures,
  useMetricNames,
  useReportingCatalog,
} from "./reporting.queries";

type ReportingCoverage = components["schemas"]["ReportingCoverage"];

/**
 * The one evidence drawer: what a figure measures, the time and scope it was
 * read for, its coverage, then the records behind it. The records are listed
 * under the receipt the figure was read with. A figure that failed, is not
 * supported or is hidden lists none and says why.
 */
export function ReportingEvidenceDrawer({
  evaluation,
  reference,
  editionId,
  onClose,
}: Readonly<{
  evaluation: ReportingEvaluation;
  reference: ReportingEvidenceRef;
  editionId?: string;
  onClose: () => void;
}>) {
  const t = useT();
  const metricName = useMetricNames();
  const { locale } = useLocale();
  const catalog = useReportingCatalog();
  const definition = catalog.data?.metrics.find(
    (metric) => metric.id === reference.metric,
  );
  const selected = evidenceSelection(evaluation, reference);
  const { metric, point, outcome, interval } = selected;
  const state = figureState(selected.coverage, selected.selectedValue);
  const stateLabel = editionId
    ? formatDateTime(
        evaluation.context.state_at,
        locale,
        evaluation.context.timezone,
      )
    : t("reporting.currentPipeline");
  return (
    <EvidenceFrame
      title={metricName(reference.metric)}
      onClose={onClose}
      lines={[
        evaluation.context.scope.label +
          (point?.label
            ? ` · ${reference.context_id === "movement_delta" ? movementLabel(point.key, point.label, t) : breakdownLabel(point.key, point.label, t)}`
            : "") +
          (outcome
            ? ` · ${cohortOutcomeLabel(outcome, t)} · ${formatNumber(outcome.count, locale)}`
            : ""),
        interval
          ? reportingPeriodLabel(
              { ...interval, end_at: reference.through ?? interval.end_at },
              evaluation.context.timezone,
              locale,
            )
          : stateLabel,
        t(
          editionId ? "reporting.evidenceFrozenAt" : "reporting.evidenceReadAt",
          {
            at: formatDateTime(
              evaluation.context.evaluated_at,
              locale,
              evaluation.context.timezone,
            ),
            currency: evaluation.context.currency,
          },
        ),
      ]}
    >
      <p>
        {editionId && definition && metric?.version !== definition.version
          ? t("reporting.earlierDefinition")
          : definition?.definition}
      </p>
      {selected.coverage && selected.coverage.status !== "ok" && (
        <p className="t-caption">
          {t(`reporting.status.${selected.coverage.status}`)}
          {selected.coverage.reason ? ` · ${selected.coverage.reason}` : ""}
        </p>
      )}
      {state === "unavailable" ? (
        <FigureUnavailable withheld={selected.coverage?.withheld === true} />
      ) : (
        <EvidenceRecords
          evaluation={evaluation}
          reference={reference}
          editionId={editionId}
          zero={state === "zero"}
          unit={selected.metric?.unit ?? definition?.unit ?? "count"}
          selectedValue={selected.selectedValue}
          onClose={onClose}
        />
      )}
    </EvidenceFrame>
  );
}

// Whether the records behind a figure can be listed. A failed, unsupported or
// unconfigured figure has none to list, and neither has one hidden from the
// reader: listing them, or counting them, would rebuild what is hidden.
type FigureState = "records" | "zero" | "unavailable";
const UNLISTABLE: ReadonlySet<string> = new Set([
  "failed",
  "unsupported",
  "unavailable",
  "not_configured",
]);
function figureState(
  coverage: ReportingCoverage | undefined,
  value: number | null | undefined,
): FigureState {
  if (!coverage) return "records";
  if (UNLISTABLE.has(coverage.status)) return "unavailable";
  if (coverage.withheld && value == null) return "unavailable";
  if (coverage.status === "no_data" || value === 0) return "zero";
  return "records";
}

function FigureUnavailable({ withheld }: Readonly<{ withheld: boolean }>) {
  const t = useT();
  const client = useQueryClient();
  if (withheld) return <p role="status">{t("reporting.evidenceWithheld")}</p>;
  return (
    <ErrorLine
      actions={
        <Button onClick={() => reloadReportingFigures(client)}>
          {t("common.retry")}
        </Button>
      }
    >
      {t("reporting.evidenceUnavailable")}
    </ErrorLine>
  );
}

function evidenceSelection(
  evaluation: ReportingEvaluation,
  reference: ReportingEvidenceRef,
) {
  const metric = evaluation.metrics.find(
    (metric) => metric.id === reference.metric,
  );
  const chart = evaluation.charts.find(
    (candidate) =>
      candidate.metric === reference.metric &&
      (candidate.context_id === reference.context_id ||
        candidate.points.some(
          (point) =>
            point.evidence?.context_id === reference.context_id &&
            point.evidence.group_key === reference.group_key &&
            point.evidence.through === reference.through,
        )),
  );
  const named =
    reference.group_key || reference.through
      ? chart?.points.find(
          (point) =>
            point.evidence?.group_key === reference.group_key &&
            point.evidence?.through === reference.through,
        )
      : undefined;
  // A cohort's outcome count names its own members inside a point.
  const counted = reference.group_key
    ? chart?.points
        .flatMap((point) =>
          (point.outcomes ?? []).map((count) => ({ point, count })),
        )
        .find(({ count }) => count.evidence?.group_key === reference.group_key)
    : undefined;
  const point = named ?? counted?.point;
  const outcome = counted?.count;
  // An outcome's count is drawn beside its name, not as the point's value.
  const selectedValue =
    (outcome ? undefined : point?.value) ??
    (!reference.group_key &&
    !reference.through &&
    metric?.evidence.context_id === reference.context_id
      ? metric.value
      : undefined);
  const interval =
    chart?.interval ??
    (reference.context_id === "target"
      ? evaluation.context.target_interval
      : reference.context_id === "state"
        ? undefined
        : evaluation.context.interval);
  return {
    metric,
    point,
    outcome,
    interval,
    selectedValue,
    coverage: selectedCoverage(reference, metric, chart),
  };
}

// A point of a chart answers to its chart's coverage; the figure, to its own.
function selectedCoverage(
  reference: ReportingEvidenceRef,
  metric: ReportingEvaluation["metrics"][number] | undefined,
  chart: ReportingEvaluation["charts"][number] | undefined,
): ReportingCoverage | undefined {
  if (reference.group_key || reference.through) return chart?.coverage;
  if (metric?.evidence.context_id === reference.context_id)
    return metric.coverage;
  return chart?.coverage ?? metric?.coverage;
}

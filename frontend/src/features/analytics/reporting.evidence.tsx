import "./reporting.css";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useId, useRef, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { EntityRef } from "../../components/entityref";
import { Button } from "../../design-system/atoms";
import { DataTable } from "../../design-system/datatable";
import { DrawerBody, DrawerHead } from "../../design-system/drawerbands";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { Modal } from "../../design-system/modal";
import { QueryGate } from "../../design-system/querystates";
import { Stack } from "../../design-system/stack";
import { formatActivityWhen } from "../../format/activitywhen";
import { formatDateTime, formatNumber } from "../../format/format";
import { type Translator, useLocale, usePlural, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { isEntityKind } from "../../lib/entity";
import { isVersionSkewOf, throwProblem } from "../../lib/problem";
import { cohortOutcomeLabel } from "./reporting.cohort";
import { breakdownLabel } from "./reporting.grouplabel";
import {
  type ReportingEvaluation,
  type ReportingEvidenceRef,
  reportingAmount,
  reportingPeriodLabel,
  reportingQuery,
  unitLabel,
} from "./reporting.model";
import { movementLabel } from "./reporting.movement";
import { useReportingPages } from "./reporting.pagination";
import {
  reloadReportingFigures,
  useMetricNames,
  useReportingCatalog,
} from "./reporting.queries";

type ReportingCoverage = components["schemas"]["ReportingCoverage"];

const metricDateHeading: Partial<
  Record<ReportingEvidenceRef["metric"], MessageKey>
> = {
  bookings_won: "reporting.closedOn",
  meetings_held: "reporting.meetingOn",
  accepted_opportunities: "reporting.acceptedOn",
  leads_created: "reporting.createdOn",
  leads_promoted: "reporting.createdOn",
  lead_status_moves: "reporting.movedOn",
  lead_first_response: "reporting.arrivedOn",
  leads_awaiting_response: "reporting.arrivedOn",
  partner_pipeline: "reporting.createdOn",
  partner_won: "reporting.closedOn",
  partner_commission: "reporting.accruedOn",
  stage_conversion: "reporting.enteredOn",
  lead_cohort_outcomes: "reporting.arrivedOn",
};

// A cohort member's row carries its days to an outcome, or open so far, where
// the reading itself is a rate.
const COHORT_METRICS: readonly ReportingEvidenceRef["metric"][] = [
  "stage_conversion",
  "lead_cohort_outcomes",
];

// A record's figure is a deal's value unless the metric counts or times it, or
// the money is a commission rather than the deal.
function valueHeading(
  reference: ReportingEvidenceRef,
  unit: string | undefined,
  t: Translator,
): string {
  if (reference.metric === "partner_commission")
    return t("reporting.commissionAmount");
  return unit === "count" || unit === "days" || unit === "hours"
    ? unitLabel(unit, t)
    : t("reporting.dealValue");
}

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

// The permitted records behind a figure, a page at a time, read under the
// evaluation's own receipt. A measured zero lists its empty population.
function EvidenceRecords({
  evaluation,
  reference,
  editionId,
  zero,
  unit,
  selectedValue,
  onClose,
}: Readonly<{
  evaluation: ReportingEvaluation;
  reference: ReportingEvidenceRef;
  editionId?: string;
  zero: boolean;
  unit: string;
  selectedValue: number | null | undefined;
  onClose: () => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const client = useQueryClient();
  const { cursor, next: setCursor, back, canBack } = useReportingPages();
  const query = useQuery({
    queryKey: ["reporting-evidence", evaluation, reference, editionId, cursor],
    queryFn: async ({ signal }) => {
      const params = { ...reference, cursor, limit: 50 };
      const result = editionId
        ? await api.GET("/analytics/editions/{id}/evidence", {
            params: { path: { id: editionId }, query: params },
            signal,
          })
        : await api.GET("/analytics/evidence", {
            params: {
              query: {
                ...reportingQuery(evaluation.selection),
                ...params,
                evaluation_key: evaluation.evaluation_key,
                evaluated_at: evaluation.context.evaluated_at,
                framework_revision: evaluation.context.framework_revision,
              },
            },
            signal,
          });
      if (result.error) throwProblem(result.error);
      return result.data;
    },
  });
  const rowUnit = COHORT_METRICS.includes(reference.metric) ? "days" : unit;
  const dateHeading = t(
    metricDateHeading[reference.metric] ?? "reporting.period",
  );
  if (isVersionSkewOf(query.error))
    return (
      <ErrorLine
        actions={
          <Button
            onClick={() => {
              reloadReportingFigures(client);
              onClose();
            }}
          >
            {t("common.retry")}
          </Button>
        }
      >
        {t("reporting.evidenceStale")}
      </ErrorLine>
    );
  return (
    <>
      {zero && <p>{t("reporting.evidenceZero")}</p>}
      {selectedValue != null && (
        <p className="t-num">
          {t("reporting.selectedValue")}:{" "}
          {reportingAmount(
            selectedValue,
            unit,
            evaluation.context.currency,
            locale,
          )}
        </p>
      )}
      <QueryGate query={query} pendingLabel={t("reporting.evidence")}>
        {(evidence) => (
          <>
            <p role="status">
              {plural("reporting.pageRecords", evidence.rows.length, {
                count: formatNumber(evidence.rows.length, locale),
              })}
            </p>
            <DataTable
              label={t("reporting.evidence")}
              rows={evidence.rows}
              rowKey={(row) => row.key}
              columns={[
                {
                  key: "source",
                  header: t("reporting.record"),
                  render: (row) => {
                    if (row.restricted) return t("reporting.restricted");
                    // Every record type with a page links, in a new tab so the evidence
                    // stays open. Anything else (an activity, a governance row) stays its label.
                    return row.source_id &&
                      row.source_type &&
                      isEntityKind(row.source_type) ? (
                      <EntityRef
                        kind={row.source_type}
                        id={row.source_id}
                        name={row.label}
                        newTab
                      />
                    ) : (
                      row.label
                    );
                  },
                },
                {
                  key: "date",
                  header: dateHeading,
                  render: (row) =>
                    row.occurred_at
                      ? formatActivityWhen(
                          row.occurred_at,
                          row.occurred_precision ?? "instant",
                          locale,
                          evidence.context.timezone,
                        )
                      : "—",
                },
                {
                  key: "amount",
                  header: valueHeading(reference, rowUnit, t),
                  render: (row) =>
                    reportingAmount(
                      row.value,
                      rowUnit,
                      evidence.context.currency,
                      locale,
                    ),
                },
              ]}
            />
            {canBack && (
              <Button variant="ghost" onClick={back}>
                {t("reporting.back")}
              </Button>
            )}
            {evidence.next_cursor && (
              <Button
                variant="ghost"
                onClick={() => setCursor(evidence.next_cursor)}
              >
                {t("reporting.next")}
              </Button>
            )}
          </>
        )}
      </QueryGate>
    </>
  );
}

/**
 * The frame every evidence drawer on Analytics shares: the figure's name, the
 * lines that say what it was read for, then its body. Focus starts on the
 * name and returns to the control that opened it.
 */
export function EvidenceFrame({
  title,
  lines,
  onClose,
  children,
}: Readonly<{
  title: string;
  lines: readonly string[];
  onClose: () => void;
  children: ReactNode;
}>) {
  const titleId = useId();
  const heading = useRef<HTMLHeadingElement>(null);
  return (
    <Modal
      open
      onClose={onClose}
      labelledBy={titleId}
      intent="drawer"
      initialFocusTo={() => heading.current}
    >
      <DrawerHead>
        <Heading
          ref={heading}
          tabIndex={-1}
          id={titleId}
          as="h2"
          size="medium"
          className="modal-title"
        >
          {title}
        </Heading>
        {lines.map((line) => (
          <p key={line} className="t-caption">
            {line}
          </p>
        ))}
      </DrawerHead>
      <DrawerBody>
        <Stack gap="4">{children}</Stack>
      </DrawerBody>
    </Modal>
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

/**
 * What a reader opened from a view, for as long as the question it was read
 * from stands. A changed selection closes it, so a drawer never lists one
 * question's records under another's figures.
 */
export function useSelectionHeld<Held>(question: string) {
  const [held, setHeld] = useState<Readonly<{
    question: string;
    item: Held;
  }> | null>(null);
  const item = held?.question === question ? held.item : null;
  const open = (next: Held) => setHeld({ question, item: next });
  const close = () => setHeld(null);
  return { held: item, open, close };
}

/**
 * The evidence a reader opened. `from` is the evaluation the reference came
 * from, when a view reads its charts apart from its headline figures.
 */
export function useSelectionEvidence(question: string) {
  const { held, open, close } =
    useSelectionHeld<
      Readonly<{ reference: ReportingEvidenceRef; from?: ReportingEvaluation }>
    >(question);
  return {
    evidence: held?.reference ?? null,
    evidenceFrom: held?.from,
    openEvidence: (
      reference: ReportingEvidenceRef,
      from?: ReportingEvaluation,
    ) => open({ reference, from }),
    closeEvidence: close,
  };
}

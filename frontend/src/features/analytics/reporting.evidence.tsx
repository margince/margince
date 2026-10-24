import "./reporting.css";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useRef } from "react";
import { api } from "../../api/client";
import { EntityRef } from "../../components/entityref";
import { Button } from "../../design-system/atoms";
import { DataTable } from "../../design-system/datatable";
import { DrawerBody, DrawerHead } from "../../design-system/drawerbands";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { Modal } from "../../design-system/modal";
import { Popover } from "../../design-system/popover";
import { QueryGate } from "../../design-system/querystates";
import { Stack } from "../../design-system/stack";
import { formatDateTime, formatNumber } from "../../format/format";
import { type Translator, useLocale, usePlural, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { isEntityKind } from "../../lib/entity";
import { isVersionSkewOf, throwProblem } from "../../lib/problem";
import {
  metricLabel,
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
  useReportingCatalog,
} from "./reporting.queries";

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
  partner_pipeline: "reporting.createdOn",
  partner_won: "reporting.closedOn",
  partner_commission: "reporting.accruedOn",
};

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
  const plural = usePlural();
  const { locale } = useLocale();
  const client = useQueryClient();
  const title = useId();
  const heading = useRef<HTMLHeadingElement>(null);
  const { cursor, next: setCursor, back, canBack } = useReportingPages();
  const query = useQuery({
    queryKey: ["reporting-evidence", evaluation, reference, editionId, cursor],
    queryFn: async () => {
      const params = { ...reference, cursor, limit: 50 };
      const result = editionId
        ? await api.GET("/analytics/editions/{id}/evidence", {
            params: { path: { id: editionId }, query: params },
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
          });
      if (result.error) throwProblem(result.error);
      return result.data;
    },
  });
  const catalog = useReportingCatalog();
  const definition = catalog.data?.metrics.find(
    (metric) => metric.id === reference.metric,
  );
  const { metric, point, interval, selectedValue } = evidenceSelection(
    evaluation,
    reference,
  );
  const stateLabel = editionId
    ? formatDateTime(
        evaluation.context.state_at,
        locale,
        evaluation.context.timezone,
      )
    : t("reporting.currentPipeline");
  const dateHeading = t(
    metricDateHeading[reference.metric] ?? "reporting.period",
  );
  return (
    <Modal
      open
      onClose={onClose}
      labelledBy={title}
      intent="drawer"
      initialFocusTo={() => heading.current}
    >
      <DrawerHead>
        <Heading
          ref={heading}
          tabIndex={-1}
          id={title}
          as="h2"
          size="medium"
          className="modal-title"
        >
          {metricLabel(reference.metric, t)}
        </Heading>
        <p className="t-caption">
          {evaluation.context.scope.label}
          {point?.label
            ? ` · ${reference.context_id === "movement_delta" ? movementLabel(point.key, point.label, t) : point.label}`
            : ""}
        </p>
        <p className="t-caption">
          {interval
            ? reportingPeriodLabel(
                {
                  ...interval,
                  end_at: reference.through ?? interval.end_at,
                },
                evaluation.context.timezone,
                locale,
              )
            : stateLabel}
        </p>
      </DrawerHead>
      <DrawerBody>
        <Stack gap="4">
          <MetricDefinition
            definition={definition}
            version={metric?.version}
            frozen={Boolean(editionId)}
          />
          {metric?.coverage.status !== undefined &&
            metric.coverage.status !== "ok" && (
              <Popover
                onHover
                label={t(`reporting.status.${metric.coverage.status}`)}
              >
                <p>{metric.coverage.reason}</p>
              </Popover>
            )}
          <Popover onHover label={t("reporting.contextDetails")}>
            <p>
              {evaluation.context.currency} · {evaluation.context.timezone}
            </p>
            <p>
              {editionId ? t("reporting.frozen") : t("reporting.live")} ·{" "}
              {formatDateTime(
                evaluation.context.evaluated_at,
                locale,
                evaluation.context.timezone,
              )}
            </p>
          </Popover>
          {selectedValue != null && (
            <p className="t-num">
              {t("reporting.selectedValue")}:{" "}
              {reportingAmount(
                selectedValue,
                metric?.unit ?? definition?.unit ?? "count",
                evaluation.context.currency,
                locale,
              )}
            </p>
          )}
          {isVersionSkewOf(query.error) ? (
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
          ) : (
            <QueryGate query={query} pendingLabel={t("reporting.evidence")}>
              {(evidence) => (
                <>
                  <p>
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
                            ? formatDateTime(
                                row.occurred_at,
                                locale,
                                evidence.context.timezone,
                              )
                            : "—",
                      },
                      {
                        key: "amount",
                        header: valueHeading(reference, metric?.unit, t),
                        render: (row) =>
                          reportingAmount(
                            row.value,
                            metric?.unit ?? definition?.unit ?? "count",
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
          )}
        </Stack>
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
  const point =
    reference.group_key || reference.through
      ? chart?.points.find(
          (point) =>
            point.evidence?.group_key === reference.group_key &&
            point.evidence?.through === reference.through,
        )
      : undefined;
  const selectedValue =
    point?.value ??
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
  return { metric, point, interval, selectedValue };
}

function MetricDefinition({
  definition,
  version,
  frozen,
}: Readonly<{
  definition?: { version: string; definition: string };
  version?: string;
  frozen: boolean;
}>) {
  const t = useT();
  const changed = frozen && definition && version !== definition.version;
  return (
    <Popover onHover label={t("reporting.definition")}>
      <p>
        {changed ? t("reporting.earlierDefinition") : definition?.definition}
      </p>
    </Popover>
  );
}

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useRef } from "react";
import { api } from "../api/client";
import { navigate } from "../app/router";
import { Button } from "../design-system/atoms";
import { DataTable } from "../design-system/datatable";
import { Heading } from "../design-system/heading";
import { Modal } from "../design-system/modal";
import { Popover } from "../design-system/popover";
import { formatDateTime, formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import { isVersionSkewOf, QueryGate, throwProblem } from "./common";
import {
  metricLabel,
  type ReportingEvaluation,
  type ReportingEvidenceRef,
  reportingAmount,
  reportingQuery,
} from "./reporting.model";
import { useReportingPages } from "./reporting.pagination";

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
  const catalog = useQuery({
    queryKey: ["reporting-catalog"],
    queryFn: async () => {
      const { data, error } = await api.GET("/analytics/metrics");
      if (error) throwProblem(error);
      return data;
    },
  });
  const definition = catalog.data?.metrics.find(
    (metric) => metric.id === reference.metric,
  );
  const { metric, point, interval, selectedValue } = evidenceSelection(
    evaluation,
    reference,
  );
  return (
    <Modal
      open
      onClose={onClose}
      labelledBy={title}
      placement="right"
      initialFocusTo={() => heading.current}
    >
      <div className="reporting-dialog">
        <Heading ref={heading} tabIndex={-1} id={title} as="h2" size="medium">
          {metricLabel(reference.metric, t)}
        </Heading>
        {!point?.at && point?.label && <p>{point.label}</p>}
        <Popover onHover label={t("reporting.definition")}>
          <p>{definition?.definition}</p>
        </Popover>
        <p className="t-caption">
          {evaluation.context.scope.label} · {evaluation.context.currency} ·{" "}
          {evaluation.context.timezone}
        </p>
        <p className="t-caption">
          {point?.at && !reference.through
            ? point.label
            : formatDateTime(
                interval?.start_at ?? evaluation.context.state_at,
                locale,
                evaluation.context.timezone,
              )}{" "}
          {(!point?.at || reference.through) && interval && (
            <>
              {" "}
              –{" "}
              {formatDateTime(
                new Date(
                  Date.parse(reference.through ?? interval.end_at) - 1,
                ).toISOString(),
                locale,
                evaluation.context.timezone,
              )}
            </>
          )}
        </p>
        {metric?.coverage.reason && (
          <Popover
            onHover
            label={t(`reporting.status.${metric.coverage.status}`)}
          >
            <p>{metric.coverage.reason}</p>
          </Popover>
        )}
        <p className="t-caption">
          {editionId ? t("reporting.frozen") : t("reporting.live")} ·{" "}
          {formatDateTime(
            evaluation.context.evaluated_at,
            locale,
            evaluation.context.timezone,
          )}
        </p>
        {isVersionSkewOf(query.error) && (
          <Button
            onClick={() => {
              client.invalidateQueries({
                predicate: (query) =>
                  [
                    "reporting-evaluation",
                    "reporting-live",
                    "reporting-forecast",
                  ].includes(String(query.queryKey[0])),
              });
              onClose();
            }}
          >
            {t("common.retry")}
          </Button>
        )}
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
                    header: t("reporting.evidence"),
                    render: (row) =>
                      row.restricted ? (
                        t("reporting.restricted")
                      ) : row.source_id && row.source_type === "deal" ? (
                        <Button
                          variant="link"
                          onClick={() =>
                            navigate({ screen: "deals", id: row.source_id })
                          }
                        >
                          {row.label}
                        </Button>
                      ) : (
                        row.label
                      ),
                  },
                  {
                    key: "date",
                    header: t("reporting.period"),
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
                    key: "unit",
                    header: t("reporting.unit"),
                    render: () =>
                      metric?.unit === "days"
                        ? t("reporting.daysUnit")
                        : metric?.unit === "count"
                          ? t("reporting.countUnit")
                          : (metric?.unit ?? definition?.unit),
                  },
                  {
                    key: "amount",
                    header: t("reporting.actual"),
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
      </div>
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
      candidate.context_id === reference.context_id,
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

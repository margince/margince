import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useRef, useState } from "react";
import { api } from "../api/client";
import { navigate } from "../app/router";
import { Button } from "../design-system/atoms";
import { DataTable } from "../design-system/datatable";
import { Heading } from "../design-system/heading";
import { Modal } from "../design-system/modal";
import { formatDateTime } from "../format/format";
import { useLocale, useT } from "../i18n";
import { isVersionSkewOf, QueryGate, throwProblem } from "./common";
import {
  metricLabel,
  type ReportingEvaluation,
  type ReportingEvidenceRef,
  reportingAmount,
  reportingQuery,
} from "./reporting.model";

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
  const { locale } = useLocale();
  const client = useQueryClient();
  const title = useId();
  const heading = useRef<HTMLHeadingElement>(null);
  const [cursor, setCursor] = useState<string>();
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
  const metric = evaluation.metrics.find(
    (metric) => metric.id === reference.metric,
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
        <p>{definition?.definition}</p>
        <p className="t-caption">
          {evaluation.context.scope.label} · {evaluation.context.currency} ·{" "}
          {evaluation.context.timezone}
        </p>
        <p className="t-caption">
          {formatDateTime(
            evaluation.context.interval.start_at,
            locale,
            evaluation.context.timezone,
          )}{" "}
          –{" "}
          {formatDateTime(
            evaluation.context.interval.end_at,
            locale,
            evaluation.context.timezone,
          )}{" "}
          · {metric?.version}
        </p>
        {metric?.coverage.reason && (
          <p role="status">{metric.coverage.reason}</p>
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
        <QueryGate query={query} pendingLabel={t("reporting.evidence")}>
          {(evidence) => (
            <>
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

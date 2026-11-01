// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import { EntityRef } from "../../components/entityref";
import { Button } from "../../design-system/atoms";
import { DataTable } from "../../design-system/datatable";
import { ErrorLine } from "../../design-system/errorline";
import { QueryGate } from "../../design-system/querystates";
import { formatActivityWhen } from "../../format/activitywhen";
import { formatNumber } from "../../format/format";
import { type Translator, useLocale, usePlural, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { isEntityKind } from "../../lib/entity";
import { isVersionSkewOf, throwProblem } from "../../lib/problem";
import {
  type ReportingEvaluation,
  type ReportingEvidenceRef,
  reportingAmount,
  reportingQuery,
  unitLabel,
} from "./reporting.model";
import { useReportingPages } from "./reporting.pagination";
import { reloadReportingFigures } from "./reporting.queries";

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

// The permitted records behind a figure, a page at a time, read under the
// evaluation's own receipt. A measured zero lists its empty population.
export function EvidenceRecords({
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

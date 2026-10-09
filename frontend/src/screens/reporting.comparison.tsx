import "./reporting.css";
import { useQuery } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button, Field } from "../design-system/atoms";
import { DataTable } from "../design-system/datatable";
import { DrawerBody, DrawerHead } from "../design-system/drawerbands";
import { Heading } from "../design-system/heading";
import { Modal } from "../design-system/modal";
import { Select } from "../design-system/select";
import { useLocale, useT } from "../i18n";
import { QueryGate, throwProblem } from "./common";
import { ReportingEvidenceDrawer } from "./reporting.evidence";
import {
  editionLabel,
  metricLabel,
  type ReportingEdition,
  type ReportingEvidenceRef,
  reportingAmount,
} from "./reporting.model";

export function ReportingComparison({
  editions,
  onClose,
  hasMore,
  loadingMore,
  onLoadMore,
}: Readonly<{
  editions: readonly ReportingEdition[];
  onClose: () => void;
  hasMore?: boolean;
  loadingMore?: boolean;
  onLoadMore?: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const title = useId();
  const firstPair = editions.flatMap((before) =>
    editions
      .filter((after) => adjacentEditions(before, after))
      .map((after) => ({ before, after })),
  )[0];
  const [selection, setSelection] = useState<{
    left: string;
    right: string;
  } | null>(null);
  const left = selection?.left ?? firstPair?.before.id ?? editions[1]?.id ?? "";
  const right = selection?.right ?? firstPair?.after.id ?? "";
  const [evidence, setEvidence] = useState<{
    edition: ReportingEdition;
    reference: ReportingEvidenceRef;
  } | null>(null);
  const query = useQuery({
    enabled: !!left && !!right && left !== right,
    queryKey: ["reporting-comparison", left, right],
    queryFn: async () => {
      const { data, error } = await api.GET("/analytics/editions/compare", {
        params: { query: { left_id: left, right_id: right } },
      });
      if (error) throwProblem(error);
      return data;
    },
  });
  const options = editions.map((edition) => ({
    value: edition.id,
    label: editionLabel(edition, locale),
  }));
  return (
    <Modal open onClose={onClose} labelledBy={title} intent="drawer-reading">
      <DrawerHead>
        <Heading size="large" id={title} className="t-h2 modal-title">
          {t("reporting.compare")}
        </Heading>
        <p>{t("reporting.comparisonHelp")}</p>
      </DrawerHead>
      <DrawerBody className="form-stack">
        {hasMore && (
          <Button variant="ghost" pending={loadingMore} onClick={onLoadMore}>
            {t("reporting.loadOlder")}
          </Button>
        )}
        <div className="form-row">
          <Field label={t("reporting.left")}>
            {(field) => (
              <Select
                {...field}
                value={left}
                options={options}
                onChange={(value) => {
                  setSelection({ left: value, right: "" });
                }}
              />
            )}
          </Field>
          <Field label={t("reporting.right")}>
            {(field) => (
              <Select
                {...field}
                value={right}
                options={options.filter((option) => {
                  const before = editions.find(
                    (edition) => edition.id === left,
                  );
                  const after = editions.find(
                    (edition) => edition.id === option.value,
                  );
                  return before && after && adjacentEditions(before, after);
                })}
                onChange={(value) => setSelection({ left, right: value })}
              />
            )}
          </Field>
        </div>
        {!left || !right || left === right ? (
          <p role="status">{t("reporting.noComparison")}</p>
        ) : (
          <QueryGate query={query} pendingLabel={t("reporting.compare")}>
            {(comparison) => (
              <>
                {!comparison.compatible && (
                  <p role="status">
                    {comparison.reason ?? t("reporting.noComparison")}
                  </p>
                )}
                {comparison.compatible && (
                  <ComparisonMetrics
                    comparison={comparison}
                    onEvidence={setEvidence}
                  />
                )}
              </>
            )}
          </QueryGate>
        )}
        {evidence && (
          <ReportingEvidenceDrawer
            key={`${evidence.edition.id}:${evidence.reference.metric}`}
            evaluation={evidence.edition.evaluation}
            reference={evidence.reference}
            editionId={evidence.edition.id}
            onClose={() => setEvidence(null)}
          />
        )}
      </DrawerBody>
    </Modal>
  );
}

type EvidenceSelection = {
  edition: ReportingEdition;
  reference: ReportingEvidenceRef;
};
function ComparisonMetrics({
  comparison,
  onEvidence,
}: Readonly<{
  comparison: components["schemas"]["ReportingComparison"];
  onEvidence: (selection: EvidenceSelection) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const rows = comparison.left.evaluation.metrics.flatMap((before) => {
    const after = comparison.right.evaluation.metrics.find(
      (metric) => metric.id === before.id,
    );
    return after
      ? [
          {
            before,
            after,
            delta: comparison.deltas.find(
              (delta) => delta.metric === before.id,
            ),
          },
        ]
      : [];
  });
  const amount = (
    value: number | null | undefined,
    unit: string,
    edition: ReportingEdition,
  ) =>
    reportingAmount(value, unit, edition.evaluation.context.currency, locale);
  return (
    <DataTable
      label={t("reporting.compare")}
      rows={rows}
      rowKey={(row) => row.before.id}
      columns={[
        {
          key: "metric",
          header: t("reporting.metrics"),
          render: (row) => metricLabel(row.before.id, t),
        },
        {
          key: "before",
          header: t("reporting.left"),
          render: (row) => (
            <Button
              variant="link"
              onClick={() =>
                onEvidence({
                  edition: comparison.left,
                  reference: row.before.evidence,
                })
              }
            >
              {amount(row.before.value, row.before.unit, comparison.left)}
            </Button>
          ),
        },
        {
          key: "after",
          header: t("reporting.right"),
          render: (row) => (
            <Button
              variant="link"
              onClick={() =>
                onEvidence({
                  edition: comparison.right,
                  reference: row.after.evidence,
                })
              }
            >
              {amount(row.after.value, row.after.unit, comparison.right)}
            </Button>
          ),
        },
        {
          key: "change",
          header: t("reporting.change"),
          render: (row) =>
            row.delta ? (
              <span className="t-num">
                {row.delta.absolute != null && row.delta.absolute > 0
                  ? "+"
                  : ""}
                {amount(row.delta.absolute, row.before.unit, comparison.left)}
                {row.delta.percentage == null
                  ? ""
                  : ` · ${row.delta.percentage > 0 ? "+" : ""}${reportingAmount(row.delta.percentage, "percent", "", locale)}`}
              </span>
            ) : (
              "—"
            ),
        },
      ]}
    />
  );
}

function adjacentEditions(
  before: ReportingEdition,
  after: ReportingEdition,
): boolean {
  return (
    before.id !== after.id &&
    Date.parse(before.evaluation.context.interval.end_at) ===
      Date.parse(after.evaluation.context.interval.start_at)
  );
}

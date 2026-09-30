import { useQuery } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button, Field } from "../design-system/atoms";
import { Heading } from "../design-system/heading";
import { Modal } from "../design-system/modal";
import { Panel, PanelBody } from "../design-system/panel";
import { GroupedBars } from "../design-system/report-charts";
import { Select } from "../design-system/select";
import { formatDateTime } from "../format/format";
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
    <Modal open onClose={onClose} labelledBy={title} size="wide">
      <div className="reporting-dialog">
        <Heading as="h2" size="medium" id={title}>
          {t("reporting.compare")}
        </Heading>
        <p>{t("reporting.comparisonHelp")}</p>
        {hasMore && (
          <Button variant="ghost" pending={loadingMore} onClick={onLoadMore}>
            {t("reporting.loadOlder")}
          </Button>
        )}
        <div className="reporting-toolbar">
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
                {comparison.compatible &&
                  comparison.left.evaluation.metrics.map((before) => (
                    <ComparisonMetric
                      key={before.id}
                      comparison={comparison}
                      before={before}
                      onEvidence={setEvidence}
                    />
                  ))}
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
      </div>
    </Modal>
  );
}

type EvidenceSelection = {
  edition: ReportingEdition;
  reference: ReportingEvidenceRef;
};
function ComparisonMetric({
  comparison,
  before,
  onEvidence: setEvidence,
}: Readonly<{
  comparison: components["schemas"]["ReportingComparison"];
  before: components["schemas"]["ReportingMetric"];
  onEvidence: (selection: EvidenceSelection) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const after = comparison.right.evaluation.metrics.find(
    (metric) => metric.id === before.id,
  );
  if (!after) return null;
  const delta = comparison.compatible
    ? comparison.deltas.find((delta) => delta.metric === before.id)
    : undefined;
  const format = (value: number | null | undefined) =>
    reportingAmount(
      value,
      before.unit,
      comparison.left.evaluation.context.currency,
      locale,
    );
  return (
    <Panel title={metricLabel(before.id, t)}>
      <PanelBody>
        {before.unit === after.unit &&
        (before.unit !== "money" ||
          comparison.left.evaluation.context.currency ===
            comparison.right.evaluation.context.currency) ? (
          <GroupedBars
            label={metricLabel(before.id, t)}
            dataLabel={t("reporting.data")}
            valueLabel={editionLabel(comparison.left, locale)}
            comparisonLabel={editionLabel(comparison.right, locale)}
            readings={[
              {
                key: before.id,
                label: metricLabel(before.id, t),
                value: before.value,
                amount: format(before.value),
                comparison: after.value,
                comparisonAmount: format(after.value),
              },
            ]}
            onSelect={(key) =>
              setEvidence(
                key.endsWith(":comparison")
                  ? {
                      edition: comparison.right,
                      reference: after.evidence,
                    }
                  : {
                      edition: comparison.left,
                      reference: before.evidence,
                    },
              )
            }
          />
        ) : (
          <div className="reporting-comparison-values">
            <Button
              variant="link"
              onClick={() =>
                setEvidence({
                  edition: comparison.left,
                  reference: before.evidence,
                })
              }
            >
              {formatDateTime(
                comparison.left.captured_at,
                locale,
                comparison.left.evaluation.context.timezone,
              )}{" "}
              · {format(before.value)}
            </Button>
            <Button
              variant="link"
              onClick={() =>
                setEvidence({
                  edition: comparison.right,
                  reference: after.evidence,
                })
              }
            >
              {formatDateTime(
                comparison.right.captured_at,
                locale,
                comparison.right.evaluation.context.timezone,
              )}{" "}
              ·{" "}
              {reportingAmount(
                after.value,
                after.unit,
                comparison.right.evaluation.context.currency,
                locale,
              )}
            </Button>
          </div>
        )}
        {delta && (
          <p className="t-num">
            {format(delta.absolute)}
            {delta.percentage == null
              ? ""
              : ` · ${reportingAmount(delta.percentage, "percent", comparison.left.evaluation.context.currency, locale)}`}
          </p>
        )}
      </PanelBody>
    </Panel>
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

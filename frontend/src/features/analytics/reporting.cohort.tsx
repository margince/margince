import { Button, EmptyState } from "../../design-system/atoms";
import { DataTable } from "../../design-system/datatable";
import { formatNumber, formatPercent, MONEY_ABSENT } from "../../format/format";
import { type Translator, useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import type { ReportingChart, ReportingEvidenceRef } from "./reporting.model";

type CohortPoint = ReportingChart["points"][number];
type CohortCount = NonNullable<CohortPoint["outcomes"]>[number];

// The reader's own words for each outcome the server names. An outcome a newer
// server adds keeps the server's label.
const OUTCOME_LABEL: Readonly<Record<string, MessageKey>> = {
  advanced: "reporting.cohortOutcome.advanced",
  won: "reporting.cohortOutcome.won",
  lost: "reporting.cohortOutcome.lost",
  regressed: "reporting.cohortOutcome.regressed",
  left_pipeline: "reporting.cohortOutcome.left_pipeline",
  unresolved: "reporting.cohortOutcome.unresolved",
  history_partial: "reporting.cohortOutcome.history_partial",
  accepted: "reporting.cohortOutcome.accepted",
  qualified: "reporting.cohortOutcome.qualified",
  disqualified: "reporting.cohortOutcome.disqualified",
};

/** An outcome's name in the reader's words, else the server's label. */
export function cohortOutcomeLabel(
  count: Readonly<{ key: string; label: string }>,
  t: Translator,
): string {
  const key = OUTCOME_LABEL[count.key];
  return key ? t(key) : count.label;
}

// The outcomes that stay out of the rate, each with the note that says why.
const NOTE_OF_OUTCOME: Readonly<Record<string, MessageKey>> = {
  unresolved: "reporting.cohortStillOpenNote",
  history_partial: "reporting.cohortPartialNote",
};

// How a member entered a stage, beside the outcomes. A first entry is the rest.
const ENTRY_COLUMNS = ["repeat", "reopened"] as const;

/**
 * A cohort drawn as a table: one row per stage, or one row of arrivals, with
 * a column per outcome. Every count opens the members it counts, and the
 * notes below say which counts stay out of the rate and what the days measure.
 */
export function CohortTable({
  chart,
  label,
  onEvidence,
}: Readonly<{
  chart: ReportingChart;
  label: string;
  onEvidence: (reference: ReportingEvidenceRef) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const stages = chart.kind === "stage_cohort";
  const outcomes = chart.points[0]?.outcomes ?? [];
  const members = chart.points.reduce(
    (total, point) => total + (point.observations ?? 0),
    0,
  );
  if (members === 0)
    return (
      <EmptyState>
        {t(stages ? "reporting.cohortEmpty" : "reporting.cohortNoArrivals")}
      </EmptyState>
    );
  const outcomeLabel = (count: CohortCount) => cohortOutcomeLabel(count, t);
  const shown = (key: string) =>
    chart.points.some((point) =>
      point.outcomes?.some((count) => count.key === key && count.count > 0),
    );
  return (
    <>
      <DataTable
        label={label}
        rows={chart.points}
        rowKey={(point) => point.key}
        stickyFirst
        columns={[
          {
            key: "label",
            header: stages ? t("reporting.cohortStage") : label,
            render: (point) => (
              <Button
                variant="link"
                onClick={() => point.evidence && onEvidence(point.evidence)}
              >
                {point.label}
              </Button>
            ),
          },
          {
            key: "members",
            header: stages
              ? t("reporting.cohortEntered")
              : t("reporting.cohortArrived"),
            render: (point) => formatNumber(point.observations ?? 0, locale),
          },
          ...outcomes.map((outcome) => ({
            key: outcome.key,
            header: outcomeLabel(outcome),
            render: (point: CohortPoint) => (
              <CohortCell
                count={point.outcomes?.find(
                  (count) => count.key === outcome.key,
                )}
                onEvidence={onEvidence}
              />
            ),
          })),
          ...(stages
            ? ENTRY_COLUMNS.map((entry) => ({
                key: `entry-${entry}`,
                header: t(`reporting.cohortEntry.${entry}`),
                render: (point: CohortPoint) =>
                  formatNumber(
                    point.entries?.find((count) => count.key === entry)
                      ?.count ?? 0,
                    locale,
                  ),
              }))
            : []),
          {
            key: "rate",
            header: t("reporting.cohortRate"),
            render: (point) =>
              point.value == null
                ? t("reporting.cohortNoRate")
                : formatPercent(point.value / 100, locale),
          },
        ]}
      />
      {Object.entries(NOTE_OF_OUTCOME).map(
        ([key, note]) =>
          (key === "unresolved" || shown(key)) && (
            <p key={key} className="sub">
              {t(note)}
            </p>
          ),
      )}
      <p className="sub">
        {t(
          stages
            ? "reporting.cohortStageDaysNote"
            : "reporting.cohortLeadDaysNote",
        )}
      </p>
    </>
  );
}

// One outcome's count, which opens its members, and the median days beside it.
function CohortCell({
  count,
  onEvidence,
}: Readonly<{
  count: CohortCount | undefined;
  onEvidence: (reference: ReportingEvidenceRef) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  if (!count) return MONEY_ABSENT;
  if (count.count === 0) return formatNumber(0, locale);
  return (
    <>
      <Button
        variant="link"
        onClick={() => count.evidence && onEvidence(count.evidence)}
      >
        {formatNumber(count.count, locale)}
      </Button>
      {count.median_days != null && (
        <span className="sub">
          {" "}
          {t("reporting.cohortMedianDays", {
            days: formatNumber(Math.round(count.median_days), locale),
          })}
        </span>
      )}
    </>
  );
}

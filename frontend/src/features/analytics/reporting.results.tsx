import { QueryGate, type QueryLike } from "../../design-system/querystates";
import { formatDateTime } from "../../format/format";
import { monthName } from "../../format/format.dates";
import { useLocale, useT } from "../../i18n";
import { ReportingCharts } from "./reporting.charts";
import {
  editionStatus,
  type ReportingEdition,
  type ReportingEvaluation,
  type ReportingEvidenceRef,
} from "./reporting.model";
import { ReaderPreview } from "./reporting.readerpreview";

/**
 * What the report page draws under its header: one frozen edition, the report
 * as a role reads it while previewed, or the live evaluation.
 */
export function ReportResults({
  reportId,
  editionId,
  edition,
  live,
  previewRole,
  previewRevision,
  onEndPreview,
  onEvidence,
}: Readonly<{
  reportId: string;
  editionId?: string;
  edition: QueryLike<ReportingEdition>;
  live: QueryLike<ReportingEvaluation>;
  previewRole?: string;
  previewRevision?: number;
  onEndPreview: () => void;
  onEvidence: (reference: ReportingEvidenceRef) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  if (editionId)
    return (
      <QueryGate query={edition} pendingLabel={t("reporting.frozen")}>
        {(edition) => (
          <>
            <p className="reporting-archive-banner" role="status">
              {t("reporting.frozen")} ·{" "}
              {formatDateTime(
                edition.captured_at,
                locale,
                edition.evaluation.context.timezone,
              )}{" "}
              · {editionStatus(edition, t)}
            </p>
            <ReportingCharts
              editionId={editionId}
              kept={edition}
              evaluation={edition.evaluation}
              onEvidence={onEvidence}
            />
          </>
        )}
      </QueryGate>
    );
  if (previewRole)
    return (
      <ReaderPreview
        reportId={reportId}
        revision={previewRevision}
        role={previewRole}
        onEnd={onEndPreview}
      />
    );
  return (
    <QueryGate query={live} pendingLabel={t("reporting.live")}>
      {(evaluation) => (
        <>
          <p className="t-caption">{t("reporting.live")}</p>
          <FiscalQuarterNote evaluation={evaluation} />
          <ReportingCharts
            evaluation={evaluation}
            report={{ id: reportId, revision: previewRevision }}
            onEvidence={onEvidence}
          />
        </>
      )}
    </QueryGate>
  );
}

/**
 * A live report that reads this quarter follows the current fiscal start, so
 * it says which one. A sent edition keeps the start it was captured under.
 */
function FiscalQuarterNote({
  evaluation,
}: Readonly<{ evaluation: ReportingEvaluation }>) {
  const t = useT();
  const { locale } = useLocale();
  const { selection, context } = evaluation;
  const fiscal =
    selection.period === "this_quarter" ||
    selection.target_basis === "fiscal_quarter" ||
    selection.close_window === "fiscal_quarter";
  if (!fiscal || context.fiscal_year_start_month == null) return null;
  return (
    <p className="t-caption">
      {t("reporting.fiscalQuarterNote", {
        month: monthName(context.fiscal_year_start_month, locale),
      })}
    </p>
  );
}

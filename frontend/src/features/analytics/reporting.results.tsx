import { QueryGate, type QueryLike } from "../../design-system/querystates";
import { formatDateTime } from "../../format/format";
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
          <ReportingCharts evaluation={evaluation} onEvidence={onEvidence} />
        </>
      )}
    </QueryGate>
  );
}

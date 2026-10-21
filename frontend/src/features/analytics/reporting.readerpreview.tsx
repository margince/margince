import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import { Button } from "../../design-system/atoms";
import { QueryGate } from "../../design-system/querystates";
import { Select } from "../../design-system/select";
import { useT } from "../../i18n";
import { useCan } from "../../lib/capability";
import { unwrap } from "../../lib/problem";
import { useRoles } from "../../lib/roles.queries";
import { ReportingCharts } from "./reporting.charts";

/**
 * Which role the report is previewed as, offered to a seat that may read the
 * role matrix, which a preview under a role discloses. Empty is the caller's
 * own view.
 */
export function PreviewAsSelect({
  role,
  onChange,
}: Readonly<{ role?: string; onChange: (role?: string) => void }>) {
  const t = useT();
  const canReadRoles = useCan("role_admin", "read");
  const roles = useRoles(canReadRoles);
  if (!canReadRoles || !roles.data) return null;
  return (
    <Select
      aria-label={t("reporting.previewAs")}
      value={role ?? ""}
      placeholder={t("reporting.previewAs")}
      options={[
        { value: "", label: t("reporting.previewAsSelf") },
        ...roles.data.map((candidate) => ({
          value: candidate.key,
          label: candidate.name,
        })),
      ]}
      onChange={(value) => onChange(value || undefined)}
    />
  );
}

/** The report as a seat holding the role reads it, in place of the live view. */
export function ReaderPreview({
  reportId,
  revision,
  role,
  onEnd,
}: Readonly<{
  reportId: string;
  revision?: number;
  role: string;
  onEnd: () => void;
}>) {
  const t = useT();
  const roles = useRoles(true);
  const name = roles.data?.find((candidate) => candidate.key === role)?.name;
  // Records open under the caller's own access, which is not the preview's,
  // so a chart's records are not offered here.
  const [askedForRecords, setAskedForRecords] = useState(false);
  const preview = useQuery({
    queryKey: [
      "reporting-reader-preview",
      reportId,
      revision ?? "published",
      role,
    ],
    queryFn: async () =>
      unwrap(
        await api.GET("/analytics/reports/{id}/reader-preview", {
          params: { path: { id: reportId }, query: { role, revision } },
        }),
      ),
  });
  return (
    <>
      <div className="reporting-archive-banner" role="status">
        <p>{t("reporting.previewAsBanner", { role: name ?? role })}</p>
        <Button variant="ghost" onClick={onEnd}>
          {t("reporting.endPreview")}
        </Button>
      </div>
      <QueryGate query={preview} pendingLabel={t("reporting.previewLoading")}>
        {(evaluation) => (
          <>
            {askedForRecords && (
              <p role="status">{t("reporting.previewNoRecords")}</p>
            )}
            <ReportingCharts
              evaluation={evaluation}
              onEvidence={() => setAskedForRecords(true)}
            />
          </>
        )}
      </QueryGate>
    </>
  );
}

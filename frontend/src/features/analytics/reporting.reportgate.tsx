import type { ComponentProps, ReactNode } from "react";
import { Button } from "../../design-system/atoms";
import { QueryGate } from "../../design-system/querystates";
import { useT } from "../../i18n";
import { openAnalyticsSection } from "../../lib/analytics.address";
import type { ReportingReport } from "./reporting.model";

// A saved report that is gone says so and offers the way back to the list.
export function ReportGate({
  query,
  children,
}: Readonly<{
  query: ComponentProps<typeof QueryGate<ReportingReport>>["query"];
  children: (report: ReportingReport) => ReactNode;
}>) {
  const t = useT();
  return (
    <QueryGate
      query={query}
      pendingLabel={t("reporting.reports")}
      goneKey="reporting.reportGone"
      goneAction={
        <Button onClick={() => openAnalyticsSection("reports")}>
          {t("common.goneBackToAnalytics")}
        </Button>
      }
    >
      {children}
    </QueryGate>
  );
}

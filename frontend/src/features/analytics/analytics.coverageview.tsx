import { Button, EmptyState } from "../../design-system/atoms";
import { DataTable } from "../../design-system/datatable";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { formatDateTime } from "../../format/format";
import { type Locale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { openAnalyticsSection } from "../../lib/analytics.address";
import { useCan } from "../../lib/capability";
import { type DataCoverageRow, useDataCoverage } from "./analytics.coverage";
import { sourceName } from "./analytics.forecast.review";

// Every word a coverage row's state can carry, spelled per state so an
// unread source never borrows a read one's copy. A hand-kept mirror of the
// server's own vocabulary; an unknown state renders its raw word rather than
// a wrong sentence.
const COVERAGE_STATE_KEY: Readonly<Record<string, MessageKey>> = {
  checked: "analytics.covChecked",
  stale: "analytics.covStale",
  unavailable: "analytics.covUnavailable",
  permission_limited: "analytics.covPermissionLimited",
  not_connected: "analytics.covNotConnected",
};

// Which connectors the nightly check could read, and how far. Ops-facing: the
// server gates the read, and a seat without the grant never sees the tab.
export function DataCoverageView({
  locale,
  timezone,
}: Readonly<{ locale: Locale; timezone: string }>) {
  const t = useT();
  const coverage = useDataCoverage();
  const allowed = useCan("data_coverage", "read");
  if (!allowed) {
    return <EmptyState>{t("common.permissionDenied")}</EmptyState>;
  }
  return (
    <QueryGate query={coverage} pendingLabel={t("analytics.sectionCoverage")}>
      {(run) =>
        run == null ? (
          <Panel title={t("analytics.sectionCoverage")}>
            <EmptyState>
              {t("analytics.coverageNeverRun")}
              <Button
                variant="link"
                onClick={() => openAnalyticsSection("forecast")}
              >
                {t("analytics.sectionForecast")}
              </Button>
            </EmptyState>
          </Panel>
        ) : (
          <Panel title={t("analytics.sectionCoverage")}>
            <PanelBody>
              <PanelIntro>{t("analytics.coverageSub")}</PanelIntro>
              <DataTable
                label={t("analytics.sectionCoverage")}
                columns={[
                  {
                    key: "source",
                    header: t("analytics.covSource"),
                    render: (row: DataCoverageRow) => sourceName(row.source, t),
                  },
                  {
                    key: "state",
                    header: t("analytics.covState"),
                    render: (row: DataCoverageRow) => {
                      const stateKey = COVERAGE_STATE_KEY[row.state];
                      return stateKey ? t(stateKey) : row.state;
                    },
                  },
                  {
                    key: "through",
                    header: t("analytics.covThrough"),
                    render: (row: DataCoverageRow) =>
                      row.checked_through
                        ? formatDateTime(row.checked_through, locale, timezone)
                        : "—",
                  },
                ]}
                rows={run.sources}
                rowKey={(row) => row.source}
              />
              {/* Record-level input problems live where they are answered: the
                Forecast input review. One resolution surface, not two. */}
              <Button
                variant="link"
                onClick={() => openAnalyticsSection("forecast")}
              >
                {t("review.title")}
              </Button>
              {run.sources.some((source) => source.state !== "checked") && (
                <p className="t-caption">{t("reporting.coverageAction")}</p>
              )}
            </PanelBody>
          </Panel>
        )
      }
    </QueryGate>
  );
}

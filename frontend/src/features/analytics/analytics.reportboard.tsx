// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type Locale, useT } from "../../i18n";
import { AnalyticsBoard, AnalyticsSheet } from "./analytics.board";
import type { Stage } from "./analytics.cells";
import { type AnalyticsScope, scopeKey } from "./analytics.context";
import { type CardKey, ReportCard } from "./analytics.reports";

// A section's report cards as one sheet, with the one that reads as a short
// list in the rail beside the tables.
export function ReportBoard({
  reports,
  stages,
  locale,
  scope,
}: Readonly<{
  reports: readonly CardKey[];
  stages: readonly Stage[];
  locale: Locale;
  scope?: AnalyticsScope;
}>) {
  const t = useT();
  if (reports.length === 0) return null;
  // Keyed by scope too, so an open explanation never outlives the records it
  // explains.
  const card = (report: CardKey) => (
    <ReportCard
      key={scope ? `${report}:${scopeKey(scope)}` : report}
      report={report}
      stages={stages}
      locale={locale}
      scope={scope}
      section
    />
  );
  const railed = reports.filter((report) => RAIL_REPORTS.has(report));
  return (
    <AnalyticsBoard
      railLabel={t("analytics.board.rail")}
      main={
        <AnalyticsSheet>
          {reports
            .filter((report) => !RAIL_REPORTS.has(report))
            .map((report) => card(report))}
        </AnalyticsSheet>
      }
      rail={railed.length > 0 ? railed.map((report) => card(report)) : null}
    />
  );
}

// The probability bands read as a short list beside the pipeline's tables.
// Wider tables stay on the sheet, where every column fits.
const RAIL_REPORTS: ReadonlySet<CardKey> = new Set(["forecast"]);

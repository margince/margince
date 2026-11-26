// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, Suspense } from "react";
import type { components } from "../../api/schema";
import { OverviewHead } from "./analytics.overview.head";
import { OverviewSaveActions } from "./analytics.overview.readings";
import {
  OverviewViewMenu,
  useSalesViewRows,
} from "./analytics.overview.viewmenu";
import { ReviewDecisionsAction } from "./analytics.overview.work";
import { DimensionChips, dimensionCount } from "./reporting.dimensions";
import { ReportingExportButton } from "./reporting.export";
import { ReportingFilters } from "./reporting.filters";
import type {
  ReportingEvaluation,
  ReportingSelection,
} from "./reporting.model";

type Pipeline = components["schemas"]["Pipeline"];

/**
 * Sales review's controls. The view, scope and period stand beside the page's
 * name. The pipeline, the close window and the filters sit behind View. A set
 * filter shows as a chip on the line that says what the figures cover.
 */
export function SalesHeader({
  selection,
  pipelines,
  switcher,
  scopeControl,
  dates,
  caption,
  evaluation,
  charts,
  saved,
  scope,
  onChange,
}: Readonly<{
  selection: ReportingSelection;
  pipelines: readonly Pipeline[];
  switcher: ReactNode;
  scopeControl: ReactNode;
  dates: Parameters<typeof ReportingFilters>[0]["dates"];
  caption?: ReactNode;
  // The answered evaluation, once there is one to save or export.
  evaluation: ReportingEvaluation | undefined;
  // The chart read, whose chart rows the export carries after the figures.
  // The export waits for it, so it never leaves the charts out unsaid.
  charts?: Readonly<{ data?: ReportingEvaluation; isPending: boolean }>;
  saved: ReportingSelection | undefined;
  scope: string;
  onChange: (next: ReportingSelection) => void;
}>) {
  const rows = useSalesViewRows(selection, pipelines, onChange);
  return (
    <>
      <OverviewHead
        switcher={switcher}
        dials={
          <>
            {scopeControl}
            <ReportingFilters
              selection={selection}
              showScope={false}
              showTargets={false}
              showPipeline={false}
              dates={dates}
              onChange={onChange}
            />
          </>
        }
        caption={
          <span className="analytics-chips">
            <DimensionChips
              selection={selection}
              productLabel={evaluation?.context.product_label}
              initiativeLabel={evaluation?.context.initiative_label}
              onChange={onChange}
            />
            {caption != null && <span>{caption}</span>}
          </span>
        }
      />
      <OverviewViewMenu
        rows={rows}
        filterCount={dimensionCount(selection)}
        footer={
          <>
            <OverviewSaveActions selection={saved} scope={scope} />
            <Suspense fallback={null}>
              <ReviewDecisionsAction />
            </Suspense>
            {evaluation && !charts?.isPending && (
              <ReportingExportButton
                evaluation={evaluation}
                charts={charts?.data}
              />
            )}
          </>
        }
      />
    </>
  );
}

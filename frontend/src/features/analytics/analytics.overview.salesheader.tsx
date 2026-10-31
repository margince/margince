// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useState } from "react";
import type { components } from "../../api/schema";
import { Badge, Button } from "../../design-system/atoms";
import { ExpanderChevron } from "../../design-system/disclosure";
import { FilterBar } from "../../design-system/filterbar";
import { OverflowMenu } from "../../design-system/overflowmenu";
import { useT } from "../../i18n";
import { OverviewSaveActions } from "./analytics.overview.readings";
import { ReportingExportButton } from "./reporting.export";
import { ReportingFilters } from "./reporting.filters";
import type {
  ReportingEvaluation,
  ReportingSelection,
} from "./reporting.model";

type Pipeline = components["schemas"]["Pipeline"];

/**
 * Sales review's controls. Scope and period stand open; the pipeline and the
 * close window sit behind Filters, with what they hold always shown as chips.
 * Save view, the templates and the export sit behind More.
 */
export function SalesHeader({
  selection,
  pipelines,
  switcher,
  scopeControl,
  dates,
  caption,
  evaluation,
  saved,
  scope,
  onChange,
}: Readonly<{
  selection: ReportingSelection;
  pipelines: readonly Pipeline[];
  switcher: ReactNode;
  scopeControl: ReactNode;
  dates: Parameters<typeof ReportingFilters>[0]["dates"];
  caption: ReactNode;
  // The answered evaluation, once there is one to save or export.
  evaluation: ReportingEvaluation | undefined;
  saved: ReportingSelection | undefined;
  scope: string;
  onChange: (next: ReportingSelection) => void;
}>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const pipeline = pipelines.find(({ id }) => id === selection.pipeline_id);
  return (
    <FilterBar
      label={t("analytics.filters")}
      quiet
      // The active filters lead the line that says what the figures cover.
      caption={
        <span className="analytics-chips">
          <Badge>
            {t("analytics.overview.chipPipeline", {
              name: pipeline?.name ?? t("reporting.allPipelines"),
            })}
          </Badge>
          <Badge>
            {t("analytics.overview.chipClose", {
              window: t(`reporting.${selection.close_window}`),
            })}
          </Badge>
          <span>{caption}</span>
        </span>
      }
    >
      {switcher}
      {scopeControl}
      <ReportingFilters
        selection={selection}
        showScope={false}
        showTargets={false}
        showPipeline={false}
        dates={dates}
        onChange={onChange}
      />
      <Button
        variant="ghost"
        aria-expanded={open}
        onClick={() => setOpen((was) => !was)}
      >
        {t("analytics.overview.filtersToggle")}
        <ExpanderChevron />
      </Button>
      <OverflowMenu label={t("analytics.overview.more")}>
        <OverviewSaveActions selection={saved} scope={scope} />
        {evaluation && <ReportingExportButton evaluation={evaluation} />}
      </OverflowMenu>
      {open && (
        <ReportingFilters
          selection={selection}
          showScope={false}
          showTargets={false}
          showPeriod={false}
          onChange={onChange}
        />
      )}
    </FilterBar>
  );
}

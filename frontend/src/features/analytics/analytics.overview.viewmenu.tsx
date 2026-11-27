// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useId, useState } from "react";
import type { components } from "../../api/schema";
import { Radio } from "../../design-system/atoms";
import { ListViewMenu, type ViewRow } from "../../design-system/listviewmenu";
import { SlotFill } from "../../design-system/portalslot";
import { useT } from "../../i18n";
import { AnalyticsViewSlot } from "./analytics.tabverbs";
import { dimensionCount } from "./reporting.dimensions";
import { ReportingFilters } from "./reporting.filters";
import type { ReportingSelection } from "./reporting.model";

type Pipeline = components["schemas"]["Pipeline"];

const CLOSE_WINDOWS = ["all_open", "fiscal_quarter"] as const;

/**
 * Sales review's quieter dials, for its View menu: the pipeline, the close
 * window and the filters, each showing what it holds.
 */
export function useSalesViewRows(
  selection: ReportingSelection,
  pipelines: readonly Pipeline[],
  onChange: (next: ReportingSelection) => void,
): ViewRow[] {
  const t = useT();
  const filters = dimensionCount(selection);
  const pipeline = pipelines.find(({ id }) => id === selection.pipeline_id);
  return [
    {
      key: "pipeline",
      label: t("reporting.pipeline"),
      value: pipeline?.name ?? t("reporting.allPipelines"),
      step: () => (
        <ChoiceStep
          value={selection.pipeline_id ?? ""}
          options={[
            { value: "", label: t("reporting.allPipelines") },
            ...pipelines.map(({ id, name }) => ({ value: id, label: name })),
          ]}
          onPick={(value) =>
            onChange({ ...selection, pipeline_id: value || undefined })
          }
        />
      ),
    },
    {
      key: "close",
      label: t("reporting.closeWindow"),
      value: t(`reporting.${selection.close_window}`),
      changed: selection.close_window !== "all_open",
      step: () => (
        <ChoiceStep
          value={selection.close_window}
          options={CLOSE_WINDOWS.map((value) => ({
            value,
            label: t(`reporting.${value}`),
          }))}
          onPick={(value) => {
            const close = CLOSE_WINDOWS.find((window) => window === value);
            if (close) onChange({ ...selection, close_window: close });
          }}
        />
      ),
    },
    {
      key: "filters",
      label: t("analytics.overview.filtersToggle"),
      value: filters > 0 ? String(filters) : t("list.viewMenuNone"),
      changed: filters > 0,
      step: () => (
        <div className="analytics-view-filters">
          <ReportingFilters
            selection={selection}
            showScope={false}
            showPeriod={false}
            showPipeline={false}
            showTargets={false}
            showDimensions
            onChange={onChange}
          />
        </div>
      ),
    },
  ];
}

/**
 * An Overview view's View button at the end of the tab row, as on a list page.
 * It holds the view's quieter dials, with its verbs as the footer.
 */
export function OverviewViewMenu({
  rows = [],
  filterCount = 0,
  footer,
}: Readonly<{
  rows?: readonly ViewRow[];
  filterCount?: number;
  footer: ReactNode;
}>) {
  const [open, setOpen] = useState(false);
  return (
    <SlotFill slot={AnalyticsViewSlot} className="analytics-view-inline">
      <ListViewMenu
        open={open}
        onOpenChange={setOpen}
        filterCount={filterCount}
        changed={rows.some((row) => row.changed)}
        rows={rows}
        footer={footer}
      />
    </SlotFill>
  );
}

// One pick from a short list, as the sort step offers its orders.
function ChoiceStep({
  value,
  options,
  onPick,
}: Readonly<{
  value: string;
  options: readonly Readonly<{ value: string; label: string }>[];
  onPick: (value: string) => void;
}>) {
  const group = useId();
  return (
    <>
      {options.map((option) => (
        <Radio
          key={option.value}
          className="lt-mi"
          name={group}
          label={option.label}
          checked={option.value === value}
          onChange={() => onPick(option.value)}
        />
      ))}
    </>
  );
}

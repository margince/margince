// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { Checkbox, Disclosure } from "../../design-system/atoms";
import { useT } from "../../i18n";
import { CardPreview } from "./reporting.cardpreview";
import { ReportingFilters } from "./reporting.filters";
import {
  cardChoices,
  keptCards,
  layoutOf,
  type ReportingLayout,
} from "./reporting.layout";
import { LayoutEditor } from "./reporting.layouteditor";
import type {
  ReportingBlockKind,
  ReportingMetricID,
  ReportingSelection,
} from "./reporting.model";
import { useMetricNames, useReportingCatalog } from "./reporting.queries";

type Card = ReportingLayout[number]["cards"][number];
type MetricBlocks = ReadonlyMap<
  ReportingMetricID,
  readonly ReportingBlockKind[]
>;

/** What a report is composed of: its selection, its metrics and its cards. */
export type Composition = Readonly<{
  selection: ReportingSelection;
  setSelection: (next: ReportingSelection) => void;
  metrics: readonly ReportingMetricID[];
  setMetrics: (next: readonly ReportingMetricID[]) => void;
  layout: ReportingLayout;
  setLayout: (next: ReportingLayout) => void;
  // The layout with the cards the selected metrics can still draw.
  kept: ReportingLayout;
  metricBlocks: MetricBlocks;
  catalogLoaded: boolean;
  offeredMetrics: readonly ReportingMetricID[];
}>;

/**
 * The state behind composing a report. Which charts a metric brings is in the
 * catalog, so until it has loaded the report's own blocks stand in and no
 * card is judged undrawable.
 */
export function useComposition(initial: ReportingSelection): Composition {
  const [selection, setSelection] = useState(initial);
  const [metrics, setMetrics] = useState<readonly ReportingMetricID[]>(
    initial.metrics,
  );
  const [layout, setLayout] = useState(() => layoutOf(initial));
  const catalog = useReportingCatalog();
  const metricBlocks: MetricBlocks = new Map(
    (catalog.data?.metrics ?? [])
      .filter((metric) => metrics.includes(metric.id))
      .map((metric) => [metric.id, metric.blocks] as const),
  );
  const drawable = (card: Card) => {
    const kind = card.kind;
    if (
      kind === "question" ||
      kind === "assessment" ||
      catalog.data === undefined
    )
      return true;
    return card.metric
      ? (metricBlocks.get(card.metric)?.includes(kind) ?? false)
      : [...metricBlocks.values()].some((blocks) => blocks.includes(kind));
  };
  return {
    selection,
    setSelection,
    metrics,
    setMetrics,
    layout,
    setLayout,
    kept: keptCards(layout, drawable),
    metricBlocks,
    catalogLoaded: catalog.data !== undefined,
    offeredMetrics:
      catalog.data?.metrics.map((metric) => metric.id) ?? initial.metrics,
  };
}

/**
 * The composing controls: the selection's filters, the metrics and the cards
 * in their order. The save dialog and the custom report builder share it, so
 * a report is composed one way wherever it starts.
 */
export function ReportComposition({
  composition,
  onScopeChange,
  showDimensions = false,
  children,
}: Readonly<{
  composition: Composition;
  // Called when the selection's scope moved, for a dialog whose audience
  // follows it.
  onScopeChange?: () => void;
  showDimensions?: boolean;
  // Drawn under the charts, inside the same stack.
  children?: React.ReactNode;
}>) {
  const t = useT();
  const metricName = useMetricNames();
  const { selection, metrics, kept, metricBlocks } = composition;
  return (
    <div className="form-stack">
      <ReportingFilters
        selection={selection}
        showDimensions={showDimensions}
        onChange={(next) => {
          composition.setSelection(next);
          if (
            next.scope.kind !== selection.scope.kind ||
            next.scope.id !== selection.scope.id
          )
            onScopeChange?.();
        }}
      />
      <fieldset className="form-stack">
        <legend>{t("reporting.metrics")}</legend>
        {composition.offeredMetrics.map((metric) => (
          <Checkbox
            key={metric}
            label={metricName(metric)}
            disabled={!composition.catalogLoaded}
            checked={metrics.includes(metric)}
            onChange={(event) =>
              composition.setMetrics(
                event.target.checked
                  ? [...metrics, metric]
                  : metrics.filter((candidate) => candidate !== metric),
              )
            }
          />
        ))}
      </fieldset>
      <Disclosure summary={t("reporting.blocks")}>
        <LayoutEditor
          layout={kept}
          choices={cardChoices(kept, metricBlocks)}
          onChange={composition.setLayout}
          preview={(choice) => (
            <CardPreview
              selection={{ ...selection, metrics: [...metrics] }}
              choice={choice}
            />
          )}
        />
      </Disclosure>
      {children}
    </div>
  );
}

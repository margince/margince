// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReportingChart, ReportingEvidenceRef } from "./reporting.model";

// The records behind one reading. A waterfall's two ends open the stock it
// starts and ends with, which no point carries.
export function selectReading(
  chart: ReportingChart,
  key: string,
  onEvidence: (reference: ReportingEvidenceRef) => void,
) {
  if (
    chart.kind === "pipeline_movement" &&
    (key === "opening" || key === "closing")
  ) {
    onEvidence({ metric: chart.metric, context_id: `movement_${key}` });
    return;
  }
  const reference = chart.points.find((point) => point.key === key)?.evidence;
  if (reference) onEvidence(reference);
}

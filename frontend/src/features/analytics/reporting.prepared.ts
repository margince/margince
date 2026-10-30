// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import type { MessageKey } from "../../i18n/en";
import type { Section } from "../../lib/analytics.address";

type ReportingSelection = components["schemas"]["ReportingSelection"];
type MetricID = components["schemas"]["ReportingMetricID"];
type BlockKind = components["schemas"]["ReportingBlockKind"];

// A prepared report is a question the reporting engine answers, which a reader
// previews and saves a copy of. Or it is a section that already holds the
// answer, which the entry opens.
export type PreparedReport =
  | Readonly<{
      id: string;
      title: MessageKey;
      purpose: MessageKey;
      kind: "question";
      period: ReportingSelection["period"];
      metrics: readonly MetricID[];
      blocks: readonly BlockKind[];
    }>
  | Readonly<{
      id: string;
      title: MessageKey;
      purpose: MessageKey;
      kind: "section";
      section: Section;
      // Shown only to a reader whose own lens is their own records, the one
      // reader that section exists for.
      ownLensOnly?: boolean;
    }>;

export const PREPARED_REPORTS: readonly PreparedReport[] = [
  {
    id: "weekly-sales-review",
    title: "reporting.prepared.weeklyTitle",
    purpose: "reporting.prepared.weeklyPurpose",
    kind: "question",
    period: "last_week",
    metrics: ["bookings_won", "open_pipeline", "closed_win_rate"],
    blocks: ["bookings_trend", "stage_distribution", "owner_attainment"],
  },
  {
    id: "pipeline-health",
    title: "reporting.prepared.healthTitle",
    purpose: "reporting.prepared.healthPurpose",
    kind: "question",
    period: "this_quarter",
    metrics: ["open_pipeline", "stage_age"],
    blocks: ["stage_distribution", "stage_age", "pipeline_movement"],
  },
  {
    id: "my-prospecting-progress",
    title: "reporting.prepared.prospectingTitle",
    purpose: "reporting.prepared.prospectingPurpose",
    kind: "question",
    period: "this_week",
    metrics: [
      "meetings_held",
      "accepted_opportunities",
      "leads_promoted",
      "lead_first_response",
      "leads_awaiting_response",
    ],
    blocks: ["sdr_outcomes", "target_progress"],
  },
  {
    id: "sales-by-owner",
    title: "reporting.prepared.ownerTitle",
    purpose: "reporting.prepared.ownerPurpose",
    kind: "question",
    period: "this_quarter",
    metrics: ["bookings_won", "closed_win_rate"],
    blocks: ["owner_attainment", "target_progress"],
  },
  {
    id: "qualification-review",
    title: "reporting.prepared.qualificationTitle",
    purpose: "reporting.prepared.qualificationPurpose",
    kind: "question",
    period: "this_month",
    metrics: ["qualified_pipeline_created"],
    blocks: ["owner_attainment", "target_progress"],
  },
  {
    id: "personal-outcomes",
    title: "reporting.prepared.personalTitle",
    purpose: "reporting.prepared.personalPurpose",
    kind: "section",
    section: "outcomes",
    ownLensOnly: true,
  },
  {
    id: "account-opportunities",
    title: "reporting.prepared.accountsTitle",
    purpose: "reporting.prepared.accountsPurpose",
    kind: "section",
    section: "pipeline",
  },
  {
    id: "delivery-review",
    title: "reporting.prepared.deliveryTitle",
    purpose: "reporting.prepared.deliveryPurpose",
    kind: "section",
    section: "delivery",
  },
];

// The charts that read one pipeline's stages and so need one named.
const STAGE_BLOCKS: readonly BlockKind[] = [
  "stage_distribution",
  "stage_age",
  "pipeline_movement",
];

// A prepared question as this reader may ask it: only the metrics their
// catalog offers, and only the charts those metrics draw. A pipeline is named
// only where a stage chart needs one, because some metrics refuse a pipeline.
// Null when none of its metrics is theirs to read.
export function preparedSelection(
  report: Extract<PreparedReport, { kind: "question" }>,
  catalog: components["schemas"]["ReportingCatalog"],
  scope: ReportingSelection["scope"],
  pipelineId: string | undefined,
): ReportingSelection | null {
  const offered = catalog.metrics.filter((metric) =>
    report.metrics.includes(metric.id),
  );
  if (offered.length === 0) {
    return null;
  }
  const blocks = report.blocks.filter((block) =>
    offered.some((metric) => metric.blocks.includes(block)),
  );
  return {
    scope,
    pipeline_id: blocks.some((block) => STAGE_BLOCKS.includes(block))
      ? pipelineId
      : undefined,
    period: report.period,
    target_basis: report.period === "this_quarter" ? "fiscal_quarter" : "month",
    close_window: "fiscal_quarter",
    metrics: offered.map((metric) => metric.id),
    blocks,
  };
}

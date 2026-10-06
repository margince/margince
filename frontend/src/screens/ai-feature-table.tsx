// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId } from "react";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { DataTable } from "../design-system/datatable";
import { useTooltip } from "../design-system/tooltip";
import { useT } from "../i18n";
import { tierLabel } from "./ai-decision-labels";
import { decisionFirstOrder } from "./ai-feature-order";
import {
  ImpactBadge,
  TASK_DOT_LABEL,
  TaskDetails,
  taskDot,
} from "./ai-task-details";
import { TaskName } from "./ai-task-name";
import { ModelRef, TermChip } from "./ai-terms";

type Feature = components["schemas"]["AiFeatureRoute"];

// A task's tier is fixed by the task contract, and the binding a tier names is
// edited on the Model tiers card. `onEdit` opens a task's own request settings:
// with it the table is the AI tasks card's, which keeps its figures in the
// sheet and says only which rows an admin customised.
export function AiFeatureTable({
  rows,
  health,
  providers,
  canTrace = false,
  onEdit,
}: Readonly<{
  rows: Feature[];
  onEdit?: (row: Feature) => void;
  // Present for a reader who may see how the lanes answer.
  health?: components["schemas"]["AiHealth"];
  // Which providers refuse calls now, for a reader who may see it.
  providers?: components["schemas"]["AiProviderHealth"];
  // The trace answers on `ai_diagnostics:read` alone (ai/callread.go), so a
  // reader without it is not offered a link onto a withheld card.
  canTrace?: boolean;
}>) {
  const t = useT();
  // The model that answers. A decision model in front of it is told in the
  // name's details, so a decision-first row is as tall as any other.
  const modelCell = (row: Feature) => {
    const lead = row.effective_candidates[0];
    return lead ? (
      <ModelRef provider={lead.provider} model={lead.model} />
    ) : (
      "—"
    );
  };
  return (
    <div className="ai-task-table">
      <DataTable
        label={t("aiAdmin.features")}
        rows={decisionFirstOrder(rows)}
        rowKey={(row) => row.task}
        columns={[
          {
            key: "activity",
            header: t("aiAdmin.activity"),
            grow: true,
            render: (row: Feature) => {
              const dot = taskDot(row, health, providers);
              return (
                <span className="ai-task-name" title={row.task}>
                  {/* Without the lane read an idle dot would claim "no
                      calls"; a task known to be blocked still says so. */}
                  {health || dot === "bad" ? (
                    <span
                      className={`ai-health-dot ai-health-dot-${dot}`}
                      title={t(TASK_DOT_LABEL[dot])}
                    >
                      <span className="sr-only">{t(TASK_DOT_LABEL[dot])}</span>
                    </span>
                  ) : null}
                  <TaskName
                    name={row.display_name}
                    summary={row.summary}
                    details={
                      <TaskDetails
                        row={row}
                        health={health}
                        providers={providers}
                        canTrace={canTrace}
                        custom={
                          onEdit !== undefined &&
                          Object.keys(row.overrides ?? {}).length > 0
                        }
                      />
                    }
                  />
                  {/* The budget preview is read for what a change does to
                      each task, so there the departure stays on the row. */}
                  {onEdit ? null : <ImpactBadge row={row} />}
                </span>
              );
            },
          },
          {
            key: "tier",
            header: t("aiTerms.tier"),
            render: (row: Feature) => (
              <TermChip term="tier">{tierLabel(row.leading_tier, t)}</TermChip>
            ),
          },
          {
            key: "model",
            header: t("aiAdmin.model"),
            render: (row: Feature) => modelCell(row),
          },
          ...(onEdit
            ? [
                {
                  key: "edit",
                  header: t("aiTasks.settings"),
                  align: "end" as const,
                  // The embeddings lane is not a task an admin tunes: it has no
                  // ladder, thinking or timeout of its own.
                  render: (row: Feature) =>
                    row.defaults === undefined ? (
                      <UntunableEdit name={row.display_name} />
                    ) : (
                      <Button
                        onClick={() => onEdit(row)}
                        aria-label={`${t("aiRouting.edit")} ${row.display_name}`}
                      >
                        {t("aiRouting.edit")}
                      </Button>
                    ),
                },
              ]
            : []),
        ]}
      />
    </div>
  );
}

// Edit stays in the column so every row offers the same door, refused with
// where this lane's settings live. The tip rides a wrapper because a disabled
// button takes no pointer events; a screen reader reads the same sentence.
function UntunableEdit({ name }: Readonly<{ name: string }>) {
  const t = useT();
  const reasonId = useId();
  const reason = t("aiTasks.embeddingsEdit");
  const tip = useTooltip<HTMLSpanElement>(reason);
  return (
    <span ref={tip.ref} {...tip.trigger}>
      <Button reasonId={reasonId} aria-label={`${t("aiRouting.edit")} ${name}`}>
        {t("aiRouting.edit")}
      </Button>
      <span id={reasonId} className="sr-only">
        {reason}
      </span>
      {tip.tip}
    </span>
  );
}

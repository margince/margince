// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { DataTable, type DataTableColumn } from "../design-system/datatable";
import { forReader } from "../format/collate";
import { useLocale, useT } from "../i18n";
import { tierLabel, tierRank } from "./ai-decision-labels";
import { decisionFirstOrder } from "./ai-feature-order";
import {
  ImpactBadge,
  TASK_DOT_LABEL,
  TaskDetails,
  taskDot,
} from "./ai-task-details";
import { TaskName } from "./ai-task-name";
import { ModelRef, RowOpen, TermChip } from "./ai-terms";

type Feature = components["schemas"]["AiFeatureRoute"];

type Health = components["schemas"]["AiHealth"];
type Providers = components["schemas"]["AiProviderHealth"];

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
  health?: Health;
  // Which providers refuse calls now, for a reader who may see it.
  providers?: Providers;
  // The trace answers on `ai_diagnostics:read` alone (ai/callread.go), so a
  // reader without it is not offered a link onto a withheld card.
  canTrace?: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const columns: DataTableColumn<Feature>[] = [
    {
      key: "activity",
      header: t("aiAdmin.activity"),
      grow: true,
      render: (row) => (
        <TaskCell
          row={row}
          health={health}
          providers={providers}
          canTrace={canTrace}
          editable={onEdit !== undefined}
        />
      ),
    },
    {
      key: "tier",
      header: t("aiTerms.tier"),
      render: (row) => (
        <TermChip term="tier">{tierLabel(row.leading_tier, t)}</TermChip>
      ),
    },
  ];
  if (!onEdit) {
    // The budget preview shows the model a budget moves each task onto.
    return (
      <DataTable
        label={t("aiAdmin.features")}
        rows={decisionFirstOrder(rows)}
        rowKey={(row) => row.task}
        columns={[
          ...columns,
          { key: "model", header: t("aiAdmin.model"), render: modelCell },
        ]}
      />
    );
  }
  const sorted = [...rows].sort(
    (a, b) =>
      tierRank(a.leading_tier) - tierRank(b.leading_tier) ||
      forReader(a.display_name, b.display_name, locale),
  );
  return (
    <DataTable
      bleed
      fold
      label={t("aiAdmin.features")}
      rows={sorted}
      rowKey={(row) => row.task}
      rowTestId={(row) => `ai-task-row-${row.task}`}
      onRowClick={(row) => {
        if (row.defaults !== undefined) onEdit(row);
      }}
      columns={[
        ...columns,
        {
          key: "edit",
          header: t("aiTasks.settings"),
          headerHidden: true,
          align: "end",
          fold: "end",
          render: (row) => (
            <RowOpen
              label={t("aiRouting.editNamed", { name: row.display_name })}
              // The embeddings lane is not a task an admin tunes: it has no
              // ladder, thinking or timeout of its own.
              refusal={
                row.defaults === undefined
                  ? t("aiTasks.embeddingsEdit")
                  : undefined
              }
              onOpen={() => onEdit(row)}
            />
          ),
        },
      ]}
    />
  );
}

// The model that answers. A decision model in front of it is told in the name's
// details, so a decision-first row is as tall as any other.
function modelCell(row: Feature) {
  const lead = row.effective_candidates[0];
  return lead ? <ModelRef provider={lead.provider} model={lead.model} /> : "—";
}

function TaskCell({
  row,
  health,
  providers,
  canTrace,
  editable,
}: Readonly<{
  row: Feature;
  health: Health | undefined;
  providers: Providers | undefined;
  canTrace: boolean;
  editable: boolean;
}>) {
  const t = useT();
  const dot = taskDot(row, health, providers);
  return (
    <span className="ai-task-name" title={row.display_name}>
      {/* Without the lane read an idle dot would claim "no calls"; a task
          known to be blocked still says so. */}
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
            custom={editable && Object.keys(row.overrides ?? {}).length > 0}
          />
        }
      />
      {editable ? null : <ImpactBadge row={row} />}
    </span>
  );
}

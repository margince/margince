// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { RowOpen } from "../../design-system/rowopen";
import { forReader } from "../../format/collate";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import { aiTaskLabel } from "../../lib/ai-task-label";
import { tierLabel, tierRank } from "./ai-decision-labels";
import { decisionFirstOrder } from "./ai-feature-order";
import type { LaneKeyGap } from "./ai-lane-state";
import {
  ImpactBadge,
  TASK_DOT_LABEL,
  TaskDetails,
  taskDot,
} from "./ai-task-details";
import { TaskName } from "./ai-task-name";
import { ModelRef, TermChip } from "./ai-terms";

type Feature = components["schemas"]["AiFeatureRoute"];

type Health = components["schemas"]["AiHealth"];
type Providers = components["schemas"]["AiProviderHealth"];

export function AiFeatureTable({
  rows,
  health,
  providers,
  lanesNeedingAKey,
  canTrace = false,
  onEdit,
  canManage = false,
}: Readonly<{
  rows: Feature[];
  onEdit?: (row: Feature) => void;
  // Without it a task's sheet opens read-only, so the verb says Open, not Edit.
  canManage?: boolean;
  // Present for a reader who may see how the lanes answer.
  health?: Health;
  // Which providers refuse calls now, for a reader who may see it.
  providers?: Providers;
  // The saved bindings holding no key, so a lane nothing is calling can be
  // told from a lane whose binding cannot run.
  lanesNeedingAKey?: readonly LaneKeyGap[];
  // The trace answers on `ai_diagnostics:read` alone (ai/callread.go), so a
  // reader without it is not offered a link onto a withheld card.
  canTrace?: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const activity: DataTableColumn<Feature> = {
    key: "activity",
    header: t("aiAdmin.activity"),
    grow: true,
    render: (row) => (
      <TaskCell
        row={row}
        health={health}
        providers={providers}
        lanesNeedingAKey={lanesNeedingAKey}
        canTrace={canTrace}
        editable={onEdit !== undefined}
      />
    ),
  };
  if (!onEdit) {
    // The budget preview shows the model a budget moves each task onto.
    return (
      <DataTable
        label={t("aiAdmin.features")}
        rows={decisionFirstOrder(rows)}
        rowKey={(row) => row.task}
        columns={[
          activity,
          {
            key: "tier",
            header: t("aiTerms.tier"),
            render: (row) => (
              <TermChip term="tier">{tierLabel(row.leading_tier, t)}</TermChip>
            ),
          },
          { key: "model", header: t("aiAdmin.model"), render: modelCell },
        ]}
      />
    );
  }
  const edit: DataTableColumn<Feature> = {
    key: "edit",
    header: t("aiTasks.settings"),
    headerHidden: true,
    align: "end",
    render: (row) => (
      <RowOpen
        label={t(canManage ? "aiRouting.editNamed" : "aiRouting.openNamed", {
          name: aiTaskLabel(row.task, t, row.display_name),
        })}
        // The embeddings lane is not a task an admin tunes: it has no
        // ladder, thinking or timeout of its own.
        refusal={
          row.defaults === undefined ? t("aiTasks.embeddingsEdit") : undefined
        }
        opensDialog
        onOpen={() => onEdit(row)}
      />
    ),
  };
  // One table per tier, headed by the tier's name, so no column repeats it.
  return byTier(rows, locale, t).map(({ tier, tasks }) => {
    const name = tierLabel(tier, t);
    return (
      <DataTable
        key={tier}
        bleed
        label={name}
        rows={tasks}
        rowKey={(row) => row.task}
        rowTestId={(row) => `ai-task-row-${row.task}`}
        rowOpens={(row) => row.defaults !== undefined}
        onRowClick={onEdit}
        columns={[{ ...activity, header: name }, edit]}
      />
    );
  });
}

function byTier(
  rows: readonly Feature[],
  locale: Locale,
  t: Translator,
): { tier: string; tasks: Feature[] }[] {
  const groups = new Map<string, Feature[]>();
  for (const row of rows) {
    groups.set(row.leading_tier, [
      ...(groups.get(row.leading_tier) ?? []),
      row,
    ]);
  }
  return [...groups]
    .sort(([a], [b]) => tierRank(a) - tierRank(b))
    .map(([tier, tasks]) => ({
      tier,
      tasks: tasks.sort((a, b) =>
        forReader(
          aiTaskLabel(a.task, t, a.display_name),
          aiTaskLabel(b.task, t, b.display_name),
          locale,
        ),
      ),
    }));
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
  lanesNeedingAKey,
  canTrace,
  editable,
}: Readonly<{
  row: Feature;
  health: Health | undefined;
  providers: Providers | undefined;
  lanesNeedingAKey?: readonly LaneKeyGap[];
  canTrace: boolean;
  editable: boolean;
}>) {
  const t = useT();
  const dot = taskDot(row, health, providers);
  const name = aiTaskLabel(row.task, t, row.display_name);
  return (
    <span className="ai-task-name" title={name}>
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
        name={name}
        summary={row.summary}
        details={
          <TaskDetails
            row={row}
            health={health}
            providers={providers}
            lanesNeedingAKey={lanesNeedingAKey}
            canTrace={canTrace}
            custom={editable && Object.keys(row.overrides ?? {}).length > 0}
          />
        }
      />
      {editable ? null : <ImpactBadge row={row} />}
    </span>
  );
}

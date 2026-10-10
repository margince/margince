// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { useCan, useCanWrite } from "../app/capability";
import { EmptyState } from "../design-system/atoms";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { useT } from "../i18n";
import { AiFeaturesWithheldPanel, useAiStatus } from "./ai-admin";
import { AiFeatureTable } from "./ai-feature-table";
import { useAiHealth } from "./ai-health";
import { useProviderHealth } from "./ai-provider-health";
import { TaskSheet } from "./ai-task-sheet";
import { PanelTitle } from "./ai-terms";
import { QueryStates } from "./common";

// What each AI task runs on right now, under the bindings above it.
//
// A task's tier is fixed by the task contract and bound on the Model tiers
// card; a row here opens the task's own request settings.
export function AiTasksCard() {
  const t = useT();
  const canManage = useCanWrite("ai_routing", "update");
  const [opened, setOpened] = useState<string | null>(null);
  const canDiagnose = useCan("ai_diagnostics", "read");
  const canBudget = useCan("ai_budget", "read");
  const canRoute = useCan("ai_routing", "read");
  const status = useAiStatus(canDiagnose && canBudget);
  const health = useAiHealth(canDiagnose).data;
  const providers = useProviderHealth(canDiagnose).data;
  // `/ai/status` needs all three: without the diagnostics or budget grant
  // there is no payload, and without routing read its task list is empty —
  // either way the withheld panel says so, rather than drawing a table that
  // reads as "no tasks".
  const route = status.data?.features.find((f) => f.task === opened);
  if (!canDiagnose || !canBudget || !canRoute) {
    return <AiFeaturesWithheldPanel />;
  }
  return (
    <Panel title={<PanelTitle term="task">{t("aiTasks.title")}</PanelTitle>}>
      <PanelBody>
        <PanelIntro>{t("aiTasks.intro")}</PanelIntro>
      </PanelBody>
      {status.isSuccess && status.data.features.length > 0 ? (
        <AiFeatureTable
          rows={status.data.features}
          health={health}
          providers={providers}
          canTrace={canDiagnose}
          canManage={canManage}
          onEdit={(row) => setOpened(row.task)}
        />
      ) : (
        <PanelBody>
          <QueryStates query={status} pendingLabel={t("aiTasks.title")}>
            <EmptyState>{t("common.empty")}</EmptyState>
          </QueryStates>
        </PanelBody>
      )}
      {/* Outside the gate, from the last good read: a failed refetch
          behind an open sheet must not take the draft in it away. */}
      {route ? (
        <TaskSheet
          route={route}
          canManage={canManage}
          canSeeCalls={canDiagnose}
          onClose={() => setOpened(null)}
        />
      ) : null}
    </Panel>
  );
}

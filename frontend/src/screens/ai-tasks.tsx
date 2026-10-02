// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { useCan, useCanWrite } from "../app/capability";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { useT } from "../i18n";
import { AiFeaturesWithheldPanel, useAiStatus } from "./ai-admin";
import { AiFeatureTable } from "./ai-feature-table";
import { useAiHealth } from "./ai-health";
import { TaskSheet } from "./ai-task-sheet";
import { PanelTitle } from "./ai-terms";
import { QueryGate } from "./common";

// What each AI task runs on right now, under the bindings above it.
//
// Read-only: a task's tier is fixed by the task contract, and what that tier
// is bound to is edited on the Model tiers card. The row leads with the
// resolved model — the chain after the budget and the decision model have had
// their say — rather than with the policy's first pick.
export function AiTasksCard() {
  const t = useT();
  const canManage = useCanWrite("ai_routing", "update");
  const [opened, setOpened] = useState<string | null>(null);
  const canDiagnose = useCan("ai_diagnostics", "read");
  const canBudget = useCan("ai_budget", "read");
  const canRoute = useCan("ai_routing", "read");
  const status = useAiStatus(canDiagnose && canBudget);
  const health = useAiHealth(canDiagnose).data;
  // `/ai/status` needs all three: without the diagnostics or budget grant
  // there is no payload, and without routing read its task list is empty —
  // either way the withheld panel says so, rather than drawing a table that
  // reads as "no tasks".
  if (!canDiagnose || !canBudget || !canRoute) {
    return <AiFeaturesWithheldPanel />;
  }
  return (
    <Panel title={<PanelTitle term="task">{t("aiTasks.title")}</PanelTitle>}>
      <PanelBody>
        <PanelIntro>{t("aiTasks.intro")}</PanelIntro>
        <QueryGate query={status} pendingLabel={t("aiTasks.title")}>
          {(current) => {
            const route = current.features.find((f) => f.task === opened);
            return (
              <>
                <AiFeatureTable
                  rows={current.features}
                  health={health}
                  canTrace={canDiagnose}
                  onEdit={(row) => setOpened(row.task)}
                />
                {route ? (
                  <TaskSheet
                    route={route}
                    canManage={canManage}
                    canSeeCalls={canDiagnose}
                    onClose={() => setOpened(null)}
                  />
                ) : null}
              </>
            );
          }}
        </QueryGate>
      </PanelBody>
    </Panel>
  );
}

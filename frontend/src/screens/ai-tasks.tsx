// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useCan } from "../app/capability";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { useT } from "../i18n";
import {
  AiFeaturesWithheldPanel,
  AiFeatureTable,
  useAiStatus,
} from "./ai-admin";
import { QueryGate } from "./common";

// What each AI task runs on right now, under the bindings above it.
//
// Read-only: a task's tier is fixed by the task contract, and what that tier
// is bound to is edited on the Model tiers card. The row leads with the
// resolved model — the chain after the budget and the decision model have had
// their say — rather than with the policy's first pick.
export function AiTasksCard() {
  const t = useT();
  const canDiagnose = useCan("ai_diagnostics", "read");
  const canBudget = useCan("ai_budget", "read");
  const canRoute = useCan("ai_routing", "read");
  const status = useAiStatus(canDiagnose && canBudget);
  // `/ai/status` needs all three: without the diagnostics or budget grant
  // there is no payload, and without routing read its task list is empty —
  // either way the withheld panel says so, rather than drawing a table that
  // reads as "no tasks".
  if (!canDiagnose || !canBudget || !canRoute) {
    return <AiFeaturesWithheldPanel />;
  }
  return (
    <Panel title={t("aiTasks.title")}>
      <PanelBody>
        <PanelIntro>{t("aiTasks.intro")}</PanelIntro>
        <QueryGate query={status} pendingLabel={t("aiTasks.title")}>
          {(current) => <AiFeatureTable rows={current.features} />}
        </QueryGate>
      </PanelBody>
    </Panel>
  );
}

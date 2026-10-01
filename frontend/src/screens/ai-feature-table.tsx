// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { routeHash } from "../app/router";
import { hashWithParams } from "../app/urlstate";
import { Badge } from "../design-system/atoms";
import { CellStack } from "../design-system/cellstack";
import { DataTable } from "../design-system/datatable";
import { useT } from "../i18n";
import { decisionSkipLabel, tierLabel } from "./ai-decision-labels";
import { decisionFirstOrder } from "./ai-feature-order";
import { TaskState } from "./ai-lane-state";
import { ModelChain, ModelRef, TermChip } from "./ai-terms";
import { CALL_TASK_PARAM } from "./aicalls";
import { settingsHref } from "./settingsrouting";

type Feature = components["schemas"]["AiFeatureRoute"];

// Read-only by design: a task's tier is fixed by the task contract, and the
// binding a tier names is edited on the Model tiers card.
export function AiFeatureTable({
  rows,
  health,
  canTrace = false,
}: Readonly<{
  rows: Feature[];
  // Present for a reader who may see how the lanes answer.
  health?: components["schemas"]["AiHealth"];
  // The trace answers on `ai_diagnostics:read` alone (ai/callread.go), so a
  // reader without it is not offered a link onto a withheld card.
  canTrace?: boolean;
}>) {
  const t = useT();
  // Only a departure from the default is worth a badge; the unchanged case is
  // what every quiet row already says.
  const impact = (row: Feature) => {
    switch (row.impact) {
      case "budget_blocked":
        return t("aiAdmin.impact.blocked");
      case "model_changed":
        return t("aiAdmin.impact.model");
      case "decision_changed":
        return t("aiAdmin.impact.decision");
      case "fallback_changed":
        return t("aiAdmin.impact.fallback");
      case "unconfigured":
        return t("aiAdmin.impact.unconfigured");
      case "unchanged":
        return row.budget_exempt ? t("aiAdmin.impact.exempt") : null;
      default:
        // A new impact must be named here rather than read as "unchanged".
        return row.impact satisfies never;
    }
  };
  // Which model answers, and where the decision model stands in front of the
  // ladder. Only the lead is summarized: the rungs behind it are the tiers
  // above, and repeating them per task is the same list said again.
  const summary = (row: Feature) => {
    const lead = row.effective_candidates[0];
    const ladder = <ModelRef provider={lead.provider} model={lead.model} />;
    const decision = row.decision_candidate;
    if (!row.decision_first || !decision) {
      return ladder;
    }
    return (
      <ModelChain
        steps={[decision, lead]}
        connector={t("aiAdmin.thenLadder")}
      />
    );
  };
  const modelCell = (row: Feature) =>
    row.effective_candidates.length ? summary(row) : "—";
  return (
    <DataTable
      label={t("aiAdmin.features")}
      rows={decisionFirstOrder(rows)}
      rowKey={(row) => row.task}
      columns={[
        {
          key: "activity",
          header: t("aiAdmin.activity"),
          render: (row: Feature) => {
            const changed = impact(row);
            return (
              <CellStack>
                <span>{row.display_name}</span>
                <span className="t-caption">
                  {row.task} · {row.execution_mode}
                </span>
                {row.decision_first ? (
                  <Badge>{t("aiTasks.decisionFirst")}</Badge>
                ) : null}
                <TaskState
                  health={health}
                  tier={row.leading_tier}
                  decisionFirst={row.decision_first === true}
                />
                {changed ? (
                  <Badge
                    tone={
                      row.impact === "budget_blocked" ||
                      row.impact === "unconfigured"
                        ? "warning"
                        : undefined
                    }
                  >
                    {changed}
                  </Badge>
                ) : null}
              </CellStack>
            );
          },
        },
        {
          key: "tier",
          header: t("aiTerms.tier"),
          render: (row: Feature) => (
            <CellStack>
              <TermChip term="tier">{tierLabel(row.leading_tier, t)}</TermChip>
              {canTrace ? (
                <a className="link-button t-caption" href={callsHref(row.task)}>
                  {t("aiTasks.viewCalls")}
                </a>
              ) : null}
            </CellStack>
          ),
        },
        {
          key: "model",
          header: t("aiAdmin.model"),
          render: (row: Feature) => (
            <>
              {modelCell(row)}
              {row.decision_skip_reason ? (
                <p className="t-caption">
                  {decisionSkipLabel(row.decision_skip_reason, t)}
                </p>
              ) : null}
            </>
          ),
        },
      ]}
    />
  );
}

/** The call trace narrowed to one task: where a task row sends its reader. */
function callsHref(task: string): string {
  return hashWithParams(
    routeHash(settingsHref("model-calls")),
    new Map([[CALL_TASK_PARAM, task]]),
  );
}

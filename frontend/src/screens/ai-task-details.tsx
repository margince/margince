// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { Badge } from "../design-system/atoms";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { decisionSkipLabel } from "./ai-decision-labels";
import { laneRung, TaskState } from "./ai-lane-state";
import { ProviderHealthNotice } from "./ai-provider-health-notice";
import { providerName } from "./ai-provider-names";
import { TermChip } from "./ai-terms";
import { CALL_TASK_PARAM, callsHrefFor } from "./aicalls";

// What a task row knows beyond its name, opened from the name so every row in
// the AI tasks table stays one line: its states, and what its calls do while
// the provider they need is not answering.

type Feature = components["schemas"]["AiFeatureRoute"];
type Health = components["schemas"]["AiHealth"];
type ProviderHealth = components["schemas"]["AiProviderHealth"];
type HealthEntry = components["schemas"]["AiProviderHealthEntry"];

export type TaskDot = "ok" | "bad" | "idle";

/** What the dot says, for a reader who cannot tell its colours apart. */
export const TASK_DOT_LABEL = {
  ok: "aiTasks.dot.ok",
  bad: "aiTasks.dot.bad",
  idle: "aiTasks.dot.idle",
} as const satisfies Record<TaskDot, MessageKey>;

/**
 * The providers on this task's chain that refuse calls now. `degraded` still
 * takes calls, so it blocks nothing (docs/explanation/ai-provider-health.md).
 */
function blockedOnChain(
  row: Feature,
  providers: ProviderHealth | undefined,
): HealthEntry[] {
  const chain = new Set(row.effective_candidates.map((c) => c.provider));
  if (row.decision_first && row.decision_candidate) {
    chain.add(row.decision_candidate.provider);
  }
  return (providers?.providers ?? []).filter(
    (entry) => entry.health !== "degraded" && chain.has(entry.provider),
  );
}

/** The router skips a blocked rung, so only a chain blocked end to end defers. */
function deferredNow(row: Feature, blocked: readonly HealthEntry[]): boolean {
  const down = new Set(blocked.map((entry) => entry.provider));
  return (
    row.effective_candidates.length > 0 &&
    row.effective_candidates.every((c) => down.has(c.provider))
  );
}

function decisionBlocked(row: Feature, blocked: readonly HealthEntry[]) {
  const provider = row.decision_candidate?.provider;
  return blocked.some((entry) => entry.provider === provider);
}

/** A blocked decision model is skipped, so the ladder is what answers. */
function startsOnDecision(row: Feature, blocked: readonly HealthEntry[]) {
  return row.decision_first === true && !decisionBlocked(row, blocked);
}

/** The row's one-glance state, drawn as the Model tiers card draws a lane's. */
export function taskDot(
  row: Feature,
  health: Health | undefined,
  providers: ProviderHealth | undefined,
): TaskDot {
  const blocked = blockedOnChain(row, providers);
  if (
    row.impact === "budget_blocked" ||
    row.impact === "unconfigured" ||
    deferredNow(row, blocked)
  ) {
    return "bad";
  }
  const rung = health
    ? laneRung(health, row.leading_tier, startsOnDecision(row, blocked))
    : undefined;
  if (!rung) return "idle";
  return rung.healthy ? "ok" : "bad";
}

export function TaskDetails({
  row,
  health,
  providers,
  canTrace,
  custom,
}: Readonly<{
  row: Feature;
  health: Health | undefined;
  providers: ProviderHealth | undefined;
  canTrace: boolean;
  custom: boolean;
}>) {
  const t = useT();
  return (
    <div className="ai-task-details">
      <p className="t-caption">
        {row.task} · {row.execution_mode}
      </p>
      {row.decision_first && row.decision_candidate ? (
        <span className="ai-task-details-badges">
          <Badge>{t("aiTasks.decisionFirst")}</Badge>
          <TermChip term="provider">
            {providerName(row.decision_candidate.provider, t)}
          </TermChip>
          <code>{row.decision_candidate.model}</code>
        </span>
      ) : null}
      <span className="ai-task-details-badges">
        {custom ? <Badge tone="accent">{t("aiTasks.custom")}</Badge> : null}
        <TaskState
          health={health}
          tier={row.leading_tier}
          decisionFirst={startsOnDecision(row, blockedOnChain(row, providers))}
        />
        <ImpactBadge row={row} />
      </span>
      {row.decision_skip_reason ? (
        <p className="t-caption">
          {decisionSkipLabel(row.decision_skip_reason, t)}
        </p>
      ) : null}
      <OutageNote row={row} providers={providers} />
      {canTrace ? (
        <a
          className="t-caption"
          href={callsHrefFor({ [CALL_TASK_PARAM]: row.task })}
        >
          {t("aiTasks.viewCalls")}
        </a>
      ) : null}
    </div>
  );
}

// What the task does once a provider it needs refuses calls, and whether one
// does now, keyed on the execution mode /ai/status sends. The generated
// docs/reference/ai-provider-outages.md states the same rule per task.
const OUTAGE_RULE = {
  background: {
    rule: "aiTasks.deferral.background",
    now: "aiTasks.deferral.nowBackground",
  },
  interactive: {
    rule: "aiTasks.deferral.interactive",
    now: "aiTasks.deferral.nowInteractive",
  },
  embedding: {
    rule: "aiTasks.deferral.embedding",
    now: "aiTasks.deferral.nowEmbedding",
  },
  degrades: {
    rule: "aiTasks.deferral.degrades",
    now: "aiTasks.deferral.nowDegrades",
  },
} as const satisfies Record<string, { rule: MessageKey; now: MessageKey }>;

function outageRule(row: Feature) {
  if (row.degrades_on_outage === true) return OUTAGE_RULE.degrades;
  const mode = row.execution_mode;
  if (mode === "background" || mode === "embedding") return OUTAGE_RULE[mode];
  return OUTAGE_RULE.interactive;
}

function OutageNote({
  row,
  providers,
}: Readonly<{ row: Feature; providers: ProviderHealth | undefined }>) {
  const t = useT();
  const blocked = blockedOnChain(row, providers);
  const rule = outageRule(row);
  const lead = row.effective_candidates[0]?.provider;
  let now: string | null = null;
  if (deferredNow(row, blocked)) now = t(rule.now);
  else if (
    blocked.some((entry) => entry.provider === lead) ||
    (row.decision_first === true && decisionBlocked(row, blocked))
  )
    now = t("aiTasks.deferral.skipping");
  return (
    <>
      <p className="t-caption">{t(rule.rule)}</p>
      {now ? <p>{now}</p> : null}
      {blocked.map((entry) => (
        <ProviderHealthNotice key={entry.provider} entry={entry} />
      ))}
    </>
  );
}

// Only a departure from the default is worth a badge; the unchanged case is
// what every quiet row already says.
export function ImpactBadge({ row }: Readonly<{ row: Feature }>) {
  const t = useT();
  let label: string | null;
  switch (row.impact) {
    case "budget_blocked":
      label = t("aiAdmin.impact.blocked");
      break;
    case "model_changed":
      label = t("aiAdmin.impact.model");
      break;
    case "decision_changed":
      label = t("aiAdmin.impact.decision");
      break;
    case "fallback_changed":
      label = t("aiAdmin.impact.fallback");
      break;
    case "unconfigured":
      label = t("aiAdmin.impact.unconfigured");
      break;
    case "unchanged":
      label = row.budget_exempt ? t("aiAdmin.impact.exempt") : null;
      break;
    default:
      // A new impact must be named here rather than read as "unchanged".
      return row.impact satisfies never;
  }
  if (!label) return null;
  const urgent =
    row.impact === "budget_blocked" || row.impact === "unconfigured";
  return <Badge tone={urgent ? "warning" : undefined}>{label}</Badge>;
}

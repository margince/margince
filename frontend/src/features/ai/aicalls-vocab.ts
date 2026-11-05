// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import type { StatusKind } from "../../design-system/statusbadge";
import { formatAiRunCost } from "../../format/aicost";
import type { Locale, useT } from "../../i18n";

type T = ReturnType<typeof useT>;
export type AiCallSummary = components["schemas"]["AiCallSummary"];

// The server buckets each call (callfilter.go `callOutcomeSQL`) and filters on
// the same expression. The badge, the dial and the drawer read the bucket
// rather than re-deriving it from the sentinel.
export type CallOutcome = components["schemas"]["AiCallOutcome"];
export const CALL_OUTCOMES = [
  "answered",
  "fallback",
  "declined",
  "failed",
] as const satisfies readonly CallOutcome[];

/**
 * The attempts the ladder itself made. A decision model that fell back is the
 * first of `calls_attempted`. The rung after it is a second model asked, not a
 * retry of the first.
 */
export function ladderAttempts(call: AiCallSummary): number {
  return call.decision_attempted
    ? call.calls_attempted - 1
    : call.calls_attempted;
}

export const OUTCOME_KIND: Readonly<Record<CallOutcome, StatusKind>> = {
  answered: "done",
  fallback: "attention",
  declined: "neutral",
  failed: "failure",
};

/**
 * What the call cost, as the usage page prices it. An unpriced call says so in
 * words: a blank would read as free.
 */
export function costLabel(call: AiCallSummary, locale: Locale, t: T): string {
  return call.estimated_cost_microusd === undefined
    ? t("aicalls.cost.unpriced")
    : formatAiRunCost(call.estimated_cost_microusd, locale);
}

export function outcomeLabel(outcome: CallOutcome, t: T): string {
  switch (outcome) {
    case "answered":
      return t("aicalls.outcome.ok");
    case "fallback":
      return t("aicalls.outcome.fallback");
    case "declined":
      return t("aicalls.outcome.declined");
    case "failed":
      return t("aicalls.outcome.error");
  }
}

// The task as a screen names it, or its key where it has no name.
export function taskName(call: AiCallSummary): string {
  return call.task_display_name ?? call.task;
}

// The served model as one string, the spelling the model dial and search use.
export function modelLabel(call: AiCallSummary): string {
  return call.served_model || call.model_id;
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { formatMicroUSD } from "../format/format";
import type { Locale, useT } from "../i18n";
import { sentenceCaseToken } from "./settings.auditlog.vocab";

type T = ReturnType<typeof useT>;
export type AiCallSummary = components["schemas"]["AiCallSummary"];

// The server buckets each call (callfilter.go `callOutcomeSQL`) and filters on
// the same expression, so the badge, the dial and the drawer read the bucket
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
 * first of `calls_attempted`, and the rung after it is a second model asked,
 * not a retry of the first.
 */
export function ladderAttempts(call: AiCallSummary): number {
  return call.decision_attempted
    ? call.calls_attempted - 1
    : call.calls_attempted;
}

export const OUTCOME_TONE = {
  answered: "success",
  fallback: "warning",
  declined: "default",
  failed: "danger",
} as const satisfies Record<CallOutcome, string>;

/**
 * What the call cost, as the usage page prices it. An unpriced call says so in
 * words: a blank would read as free.
 */
export function costLabel(call: AiCallSummary, locale: Locale, t: T): string {
  return call.estimated_cost_microusd === undefined
    ? t("aicalls.cost.unpriced")
    : formatMicroUSD(call.estimated_cost_microusd, locale);
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

// The failure codes classifyError writes. The wire widens without notice, so an
// unknown code is spelled in words from its own key rather than refused.
export function sentinelLabel(sentinel: string, t: T): string {
  switch (sentinel) {
    case "provider_quota":
      return t("aicalls.error.provider_quota");
    case "provider_throttled":
      return t("aicalls.error.provider_throttled");
    case "provider_refused":
      return t("aicalls.error.provider_refused");
    case "provider_error":
      return t("aicalls.error.provider_error");
    case "output_withheld":
      return t("aicalls.error.output_withheld");
    case "request_rejected":
      return t("aicalls.error.request_rejected");
    case "budget_deferred":
      return t("aicalls.error.budget_deferred");
    case "budget_unavailable":
      return t("aicalls.error.budget_unavailable");
    case "metering_failed":
      return t("aicalls.error.metering_failed");
    case "request_failed":
      return t("aicalls.error.request_failed");
    default:
      return sentenceCaseToken(sentinel);
  }
}

// A task is a wire key the trace carries without a display name, so it is
// spelled in words here; the key itself stays in the drawer's technical facts.
export function taskLabel(task: string): string {
  return sentenceCaseToken(task);
}

// The served model as one string, the spelling the model dial and search use.
export function modelLabel(call: AiCallSummary): string {
  return call.served_model || call.model_id;
}

export function identitySourceLabel(source: string, t: T): string {
  switch (source) {
    case "response":
      return t("aicalls.source.response");
    case "echo":
      return t("aicalls.source.echo");
    case "configured":
      return t("aicalls.source.configured");
    default:
      return sentenceCaseToken(source);
  }
}

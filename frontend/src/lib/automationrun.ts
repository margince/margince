// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";

// How an automation run ended and why, in the reader's language. The run
// history and the Home brief both name a run, and both read these maps.

type Outcome = components["schemas"]["AutomationRun"]["outcome"];

// A Record over the enum stops compiling when the contract adds an outcome.
export const RUN_OUTCOME_WORD: Readonly<Record<Outcome, MessageKey>> = {
  fired: "auto.runs.outcomeFired",
  failed: "auto.runs.outcomeFailed",
  blocked: "auto.runs.outcomeBlocked",
  skipped: "auto.runs.outcomeSkipped",
  queued_for_approval: "auto.runs.outcomeQueued",
};

// The server's run reason codes (automation.RunReasonCodes), held to them by
// backend/gates/frontendrunreasons_test.go.
export const RUN_REASON_WORDS: Readonly<Record<string, MessageKey>> = {
  target_missing: "auto.runs.reasonCode.target_missing",
  not_permitted: "auto.runs.reasonCode.not_permitted",
  conflict: "auto.runs.reasonCode.conflict",
  record_changed: "auto.runs.reasonCode.record_changed",
  no_consent: "auto.runs.reasonCode.no_consent",
  budget_exhausted: "auto.runs.reasonCode.budget_exhausted",
  not_completed: "auto.runs.reasonCode.not_completed",
  blocked: "auto.runs.reasonCode.blocked",
};

function isOutcome(word: string): word is Outcome {
  return Object.hasOwn(RUN_OUTCOME_WORD, word);
}

/** The outcome as a word; one from a newer server reads as a plain failure. */
export function runOutcomeWord(outcome: string, t: Translator): string {
  return t(
    isOutcome(outcome) ? RUN_OUTCOME_WORD[outcome] : "auto.runs.outcomeFailed",
  );
}

/** Why a run stopped; a code from a newer server reads as the generic case. */
export function runReasonWords(code: string, t: Translator): string {
  return t(RUN_REASON_WORDS[code] ?? "auto.runs.reasonCode.not_completed");
}

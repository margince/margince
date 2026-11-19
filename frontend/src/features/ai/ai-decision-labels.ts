// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import type { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { UNKNOWN_VALUE_LABEL } from "../../lib/labelfor";

type T = ReturnType<typeof useT>;

// How a task runs, as /ai/status sends it. A mode this build cannot name is
// shown as sent.
const MODE_NAMES = {
  interactive: "aiTasks.mode.interactive",
  background: "aiTasks.mode.background",
  embedding: "aiTasks.mode.embedding",
} as const satisfies Record<string, MessageKey>;

function isMode(mode: string): mode is keyof typeof MODE_NAMES {
  return Object.hasOwn(MODE_NAMES, mode);
}

export function executionModeLabel(mode: string, t: T): string {
  return isMode(mode) ? t(MODE_NAMES[mode]) : mode;
}

type CodeWords = Readonly<{
  // Standing alone: a badge, a name over its code.
  name: MessageKey;
  // Inside a sentence, after a count: "3 timed out".
  phrase: MessageKey;
  // Not a failure: the call was served, or put off to run later.
  warns?: true;
}>;

// Every code `classifyError` files on a call (backend ai/callstore.go), and the
// reasons a decision walk gives up on. ai-decision-labels.test.ts reads the Go.
const CALL_CODES: Readonly<Record<string, CodeWords>> = {
  provider_error: {
    name: "aicalls.sentinel.provider_error",
    phrase: "aiOutcome.gaveUp.failed",
  },
  provider_quota: {
    name: "aicalls.sentinel.provider_quota",
    phrase: "aiOutcome.gaveUp.quota",
  },
  provider_throttled: {
    name: "aicalls.sentinel.provider_throttled",
    phrase: "aiOutcome.gaveUp.throttled",
  },
  provider_refused: {
    name: "aicalls.sentinel.provider_refused",
    phrase: "aiOutcome.gaveUp.refused",
  },
  timeout: {
    name: "aicalls.sentinel.timeout",
    phrase: "aiOutcome.gaveUp.timeout",
  },
  canceled: {
    name: "aicalls.sentinel.canceled",
    phrase: "aiOutcome.gaveUp.canceled",
    warns: true,
  },
  output_withheld: {
    name: "aicalls.sentinel.output_withheld",
    phrase: "aiOutcome.gaveUp.withheld",
  },
  output_rejected: {
    name: "aicalls.sentinel.output_rejected",
    phrase: "aiOutcome.gaveUp.invalid",
  },
  request_rejected: {
    name: "aicalls.sentinel.request_rejected",
    phrase: "aiOutcome.gaveUp.rejected",
  },
  request_failed: {
    name: "aicalls.sentinel.request_failed",
    phrase: "aiOutcome.gaveUp.notSent",
  },
  budget_deferred: {
    name: "aicalls.sentinel.budget_deferred",
    phrase: "aiOutcome.gaveUp.deferred",
    warns: true,
  },
  budget_unavailable: {
    name: "aicalls.sentinel.budget_unavailable",
    phrase: "aiOutcome.gaveUp.budgetUnavailable",
  },
  metering_failed: {
    name: "aicalls.sentinel.metering_failed",
    phrase: "aiOutcome.gaveUp.meteringFailed",
    warns: true,
  },
  decision_error: {
    name: "aicalls.reason.decision_error",
    phrase: "aiOutcome.gaveUp.failed",
  },
  decision_below_floor: {
    name: "aicalls.reason.decision_below_floor",
    phrase: "aiOutcome.gaveUp.unsure",
  },
  decision_off_enum: {
    name: "aicalls.reason.decision_off_enum",
    phrase: "aiOutcome.gaveUp.offEnum",
  },
  schema_invalid: {
    name: "aicalls.sentinel.schema_invalid",
    phrase: "aiOutcome.gaveUp.invalid",
  },
};

export const KNOWN_CALL_CODES: readonly string[] = Object.keys(CALL_CODES);

/** A code standing alone. The server adds codes, so an unknown one is some failure. */
export function callCodeName(code: string, t: T): string {
  const words = CALL_CODES[code];
  return words ? t(words.name) : t("aicalls.outcome.failed");
}

/** Whether the code is one this screen has words for; an unknown one shows its code. */
export function isKnownCallCode(code: string): boolean {
  return Object.hasOwn(CALL_CODES, code);
}

export function callCodeTone(code: string): "warning" | "danger" {
  return CALL_CODES[code]?.warns ? "warning" : "danger";
}

/** A code inside a sentence; an unknown one is quoted as sent. */
export function gaveUpLabel(code: string, t: T): string {
  const words = CALL_CODES[code];
  return words ? t(words.phrase) : code;
}

// Why an attempt ran. An explicit switch rather than a key built from the wire
// string. The catalog is a closed union, and the server widens the free reason
// string without notice. An unknown reason is shown as sent, never a refusal.
export function attemptReasonLabel(reason: string, t: T): string {
  switch (reason) {
    case "decision_below_floor":
      return t("aicalls.reason.decision_below_floor");
    case "decision_error":
      return t("aicalls.reason.decision_error");
    case "decision_off_enum":
      return t("aicalls.reason.decision_off_enum");
    case "decision_state_too_large":
      return t("aicalls.reason.decision_state_too_large");
    case "decision_uncertified":
      return t("aicalls.reason.decision_uncertified");
    case "provider_error":
      return t("aicalls.reason.provider_error");
    case "schema_invalid":
      return t("aicalls.reason.schema_invalid");
    case "budget_degrade":
      return t("aicalls.reason.budget_degrade");
    case "timeout":
      return t("aicalls.reason.timeout");
    default:
      return reason || "—";
  }
}

type SkipReason = NonNullable<
  components["schemas"]["AiFeatureRoute"]["decision_skip_reason"]
>;

// Why a feature that declares a decision form is not answered by the decision
// model. A value added to the contract fails the typecheck here; one a newer
// server sends reads the neutral label.
export function decisionSkipLabel(reason: SkipReason, t: T): string {
  switch (reason) {
    case "unbound":
      return t("aiAdmin.decisionSkip.unbound");
    case "uncertified":
      return t("aiAdmin.decisionSkip.uncertified");
    default:
      return t(UNKNOWN_VALUE_LABEL);
  }
}

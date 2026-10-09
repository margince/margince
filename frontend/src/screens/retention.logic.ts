import type { components } from "../api/schema";
import type { MessageKey } from "../i18n/en";
import { problemCodeOf } from "./common";

// The retention-authoring screen's pure half: the authorable vocabulary, the
// three states a stored policy can be in, and the two server refusals this
// surface has to tell apart.

// The two cache entries this screen reads, named once because they move
// together: the posture decides `suppressed_by_posture` on every policy row, so
// a posture write has to invalidate the policy list as well as itself.
export const RETENTION_SETTINGS_KEY = ["retention-settings"] as const;
export const RETENTION_POLICIES_KEY = ["retention-policies"] as const;

export type RetentionScope = components["schemas"]["RetentionScope"];
export type RetentionAction = components["schemas"]["RetentionAction"];
export type RetentionPolicy = components["schemas"]["RetentionPolicy"];

// The authorable set, in the contract enum's own order (coarse scopes before
// the finer ones inside them), so the create form reads top-down the way the
// data model nests. Typed as the generated union, so a scope REMOVED from
// crm.yaml is a compile error here.
//
// A scope ADDED there is not — an ordered array cannot be exhaustively checked
// by the type system, and the failure is silent: the new scope simply never
// appears in the create form, so an admin cannot author a policy the server
// would accept. SCOPE_LABEL_KEYS below IS an exhaustive Record over the same
// union, and retention.logic.test.ts asserts this array covers its keys — that
// test is the gate, not this comment.
export const RETENTION_SCOPES: readonly RetentionScope[] = [
  "lead/unconverted",
  "activity",
  "activity/transcript",
  "contact/no_consent_no_deal",
  "deal/lost",
  "deal/won",
  "ai_call_payload/content",
  "raw_capture",
  "deal_risk_day",
  "report_edition",
];

// Ordered by how much they take away — archive keeps the record, erase does
// not. The order is the reader's warning.
export const RETENTION_ACTIONS: readonly RetentionAction[] = [
  "archive",
  "anonymize",
  "erase",
];

// A scope is a wire identifier ("deal/won"), not words. Keyed on the union so
// a widened enum fails to compile here rather than rendering a raw slug at a
// reader who has no way to know what it selects.
export const SCOPE_LABEL_KEYS: Record<RetentionScope, MessageKey> = {
  report_edition: "retention.scopeReportEdition",
  "lead/unconverted": "retention.scopeLeadUnconverted",
  activity: "retention.scopeActivity",
  "activity/transcript": "retention.scopeActivityTranscript",
  "contact/no_consent_no_deal": "retention.scopeContactNoConsentNoDeal",
  "deal/lost": "retention.scopeDealLost",
  "deal/won": "retention.scopeDealWon",
  "ai_call_payload/content": "retention.scopeAiCallPayloadContent",
  raw_capture: "retention.scopeRawCapture",
  deal_risk_day: "retention.scopeDealRiskDay",
};

export function scopeLabelKey(scope: RetentionScope): MessageKey {
  return SCOPE_LABEL_KEYS[scope];
}

export const ACTION_LABEL_KEYS: Record<RetentionAction, MessageKey> = {
  archive: "retention.actionArchive",
  anonymize: "retention.actionAnonymize",
  erase: "retention.actionErase",
};

export function actionLabelKey(action: RetentionAction): MessageKey {
  return ACTION_LABEL_KEYS[action];
}

/**
 * What a stored policy does on the next retention pass.
 *
 * Three states, because "enabled" alone does not answer the question the
 * screen exists to answer: the retain-only posture overrides a destructive
 * rule while leaving it enabled, so an enabled row can be inert. Being off
 * dominates being suppressed — a disabled rule would not act whatever the
 * posture said, and naming the posture there would blame the wrong thing.
 */
export type PolicyEffect = "acting" | "suppressed" | "disabled";

export function policyEffect(
  policy: Readonly<{ enabled: boolean; suppressed_by_posture: boolean }>,
): PolicyEffect {
  if (!policy.enabled) {
    return "disabled";
  }
  return policy.suppressed_by_posture ? "suppressed" : "acting";
}

// An acting policy carries no status: the intro says every enabled policy acts
// on the retention schedule, so a row speaks only when it differs.
const EFFECT_BADGES: Record<
  PolicyEffect,
  Readonly<{ key: MessageKey; tone?: "warning" }> | null
> = {
  acting: null,
  suppressed: { key: "retention.effectSuppressed", tone: "warning" },
  disabled: { key: "retention.effectDisabled" },
};

export function effectBadge(
  effect: PolicyEffect,
): Readonly<{ key: MessageKey; tone?: "warning" }> | null {
  return EFFECT_BADGES[effect];
}

// A suppressed row needs no sentence of its own: the posture row above it says
// what retain-only mode holds back.
const EFFECT_REASON_KEYS: Record<PolicyEffect, MessageKey | null> = {
  acting: null,
  suppressed: null,
  disabled: "retention.disabledWhy",
};

export function effectReasonKey(effect: PolicyEffect): MessageKey | null {
  return EFFECT_REASON_KEYS[effect];
}

// The tone grows with what the action takes away: archive keeps the record.
const ACTION_TONES: Record<RetentionAction, "warning" | "danger" | undefined> =
  {
    archive: undefined,
    anonymize: "warning",
    erase: "danger",
  };

export function actionTone(
  action: RetentionAction,
): "warning" | "danger" | undefined {
  return ACTION_TONES[action];
}

const DAYS_PER_YEAR = 365;

/** A window as the reader counts it: whole years when it is one, else days. */
export function keepFor(days: number): Readonly<{
  unit: "retention.keepYears" | "retention.keepDays";
  count: number;
}> {
  return days % DAYS_PER_YEAR === 0
    ? { unit: "retention.keepYears", count: days / DAYS_PER_YEAR }
    : { unit: "retention.keepDays", count: days };
}

/**
 * Whether a create failed because the scope is already taken.
 *
 * `POST /retention-policies` has exactly ONE conflict source: the database's
 * `retention_policy_unique` on (workspace, object_type, category), which the
 * store wraps in ErrConflict (retentionpolicystore.go). So `conflict` here is
 * an unambiguous duplicate-scope signal rather than a guess, and the screen
 * can say which row to edit instead of relaying a sentence about a constraint.
 */
export function isDuplicateScope(error: unknown): boolean {
  return problemCodeOf(error) === "conflict";
}

/**
 * A retain-days field's value as the contract wants it, or null when what was
 * typed is not a window.
 *
 * The contract's minimum is 1 and the type is integer, so a blank field, a
 * fraction, a zero and a negative are all the same answer: not a window yet.
 * Refusing them here keeps the Save button honest rather than sending a body
 * the server can only 422.
 */
export function parseRetainDays(input: string): number | null {
  const trimmed = input.trim();
  if (!/^\d+$/.test(trimmed)) {
    return null;
  }
  const days = Number(trimmed);
  return days >= 1 ? days : null;
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// How a logged call ended, in the reader's words. The log form, the timeline,
// the activity drawer and a company's recent list all read it here, so one
// call reads the same everywhere.

import type { components } from "../api/schema";
import type { Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";
import type { CallOutcome } from "./activitybody";

type Activity = components["schemas"]["Activity"];

/** One label per outcome; the compiler refuses a value added on one side only. */
export const CALL_OUTCOME_LABEL: Readonly<Record<CallOutcome, MessageKey>> = {
  reached: "activity.callOutcome.reached",
  not_reached: "activity.callOutcome.not_reached",
  voicemail: "activity.callOutcome.voicemail",
  call_back_later: "activity.callOutcome.call_back_later",
  put_through: "activity.callOutcome.put_through",
};

/**
 * The outcome's label on a call that has one, undefined on anything else, and
 * undefined when the caller has no translator yet.
 */
export function callOutcomeLabel(
  activity: Pick<Activity, "kind" | "call_outcome">,
  t: Translator | undefined,
): string | undefined {
  if (!t || activity.kind !== "call" || !activity.call_outcome) {
    return undefined;
  }
  return t(CALL_OUTCOME_LABEL[activity.call_outcome]);
}

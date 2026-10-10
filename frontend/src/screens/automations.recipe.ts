// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { humanizeToken } from "./audit";

// The catalog's trigger and action vocabularies, in the words a reader uses.
// Both sets are closed on the server (catalog_triggers.go, catalog_actions.go).

const TRIGGER: Readonly<Record<string, MessageKey>> = {
  "clock:no_activity_scan": "auto.trigger.noActivity",
  "clock:renewal_scan": "auto.trigger.renewal",
  "clock:check_in_scan": "auto.trigger.checkIn",
  "deal.stage_changed": "auto.trigger.stageChanged",
  "lead.created": "auto.trigger.leadCreated",
  "activity.captured": "auto.trigger.activityCaptured",
  "list.evaluated": "auto.trigger.listEvaluated",
};

const ACTION: Readonly<Record<string, MessageKey>> = {
  create_task: "auto.action.createTask",
  notify: "auto.action.notify",
  assign_owner: "auto.action.assignOwner",
  set_field: "auto.action.setField",
  draft_email: "auto.action.draftEmail",
  request_approval: "auto.action.requestApproval",
  add_to_shortlist: "auto.action.addToShortlist",
};

// A value newer than this client still reads as words rather than as a key.
function spelledOut(raw: string): string {
  return humanizeToken(raw.replace(/^clock:/, ""));
}

export function triggerLabel(trigger: string, t: Translator): string {
  const key = TRIGGER[trigger];
  return key ? t(key) : spelledOut(trigger);
}

/** What an automation does and when, as one line: "New lead: create a task". */
export function recipeSentence(
  recipe: Readonly<{ trigger: string; action: string }>,
  t: Translator,
): string {
  const action = ACTION[recipe.action];
  return t("auto.recipe", {
    trigger: triggerLabel(recipe.trigger, t),
    action: action ? t(action) : spelledOut(recipe.action),
  });
}

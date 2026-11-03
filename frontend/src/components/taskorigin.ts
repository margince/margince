// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { provenanceOf } from "../design-system/provenanceof";
import type { MessageKey } from "../i18n/en";

type Activity = components["schemas"]["Activity"];

// Why a task nobody typed exists, keyed by the source_system its writer stamps.
// Such a task often has no body. Without this sentence its drawer shows a
// title and a date, and nothing a reader can act on.
//
// A mirror of the backend's native task families
// (storekit.TaskSourceFamilies); backend/gates/frontendtaskorigins_test.go
// fails when the two stop naming the same sources.
export const TASK_ORIGINS: Readonly<Record<string, MessageKey>> = {
  assurance: "tasks.origin.assurance",
  check_in_cadence: "tasks.origin.check_in_cadence",
  accepted_recommendation: "tasks.origin.accepted_recommendation",
  commitment: "tasks.origin.commitment",
  email_request: "tasks.origin.email_request",
  lead_for_contact: "tasks.origin.lead_for_contact",
  lead_qualify: "tasks.origin.lead_qualify",
  lead_replied: "tasks.origin.lead_replied",
  lead_sla: "tasks.origin.lead_sla",
  no_activity_reminder: "tasks.origin.no_activity_reminder",
  "overnight-reconcile": "tasks.origin.overnight_reconcile",
  which_deal: "tasks.origin.which_deal",
};

/**
 * The sentence saying what prepared this task, including an accepted suggestion.
 * Freeform human work has no native origin; unnamed system work still says so.
 */
export function taskOriginKey(task: Activity): MessageKey | null {
  // Own keys only: a caller's source_system such as `constructor` would
  // otherwise read the object's prototype.
  const source = task.source_system;
  if (source && Object.hasOwn(TASK_ORIGINS, source)) {
    return TASK_ORIGINS[source] ?? null;
  }
  return provenanceOf(task.captured_by).kind === "system"
    ? "tasks.origin.system"
    : null;
}

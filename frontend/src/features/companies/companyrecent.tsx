// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import type { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";

type Activity = components["schemas"]["Activity"];
type ActivityKind = Activity["kind"];

const KIND_LABELS: Record<ActivityKind, MessageKey> = {
  email: "co.recent.kind.email",
  call: "co.recent.kind.call",
  meeting: "co.recent.kind.meeting",
  note: "co.recent.kind.note",
  task: "co.recent.kind.task",
  message: "co.recent.kind.message",
};

/**
 * The headline a row shows for an exchange: its subject, or the kind's own
 * word when it was logged without one. A call with no subject is still an
 * event, and a blank line where the headline goes reads as a row that failed
 * to load.
 *
 * The Latest card and the timeline name an exchange with the same words, so
 * the two never call one exchange by two names.
 */
export function activityHeadline(
  activity: Activity,
  t: ReturnType<typeof useT>,
): string {
  return activity.subject?.trim() || t(KIND_LABELS[activity.kind]);
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

export type MomentVerbKind =
  components["schemas"]["ContactMomentAction"]["kind"];

/**
 * Whether pressing each moment verb writes an activity, which a seat without
 * `activity.create` is refused. A declared mirror of the server's
 * `writesAnActivity` in backend/internal/compose/momentaction/withhold.go;
 * backend/gates/frontendwriteverbs_test.go fails when the two differ.
 */
export const MOMENT_VERB_WRITES: Readonly<Record<MomentVerbKind, boolean>> = {
  log_activity: true,
  complete_task: true,
  draft_reply: true,
  schedule_meeting: true,
  open_record: false,
  open_meeting_brief: false,
  open_research: false,
  ask_colleague: false,
};

const knownVerbs = new Map<string, boolean>(Object.entries(MOMENT_VERB_WRITES));

/**
 * A kind this build does not know counts as a write, as in the server's own
 * refusal. So a new verb is not offered live before it is classified.
 */
export function momentVerbWrites(kind: string): boolean {
  return knownVerbs.get(kind) ?? true;
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { provenanceOf } from "../../design-system/provenanceof";
import type { Translator } from "../../i18n";
import { displayedSubject } from "../../lib/systemdisplay";
import { omitted } from "./contact360";

type Contact360 = components["schemas"]["Contact360"];
type ConversationClaim = components["schemas"]["ConversationClaim"];

/** One thing our side owes the contact, by what it says and when it falls due. */
export type OwedPromise = Readonly<{
  key: string;
  title: string;
  due?: string;
}>;

/**
 * What our side owes the contact: each open task a user filed, and each open
 * commitment read from a conversation that no listed task carries. It mirrors
 * the set the server's promise moment ranks (owedPromises in the contact360
 * compose package). The headline and the rows under it thus count one set.
 * Null when either section is withheld from this reader.
 */
export function owedPromises(
  view: Contact360,
  t: Translator,
): OwedPromise[] | null {
  if (omitted(view, "claims") || omitted(view, "next_steps")) {
    return null;
  }
  const tasks = (view.next_steps?.data ?? [])
    .filter(
      (task) =>
        !task.is_done && provenanceOf(task.captured_by).kind !== "system",
    )
    .map((task) => ({
      key: task.id,
      title: displayedSubject(task) ?? t("task.untitled"),
      due: task.due_at ?? undefined,
    }));
  const listed = listedTaskIds(view);
  const claims = (view.claims ?? [])
    .filter(
      (claim) =>
        claim.kind === "commitment_ours" &&
        claim.status === "open" &&
        !claim.needs_review &&
        claimStandsAlone(claim, listed),
    )
    .map((claim) => ({
      key: claim.id,
      title: claim.body,
      due: claim.due_at ?? undefined,
    }));
  return [...tasks, ...claims];
}

/** The ids of the tasks the page lists in `next_steps`. */
export function listedTaskIds(view: Contact360): ReadonlySet<string> {
  return new Set((view.next_steps?.data ?? []).map((task) => task.id));
}

/**
 * Whether a claim is owed in its own name: it names no task, or its task is
 * open (`task_open`) but past the `next_steps` cap. A listed task stands for
 * its claim. A task that is done or hidden from this reader leaves nothing
 * owed. claimStandsAlone in the contact360 compose package holds the same rule.
 */
export function claimStandsAlone(
  claim: ConversationClaim,
  listed: ReadonlySet<string>,
): boolean {
  if (!claim.task_activity_id) {
    return true;
  }
  return !listed.has(claim.task_activity_id) && claim.task_open === true;
}

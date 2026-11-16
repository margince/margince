// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { provenanceOf } from "../../design-system/provenanceof";
import type { Translator } from "../../i18n";
import { displayedSubject } from "../../lib/systemdisplay";
import { omitted } from "./contact360";

type Contact360 = components["schemas"]["Contact360"];

/** One thing our side owes the contact, by what it says and when it falls due. */
export type OwedPromise = Readonly<{
  key: string;
  title: string;
  due?: string;
}>;

/**
 * What our side owes the contact: each open task a user filed, and each open
 * commitment read from a conversation that no task carries. It mirrors the
 * set the server's promise moment ranks (owedPromises in the contact360
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
  const claims = (view.claims ?? [])
    .filter(
      (claim) =>
        claim.kind === "commitment_ours" &&
        claim.status === "open" &&
        !claim.needs_review &&
        !claim.task_activity_id,
    )
    .map((claim) => ({
      key: claim.id,
      title: claim.body,
      due: claim.due_at ?? undefined,
    }));
  return [...tasks, ...claims];
}

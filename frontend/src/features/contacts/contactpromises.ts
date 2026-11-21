// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { provenanceOf } from "../../design-system/provenanceof";
import type { Translator } from "../../i18n";
import { dueOnDay } from "../../lib/standingpeeks";
import { displayedSubject } from "../../lib/systemdisplay";
import { omitted } from "./contact360";

type Contact360 = components["schemas"]["Contact360"];
type ConversationClaim = components["schemas"]["ConversationClaim"];
type Contact360Task = NonNullable<Contact360["next_steps"]>["data"][number];

/** One thing our side owes the contact, by what it says and when it falls due. */
export type OwedPromise = Readonly<{
  key: string;
  title: string;
  due?: string;
  /** The zone the due day reads in: the company's for a task with only a date. */
  zone?: string;
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
  words?: Readonly<{ zone: string; companyZone?: string }>,
): OwedPromise[] | null {
  if (omitted(view, "claims") || omitted(view, "next_steps")) {
    return null;
  }
  const tasks = (view.next_steps?.data ?? [])
    .filter(
      (task) =>
        !task.is_done && provenanceOf(task.captured_by).kind !== "system",
    )
    .map((task) => {
      const due =
        words && task.due_at
          ? dueOnDay(words, task.due_at, task.due_has_time, task.due_intent)
          : undefined;
      return {
        key: task.id,
        title: displayedSubject(task) ?? t("task.untitled"),
        due: due?.at ?? task.due_at ?? undefined,
        zone: due?.zone,
      };
    });
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

/**
 * The last task `next_steps` lists when colleague-filed work may run past its
 * cap, or undefined when the list is whole. The server sorts colleague-filed
 * tasks ahead of reminders. So a capped page whose last row is a reminder
 * holds every colleague-filed task, and a count of them is exact.
 */
export function lastTaskBeforeCap(
  view: Contact360,
): Contact360Task | undefined {
  const tasks = view.next_steps?.data ?? [];
  const last = tasks.at(-1);
  if (
    view.next_steps?.page.has_more !== true ||
    !last ||
    provenanceOf(last.captured_by).kind === "system"
  ) {
    return undefined;
  }
  return last;
}

/** The ids of the tasks the page lists in `next_steps`. */
export function listedTaskIds(view: Contact360): ReadonlySet<string> {
  return new Set((view.next_steps?.data ?? []).map((task) => task.id));
}

/**
 * Whether a claim is owed in its own name: it names no task, or its task is
 * open (`task_open`) but past the `next_steps` cap. A listed task stands for
 * its claim. A task that is done or hidden from this reader leaves nothing
 * owed. claimStandsAlone in the contact360 compose package holds the same rule,
 * and one case table there (testdata/claimstandsalone.json) runs both.
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

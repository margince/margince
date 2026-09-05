// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

type Person360 = components["schemas"]["Person360"];

// What this record owes, from BOTH places a promise is written down.
//
// The server's own rule, in one place the screens can share:
// compose/person360.owedPromises unions the open next-step TASKS with the open
// `commitment_ours` CLAIMS, and the headline that says "You owe them" is
// computed from exactly that union. A card counting one half contradicts the
// headline above it — which is what "You owe them" over "0 · nothing owed" was.
//
// It is a UNION, NOT A JOIN, for the reason the server gives: nothing writes
// conversation_claim.task_activity_id, so an extracted commitment and a task
// filed for the same thing are two unlinked rows. A reader who did both may
// see two. Guessing which pairs mean one promise is the alternative, and it
// would be a guess.

/** One thing owed, whichever side it was written down on. */
export type OwedPromise = {
  readonly key: string;
  /** When it is due, or null where nobody said. */
  readonly dueAt: string | null;
  /** What was promised, in the words the record holds. */
  readonly body: string;
  /** Where it was written down, for a card that shows its working. */
  readonly note?: string;
};

/**
 * The open promises this record owes.
 *
 * A task counts only while it is NOT done — a completed promise is not owed —
 * and a claim only while it is `commitment_ours` and still open. The other
 * claim kinds are deliberately absent: `commitment_theirs` is what the other
 * side owes US, and an `open_question` is a question nobody promised anything
 * about. Counting either would tell a rep they owe work that is not theirs.
 */
export function owedPromises(view: Person360): readonly OwedPromise[] {
  const tasks = (view.next_steps?.data ?? [])
    .filter((task) => task.is_done !== true)
    .map((task) => ({
      key: task.id,
      dueAt: task.due_at ?? null,
      // A task can arrive without a subject — one filed without one, and one
      // whose content this reader may not see, which the server nulls. Both
      // are still owed.
      body: task.subject ?? "",
    }));
  const claims = (view.claims ?? [])
    .filter(
      (claim) => claim.kind === "commitment_ours" && claim.status === "open",
    )
    .map((claim) => ({
      key: claim.id,
      dueAt: claim.due_at ?? null,
      body: claim.body,
      note: claim.source_label ?? undefined,
    }));
  return [...tasks, ...claims];
}

/**
 * Whether the count above is a floor rather than a total.
 *
 * `next_steps` is a page. A card reporting its length as "3 promises" when the
 * server holds more states a number it did not measure, which is the one thing
 * a count must not do.
 */
export function owedPromisesTruncated(view: Person360): boolean {
  return view.next_steps?.page.has_more === true;
}

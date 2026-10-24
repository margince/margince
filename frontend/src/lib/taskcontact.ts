// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import type { components } from "../api/schema";
import { useActivity } from "./activityread";
import { contactQuery } from "./contact.queries";

type Activity = components["schemas"]["Activity"];
type Contact = components["schemas"]["Contact"];

/** The contact a task is about, and whether the task was read out of a call. */
export type TaskContact = Readonly<{
  contact: Contact | undefined;
  fromCall: boolean;
}>;

// The page's contact when the activity links it, else its one contact link.
// Links come sorted by id, so of several none is "the" contact.
export function linkedContactId(
  activity: Pick<Activity, "links"> | undefined,
  pageContactId?: string,
): string | undefined {
  const linked = new Set(
    (activity?.links ?? [])
      .filter((link) => link.entity_type === "contact")
      .map((link) => link.entity_id),
  );
  if (pageContactId !== undefined) {
    return linked.has(pageContactId) ? pageContactId : undefined;
  }
  return linked.size === 1 ? [...linked][0] : undefined;
}

export function useTaskContact(
  task: Activity | undefined,
  pageContact?: Contact,
): TaskContact {
  const contactId = linkedContactId(task, pageContact?.id);
  const read = useQuery(contactQuery(pageContact ? undefined : contactId));
  // The source under `SourceEvidence`'s key, so the dialog asks for it once.
  const source = useActivity(
    task?.source_activity_id ?? undefined,
    contactId !== undefined,
  ).data;
  const fromCall =
    source?.kind === "call" && source.content_state !== "withheld";
  if (contactId === undefined) {
    return { contact: undefined, fromCall: false };
  }
  // A failed current read hides the cached answer, for useActivity's reason.
  const contact = pageContact ?? (read.isError ? undefined : read.data);
  return { contact, fromCall };
}

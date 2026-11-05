// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import type { components } from "../api/schema";
import { useActivity } from "./activityread";
import { contactQuery } from "./contact.queries";

type Activity = components["schemas"]["Activity"];
type Contact = components["schemas"]["Contact"];

/** The contact a task is about, and whether the task was read out of a call.
 *  `pending` is a contact still being read, so a caller can hold its place. */
export type TaskContact = Readonly<{
  contact: Contact | undefined;
  fromCall: boolean;
  pending: boolean;
}>;

// The page's contact when the task links it, else the contact the server named
// as the task's email target (`email_target`).
function taskContactId(
  task: Activity | undefined,
  pageContact?: Contact,
): string | undefined {
  if (pageContact) {
    const linked = (task?.links ?? []).some(
      (link) =>
        link.entity_type === "contact" && link.entity_id === pageContact.id,
    );
    return linked ? pageContact.id : undefined;
  }
  const target = task?.email_target;
  return target?.entity_type === "contact" ? target.entity_id : undefined;
}

export function useTaskContact(
  task: Activity | undefined,
  pageContact?: Contact,
): TaskContact {
  const contactId = taskContactId(task, pageContact);
  const read = useQuery(contactQuery(pageContact ? undefined : contactId));
  // The source under `SourceEvidence`'s key, so the dialog asks for it once.
  const source = useActivity(
    task?.source_activity_id ?? undefined,
    contactId !== undefined,
  ).data;
  const fromCall =
    source?.kind === "call" && source.content_state !== "withheld";
  if (contactId === undefined) {
    return { contact: undefined, fromCall: false, pending: false };
  }
  // A failed current read hides the cached answer, for useActivity's reason.
  const contact = pageContact ?? (read.isError ? undefined : read.data);
  return { contact, fromCall, pending: !pageContact && read.isPending };
}

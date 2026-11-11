// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import type { components } from "../api/schema";
import { contactQuery } from "./contact.queries";

type Activity = components["schemas"]["Activity"];
type Contact = components["schemas"]["Contact"];

/** The contact a task is about.
 *  `pending` is a contact still being read, so a caller can hold its place. */
export type TaskContact = Readonly<{
  contact: Contact | undefined;
  pending: boolean;
}>;

// The page's contact when the task links it, else the contact the server named
// as the task's email target (`email_target`). Only a mail task has a target, so
// a call or a meeting is owed to the first contact it is filed on.
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
  if (target) {
    return target.entity_type === "contact" ? target.entity_id : undefined;
  }
  return task?.links?.find((link) => link.entity_type === "contact")?.entity_id;
}

export function useTaskContact(
  task: Activity | undefined,
  pageContact?: Contact,
): TaskContact {
  const contactId = taskContactId(task, pageContact);
  const read = useQuery(contactQuery(pageContact ? undefined : contactId));
  if (contactId === undefined) {
    return { contact: undefined, pending: false };
  }
  // A failed read must not expose a cached contact after access was revoked.
  const contact = pageContact ?? (read.isError ? undefined : read.data);
  return { contact, pending: !pageContact && read.isPending };
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import type { components } from "../api/schema";
import { useActivity } from "./activityread";
import { contactQuery } from "./contact.queries";
import { leadQuery } from "./lead.queries";

type Activity = components["schemas"]["Activity"];
type Contact = components["schemas"]["Contact"];

/** The contact a task is about, and whether the task was read out of a call.
 *  `pending` is a contact still being read, so a caller can hold its place. */
export type TaskContact = Readonly<{
  contact: Contact | undefined;
  fromCall: boolean;
  pending: boolean;
}>;

// The page's contact when the task links it, else its one contact link. Links
// come sorted by id, so of several none is "the" contact.
function taskContactId(
  task: Activity | undefined,
  pageContact?: Contact,
): string | undefined {
  const linked = new Set(
    (task?.links ?? [])
      .filter((link) => link.entity_type === "contact")
      .map((link) => link.entity_id),
  );
  if (pageContact) {
    return linked.has(pageContact.id) ? pageContact.id : undefined;
  }
  return linked.size === 1 ? [...linked][0] : undefined;
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

type Lead = components["schemas"]["Lead"];

/** Where a task with no single contact drafts its email: a live lead, else
 *  the company. `lead` is the lead read, for the address and the refusal. */
export type TaskDraftTarget = Readonly<
  | { entityType: "lead"; entityId: string; lead: Lead }
  | { entityType: "company"; entityId: string }
  | { entityType: "closedLead" }
>;

function onlyLink(task: Activity, entityType: string): string | undefined {
  const linked = new Set(
    (task.links ?? [])
      .filter((link) => link.entity_type === entityType)
      .map((link) => link.entity_id),
  );
  return linked.size === 1 ? [...linked][0] : undefined;
}

/**
 * The record a task drafts to when it is linked to no single contact: its one
 * live lead, else its one company. A closed lead with no company is a target
 * that refuses, so the reader is told why. `enabled` is false while a contact
 * answers for the task. The title is never read: every task offers the verb.
 */
export function useTaskDraftTarget(
  task: Activity,
  enabled: boolean,
): Readonly<{ target: TaskDraftTarget | undefined; pending: boolean }> {
  const leadId = enabled ? onlyLink(task, "lead") : undefined;
  const companyId = enabled ? onlyLink(task, "company") : undefined;
  const read = useQuery(leadQuery(leadId));
  if (!enabled) return { target: undefined, pending: false };
  if (leadId !== undefined && read.isPending) {
    return { target: undefined, pending: true };
  }
  const lead = read.isError ? undefined : read.data;
  if (lead && !lead.archived_at) {
    return {
      target: { entityType: "lead", entityId: lead.id, lead },
      pending: false,
    };
  }
  if (companyId !== undefined) {
    return {
      target: { entityType: "company", entityId: companyId },
      pending: false,
    };
  }
  return {
    target: lead ? { entityType: "closedLead" } : undefined,
    pending: false,
  };
}

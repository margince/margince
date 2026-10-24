// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Mail } from "lucide-react";
import { useId } from "react";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { CallAction, linkVerb } from "../design-system/callaction";
import { ContactLink } from "../design-system/contactlink";
import { mailbox, primaryPhone } from "../format/contacturi";
import { type Translator, useT } from "../i18n";
import { ENTITY } from "../lib/entity";
import { routeHash } from "../lib/router";
import { useTaskContact } from "../lib/taskcontact";
import "./tasknextsteps.css";

// What a rep does next about the contact a task is owed to. Nothing here
// writes the task, so a done or read-only task still offers it.

type Activity = components["schemas"]["Activity"];
type Contact = components["schemas"]["Contact"];

/** Call, for a task read out of a call to a contact with a number to dial. */
export function TaskCallAction({
  task,
  pageContact,
}: Readonly<{ task: Activity; pageContact?: Contact }>) {
  const { contact, fromCall } = useTaskContact(task, pageContact);
  return <CallVerb contact={contact} fromCall={fromCall} />;
}

/**
 * Call, Email and Open contact. `className` wraps them in a row of their own,
 * which is left out when there is no contact to act on.
 */
export function TaskNextSteps({
  task,
  offerOpen = true,
  className,
}: Readonly<{ task: Activity; offerOpen?: boolean; className?: string }>) {
  const t = useT();
  const reasonId = useId();
  const { contact, fromCall } = useTaskContact(task);
  if (!contact) {
    return null;
  }
  const refusal = emailRefusal(contact, t);
  const steps = (
    <>
      <CallVerb contact={contact} fromCall={fromCall} />
      <EmailVerb
        contact={contact}
        reasonId={refusal === undefined ? undefined : reasonId}
      />
      {offerOpen &&
        linkVerb(
          routeHash(ENTITY.contact.route(contact.id)),
          null,
          t("task.next.openContact"),
        )}
      {/* Under every verb rather than inside Email's column, so the verbs keep
          one line. */}
      {refusal !== undefined && (
        <p id={reasonId} className="t-caption task-next-reason">
          {refusal}
        </p>
      )}
    </>
  );
  return className ? <div className={className}>{steps}</div> : steps;
}

/** Why Email is refused. `mailbox` is the test ContactLink applies. */
function emailRefusal(contact: Contact, t: Translator): string | undefined {
  if (contact.archived_at) return t("contact.rail.archivedReadOnly");
  if (!mailbox(contact.primary_email ?? "")) return t("task.next.noAddress");
  return undefined;
}

function CallVerb({
  contact,
  fromCall,
}: Readonly<{ contact: Contact | undefined; fromCall: boolean }>) {
  const t = useT();
  if (!contact || !fromCall || contact.archived_at) {
    return null;
  }
  return (
    <CallAction
      appearance="button"
      label={t("record.verb.call")}
      number={primaryPhone(contact.phones)?.phone}
    />
  );
}

function EmailVerb({
  contact,
  reasonId,
}: Readonly<{ contact: Contact; reasonId: string | undefined }>) {
  const t = useT();
  if (reasonId !== undefined) {
    return (
      <Button variant="ghost" reasonId={reasonId}>
        <Mail aria-hidden="true" />
        {t("record.verb.email")}
      </Button>
    );
  }
  return (
    <ContactLink
      kind="email"
      appearance="button"
      label={t("record.verb.email")}
      address={contact.primary_email ?? undefined}
      record={{ entityType: "contact", entityId: contact.id }}
    />
  );
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Sparkles } from "lucide-react";
import { useId } from "react";
import type { components } from "../api/schema";
import { Button, Skeleton } from "../design-system/atoms";
import { CallAction, linkVerb } from "../design-system/callaction";
import { ContactLink } from "../design-system/contactlink";
import { mailbox, primaryPhone } from "../format/contacturi";
import { type Translator, useT } from "../i18n";
import { intentAbout } from "../lib/compose.intent";
import { ENTITY } from "../lib/entity";
import { routeHash } from "../lib/router";
import { useTaskContact } from "../lib/taskcontact";
import "./tasknextsteps.css";

// What a rep does next about the contact a task is owed to. Nothing here
// writes the task, so a done or read-only task still offers it.

type Activity = components["schemas"]["Activity"];
type Contact = components["schemas"]["Contact"];

/** The room the next steps take while the reads behind them are out, so the
 *  verbs arrive without pushing the row they sit in. One Email verb wide. */
export function TaskNextStepsPending() {
  return <Skeleton width="6rem" height="var(--controlHeight)" />;
}

/** Call, for a task read out of a call to a contact with a number to dial. */
export function TaskCallAction({
  task,
  pageContact,
}: Readonly<{ task: Activity; pageContact?: Contact }>) {
  const { contact, fromCall } = useTaskContact(task, pageContact);
  return <CallVerb contact={contact} fromCall={fromCall} />;
}

/**
 * Call, Draft email and Open contact. A task owed to a contact is work for that
 * contact, so mail to them is drafted from the task rather than offered blank. `className` wraps them in a row of their own,
 * which is left out when there is no contact to act on.
 */
export function TaskNextSteps({
  task,
  offerOpen = true,
  className,
}: Readonly<{ task: Activity; offerOpen?: boolean; className?: string }>) {
  const t = useT();
  const reasonId = useId();
  const { contact, fromCall, pending } = useTaskContact(task);
  if (!contact) {
    return pending ? <TaskNextStepsPending /> : null;
  }
  const refusal = emailRefusal(contact, t);
  const steps = (
    <>
      <CallVerb contact={contact} fromCall={fromCall} />
      <EmailVerb
        contact={contact}
        intent={intentAbout(t("task.next.draftIntent"), task.subject)}
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
      record={{ entityType: "contact", entityId: contact.id }}
    />
  );
}

function EmailVerb({
  contact,
  intent,
  reasonId,
}: Readonly<{
  contact: Contact;
  intent: string;
  reasonId: string | undefined;
}>) {
  const t = useT();
  if (reasonId !== undefined) {
    return (
      <Button variant="ai" reasonId={reasonId}>
        <Sparkles aria-hidden="true" />
        {t("task.next.draftEmail")}
      </Button>
    );
  }
  return (
    <ContactLink
      kind="email"
      appearance="button"
      label={t("record.verb.email")}
      draft={{ label: t("task.next.draftEmail"), intent }}
      address={contact.primary_email ?? undefined}
      record={{ entityType: "contact", entityId: contact.id }}
    />
  );
}

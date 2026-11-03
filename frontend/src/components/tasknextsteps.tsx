// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Sparkles } from "lucide-react";
import { type ReactNode, useId } from "react";
import type { components } from "../api/schema";
import { Button, Skeleton } from "../design-system/atoms";
import { CallAction, linkVerb } from "../design-system/callaction";
import { ContactLink } from "../design-system/contactlink";
import { primaryPhone } from "../format/contacturi";
import { type Translator, useT } from "../i18n";
import { intentAbout } from "../lib/compose.intent";
import { ENTITY } from "../lib/entity";
import { routeHash } from "../lib/router";
import { useTaskContact } from "../lib/taskcontact";
import { useWriteTo } from "../lib/writeto";
import "./tasknextsteps.css";

// What a rep does next about the contact a task is owed to. Nothing here
// writes the task, so a done or read-only task still offers it.

type Activity = components["schemas"]["Activity"];
type Contact = components["schemas"]["Contact"];
type EmailTarget = components["schemas"]["ActivityEmailTarget"];

/** The room the next steps take while the reads behind them are out, so the
 *  verbs arrive without pushing the row they sit in. One Email verb wide. */
export function TaskNextStepsPending() {
  return (
    <div className="task-next-pending">
      <Skeleton width="6rem" height="var(--controlHeight)" />
    </div>
  );
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
 * contact, so mail to them is drafted from the task rather than offered blank.
 * Who the mail goes to is the server's answer (`email_target`): one contact,
 * else one live lead, else one company. `className` wraps the verbs in a row
 * of their own, which is left out when there is nothing to act on.
 */
export function TaskNextSteps({
  task,
  offerOpen = true,
  className,
}: Readonly<{ task: Activity; offerOpen?: boolean; className?: string }>) {
  const t = useT();
  const { contact, fromCall, pending } = useTaskContact(task);
  const target = task.email_target;
  const intent = intentAbout(t("task.next.draftIntent"), task.subject);
  let steps: ReactNode;
  if (pending) {
    // In the row the verbs will stand in, so they land where it stood.
    steps = <TaskNextStepsPending />;
  } else if (target?.entity_type === "contact") {
    // A contact the reader can no longer read takes its verbs with it.
    if (!contact) return null;
    steps = (
      <ContactSteps
        contact={contact}
        fromCall={fromCall}
        intent={intent}
        offerOpen={offerOpen}
        refusal={refusalOf(target, t)}
      />
    );
  } else if (target) {
    steps = <RecordDraftStep target={target} intent={intent} />;
  } else {
    return null;
  }
  return className ? <div className={className}>{steps}</div> : steps;
}

function ContactSteps({
  contact,
  fromCall,
  intent,
  offerOpen,
  refusal,
}: Readonly<{
  contact: Contact;
  fromCall: boolean;
  intent: string;
  offerOpen: boolean;
  refusal: string | undefined;
}>) {
  const t = useT();
  const reasonId = useId();
  return (
    <>
      <CallVerb contact={contact} fromCall={fromCall} />
      <EmailVerb
        contact={contact}
        intent={intent}
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
}

/**
 * Draft email to a lead or a company. The lead's draft fills To with the lead's
 * address. The company's composer asks which of its contacts the mail is for.
 * Neither has an address to hand a mail client, so with no composer to open
 * there is no verb.
 */
function RecordDraftStep({
  target,
  intent,
}: Readonly<{ target: EmailTarget; intent: string }>) {
  const t = useT();
  const reasonId = useId();
  const writeTo = useWriteTo();
  const refusal = refusalOf(target, t);
  if (refusal !== undefined) {
    return (
      <>
        <Button variant="ai" reasonId={reasonId}>
          <Sparkles aria-hidden="true" />
          {t("task.next.draftEmail")}
        </Button>
        <p id={reasonId} className="t-caption task-next-reason">
          {refusal}
        </p>
      </>
    );
  }
  if (!writeTo || target.entity_type === "contact") return null;
  return (
    <Button
      variant="ai"
      onClick={() =>
        writeTo({
          entityType: target.entity_type,
          entityId: target.entity_id,
          draftIntent: intent,
        })
      }
    >
      <Sparkles aria-hidden="true" />
      {t("task.next.draftEmail")}
    </Button>
  );
}

/** Why the server refuses the email, in the reader's words. */
function refusalOf(target: EmailTarget, t: Translator): string | undefined {
  switch (target.refusal) {
    case "archived":
      return t("contact.rail.archivedReadOnly");
    case "lead_closed":
      return t("lead.terminalReadOnly");
    case "no_address":
      return target.entity_type === "lead"
        ? t("task.next.noLeadAddress")
        : t("task.next.noAddress");
    default:
      return undefined;
  }
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

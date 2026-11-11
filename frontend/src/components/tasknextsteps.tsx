// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { CalendarPlus, ClipboardCheck } from "lucide-react";
import { type ReactNode, useId } from "react";
import type { components } from "../api/schema";
import { Button, Skeleton } from "../design-system/atoms";
import { CallAction, linkVerb } from "../design-system/callaction";
import { CopyAction } from "../design-system/clipboardcopy";
import { ContactLink } from "../design-system/contactlink";
import { primaryPhone } from "../format/contacturi";
import { type Translator, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { intentAbout } from "../lib/compose.intent";
import { ENTITY } from "../lib/entity";
import { valueFor } from "../lib/labelfor";
import { navigate, type Route, routeHash } from "../lib/router";
import { useTaskContact } from "../lib/taskcontact";
import { taskTypeOf } from "../lib/tasktype";
import { useWriteTo } from "../lib/writeto";
import { mostSpecificFirst } from "./recordlinks";
import "./tasknextsteps.css";
import { asksWhichDeal, WhichDealChoice } from "./whichdealchoice";

// The one step a task's type asks for. Nothing here writes the task, so a
// done or read-only task still offers it.

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

/** Call the task's contact when they have a number to dial. */
export function TaskCallAction({
  task,
  pageContact,
}: Readonly<{ task: Activity; pageContact?: Contact }>) {
  const { contact } = useTaskContact(task, pageContact);
  return <CallVerb contact={contact} />;
}

/**
 * The step the task's type asks for: a mail drafted for a reply or a mail to
 * write, the number and Call for a call, Book a meeting for a meeting, the
 * record to review, the choice to decide. A task to do has none: Done is it.
 * `className` wraps the step in a row of its own, which is left out when
 * there is nothing to act on.
 */
export function TaskNextSteps({
  task,
  offerOpen = true,
  className,
}: Readonly<{ task: Activity; offerOpen?: boolean; className?: string }>) {
  const { steps } = useTaskNextSteps(task, offerOpen);
  if (steps === null) return null;
  return className ? <div className={className}>{steps}</div> : steps;
}

/** The type's step, and whether it is a filled button, so Done can step back
 *  and the card keeps one filled verb. */
export function useTaskNextSteps(
  task: Activity | undefined,
  offerOpen: boolean,
): Readonly<{ steps: ReactNode; fills: boolean }> {
  const { contact, pending } = useTaskContact(task);
  const writeTo = useWriteTo();
  if (!task) return { steps: null, fills: false };
  const reachable = contact && !contact.archived_at ? contact : undefined;
  switch (taskTypeOf(task)) {
    case "reply":
    case "write": {
      const target = task.email_target;
      return {
        steps: <MailSteps task={task} offerOpen={offerOpen} />,
        fills: Boolean(target && (writeTo || target.refusal)),
      };
    }
    case "call":
      if (pending) return { steps: <TaskNextStepsPending />, fills: true };
      return reachable && primaryPhone(reachable.phones)
        ? { steps: <CallSteps contact={reachable} />, fills: true }
        : { steps: null, fills: false };
    case "meet":
      return reachable
        ? { steps: <MeetStep contactId={reachable.id} />, fills: true }
        : { steps: null, fills: false };
    case "review": {
      const record = reviewRecord(task);
      return record
        ? { steps: <ReviewStep record={record} />, fills: true }
        : { steps: null, fills: false };
    }
    case "decide":
      return {
        steps: <WhichDealChoice task={task} />,
        fills: asksWhichDeal(task),
      };
    default:
      return { steps: null, fills: false };
  }
}

/**
 * Draft email. A task owed to a contact is work for that contact, so mail to
 * them is drafted from the task rather than offered blank. Who the mail goes
 * to is the server's answer (`email_target`): a contact, a live lead or a
 * company.
 */
function MailSteps({
  task,
  offerOpen,
}: Readonly<{ task: Activity; offerOpen: boolean }>) {
  const t = useT();
  const { contact, pending } = useTaskContact(task);
  const target = task.email_target;
  const intent = intentAbout(t("task.next.draftIntent"), task.subject);
  if (pending) {
    // In the row the verbs will stand in, so they land where it stood.
    return <TaskNextStepsPending />;
  }
  if (target?.entity_type === "contact") {
    // A contact the reader can no longer read takes its verbs with it.
    if (!contact) return null;
    return (
      <ContactSteps
        contact={contact}
        intent={intent}
        offerOpen={offerOpen}
        refusal={refusalOf(target, t)}
      />
    );
  }
  return target ? <RecordDraftStep target={target} intent={intent} /> : null;
}

/** The number to read or copy, for a reader at a desk, and Call to dial it. */
function CallSteps({ contact }: Readonly<{ contact: Contact }>) {
  const t = useT();
  const number = primaryPhone(contact.phones)?.phone;
  if (!number || contact.archived_at) return null;
  const record = { entityType: "contact", entityId: contact.id } as const;
  return (
    <>
      <span className="task-next-number">
        <CallAction appearance="value" number={number} record={record} />
        <CopyAction
          text={number}
          labels={{
            copy: t("task.next.copyNumber"),
            copied: t("task.next.numberCopied"),
            remedy: t("task.next.copyNumberRemedy"),
          }}
        />
      </span>
      <CallAction
        appearance="button"
        filled
        label={t("record.verb.call")}
        number={number}
        record={record}
      />
    </>
  );
}

/** The booking flow, opened on the contact the task is owed to. */
function MeetStep({ contactId }: Readonly<{ contactId: string }>) {
  const t = useT();
  return (
    <Button
      variant="primary"
      onClick={() => navigate({ screen: "book", id: `contact-${contactId}` })}
    >
      <CalendarPlus aria-hidden="true" />
      {t("scheduling.bookContact")}
    </Button>
  );
}

const REVIEW_VERBS: Readonly<
  Record<components["schemas"]["ActivityLink"]["entity_type"], MessageKey>
> = {
  lead: "task.next.reviewLead",
  deal: "task.next.reviewDeal",
  project: "task.next.reviewProject",
  contact: "task.next.reviewContact",
  company: "task.next.reviewCompany",
};

type ReviewRecord = Readonly<{ verb: MessageKey; route: Route }>;

/** The most specific record the task is filed on, the one to check. */
function reviewRecord(task: Activity): ReviewRecord | undefined {
  const record = mostSpecificFirst(task.links ?? [])[0];
  if (!record) return undefined;
  const verb = valueFor(REVIEW_VERBS, record.entity_type, undefined);
  const entity = valueFor(ENTITY, record.entity_type, undefined);
  return verb && entity
    ? { verb, route: entity.route(record.entity_id) }
    : undefined;
}

function ReviewStep({ record }: Readonly<{ record: ReviewRecord }>) {
  const t = useT();
  return (
    <Button variant="primary" onClick={() => navigate(record.route)}>
      <ClipboardCheck aria-hidden="true" />
      {t(record.verb)}
    </Button>
  );
}

function ContactSteps({
  contact,
  intent,
  offerOpen,
  refusal,
}: Readonly<{
  contact: Contact;
  intent: string;
  offerOpen: boolean;
  refusal: string | undefined;
}>) {
  const t = useT();
  const reasonId = useId();
  return (
    <>
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
function CallVerb({ contact }: Readonly<{ contact: Contact | undefined }>) {
  const t = useT();
  if (!contact || contact.archived_at) {
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

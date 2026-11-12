// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { CalendarPlus, ClipboardCheck } from "lucide-react";
import { type ReactNode, useId } from "react";
import type { components } from "../api/schema";
import { Skeleton } from "../design-system/atoms";
import { CallAction, linkVerb } from "../design-system/callaction";
import { CopyAction } from "../design-system/clipboardcopy";
import { primaryPhone } from "../format/contacturi";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { intentAbout } from "../lib/compose.intent";
import { ENTITY } from "../lib/entity";
import { valueFor } from "../lib/labelfor";
import { type Route, routeHash } from "../lib/router";
import { useTaskContact } from "../lib/taskcontact";
import { taskTypeOf } from "../lib/tasktype";
import { useWriteTo } from "../lib/writeto";
import { mostSpecificFirst } from "./recordlinks";
import "./tasknextsteps.css";
import {
  CallVerb,
  EmailVerb,
  RecordDraftStep,
  refusalOf,
} from "./tasknextsteps.verbs";
import { asksWhichDeal, WhichDealChoice } from "./whichdealchoice";

// The one step a task's type asks for. Nothing here writes the task, so a
// done or read-only task still offers it.

type Activity = components["schemas"]["Activity"];
type Contact = components["schemas"]["Contact"];

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
 * The step the task's type asks for. A reply answers the way its message came
 * in: a mail drafts a mail, a call shows the number and Call. Reach out offers
 * both, and a meeting offers Book a meeting.
 * A review opens the record and a decision shows the choice. A task to do has
 * none: Done is it.
 * `className` wraps the step in a row of its own, which is left out when
 * there is nothing to act on.
 */
export function TaskNextSteps({
  task,
  offerOpen = true,
  className,
}: Readonly<{ task: Activity; offerOpen?: boolean; className?: string }>) {
  // With no menu here, the other ways stand after the type's step.
  const { steps, others } = useTaskNextSteps(task, offerOpen);
  if (steps === null && !others) return null;
  const all = (
    <>
      {steps}
      {others}
    </>
  );
  return className ? <div className={className}>{all}</div> : all;
}

/** The type's step, and whether it is a filled button, so Done can step back
 *  and the card keeps one filled verb. */
export function useTaskNextSteps(
  task: Activity | undefined,
  offerOpen: boolean,
): Steps {
  const { contact, pending } = useTaskContact(task);
  const writeTo = useWriteTo();
  if (!task) return NO_STEP;
  const ways = taskWays(task, contact, pending, offerOpen, writeTo !== null);
  const shown = shownWays(task);
  const target = task.email_target;
  const mail = !shown.includes("mail") && Boolean(target && !target.refusal);
  const call = !shown.includes("call") && ways.phone !== undefined;
  return {
    ...typedSteps(ways),
    others:
      mail || call ? (
        <OtherWays
          task={task}
          phone={call ? ways.phone : undefined}
          mail={mail}
        />
      ) : null,
  };
}

function taskWays(
  task: Activity,
  contact: Contact | undefined,
  pending: boolean,
  offerOpen: boolean,
  composes: boolean,
): Ways {
  const reachable = contact && !contact.archived_at ? contact : undefined;
  const target = task.email_target;
  return {
    task,
    offerOpen,
    pending,
    reachable,
    mailFills: Boolean(target && (composes || target.refusal)),
    mailWorks: Boolean(target && composes && !target.refusal),
    phone: reachable && primaryPhone(reachable.phones) ? reachable : undefined,
  };
}

type Way = "mail" | "call";

/** The ways a task's own step already offers, which its menu leaves out. */
function shownWays(task: Activity): readonly Way[] {
  switch (taskTypeOf(task)) {
    case "reply":
      if (task.reply_channel === "email") return ["mail"];
      return task.reply_channel === "call" ? ["call"] : ["mail", "call"];
    case "reach_out":
      return ["mail", "call"];
    default:
      return [];
  }
}

function typedSteps(ways: Ways): Omit<Steps, "others"> {
  const { task, reachable } = ways;
  switch (taskTypeOf(task)) {
    case "reply":
      return replySteps(ways);
    case "reach_out":
      return reachOutSteps(ways);
    case "meet":
      return reachable
        ? { steps: <MeetStep contactId={reachable.id} />, fills: true }
        : NO_STEP;
    case "review": {
      const record = reviewRecord(task);
      return record
        ? { steps: <ReviewStep record={record} />, fills: true }
        : NO_STEP;
    }
    case "decide":
      return {
        steps: <WhichDealChoice task={task} />,
        fills: asksWhichDeal(task),
      };
    default:
      return NO_STEP;
  }
}

type Steps = Readonly<{
  steps: ReactNode;
  fills: boolean;
  /** The other ways to reach the contact, for the task's menu: always open,
   *  whatever way the type picked. */
  others?: ReactNode;
}>;
const NO_STEP: Steps = { steps: null, fills: false };

/** What reaching the task's contact can use: a mail target, a number. */
type Ways = Readonly<{
  task: Activity;
  offerOpen: boolean;
  pending: boolean;
  reachable: Contact | undefined;
  mailFills: boolean;
  // A refused mail still holds its slot, but Call is then the verb that works.
  mailWorks: boolean;
  phone: Contact | undefined;
}>;

/** A reply goes back the way its message came in. A message Margince cannot
 *  answer in kind, or none, offers both ways. */
function replySteps(ways: Ways): Omit<Steps, "others"> {
  const { task, offerOpen, pending, mailFills, phone } = ways;
  if (task.reply_channel === "email") {
    return {
      steps: <MailSteps task={task} offerOpen={offerOpen} />,
      fills: mailFills,
    };
  }
  if (task.reply_channel !== "call") return reachOutSteps(ways);
  if (pending) return { steps: <TaskNextStepsPending />, fills: true };
  return phone
    ? { steps: <CallSteps contact={phone} filled />, fills: true }
    : NO_STEP;
}

/** Email first, filled when it can be drafted; else Call is the filled one. */
function reachOutSteps(ways: Ways): Omit<Steps, "others"> {
  const { task, offerOpen, mailFills, mailWorks, phone } = ways;
  return {
    steps: (
      <MailSteps
        task={task}
        offerOpen={offerOpen}
        after={phone && <CallSteps contact={phone} filled={!mailWorks} />}
      />
    ),
    fills: mailFills || Boolean(phone),
  };
}

/** The ways the type's step leaves out, as menu lines: a mail to draft when
 *  one can be, and the call. */
function OtherWays({
  task,
  phone,
  mail,
}: Readonly<{ task: Activity; phone: Contact | undefined; mail: boolean }>) {
  const t = useT();
  const { contact } = useTaskContact(task);
  const target = task.email_target;
  const intent = intentAbout(t("task.next.draftIntent"), task.subject);
  const number = phone && primaryPhone(phone.phones)?.phone;
  return (
    <>
      {mail &&
        target &&
        (target.entity_type === "contact" ? (
          contact && (
            <EmailVerb contact={contact} intent={intent} reasonId={undefined} />
          )
        ) : (
          <RecordDraftStep target={target} intent={intent} />
        ))}
      {phone && number && (
        <CallAction
          appearance="button"
          label={t("record.verb.call")}
          number={number}
          record={{ entityType: "contact", entityId: phone.id }}
        />
      )}
    </>
  );
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
  after = null,
}: Readonly<{ task: Activity; offerOpen: boolean; after?: ReactNode }>) {
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
        after={after}
      />
    );
  }
  return (
    <>
      {target && <RecordDraftStep target={target} intent={intent} />}
      {after}
    </>
  );
}

/** The number to read or copy, for a reader at a desk, and Call to dial it. */
function CallSteps({
  contact,
  filled,
}: Readonly<{ contact: Contact; filled: boolean }>) {
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
        filled={filled}
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
  return linkVerb(
    routeHash({ screen: "book", id: `contact-${contactId}` }),
    <CalendarPlus aria-hidden="true" />,
    t("scheduling.bookContact"),
    undefined,
    "btn btn-primary",
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
  return linkVerb(
    routeHash(record.route),
    <ClipboardCheck aria-hidden="true" />,
    t(record.verb),
    undefined,
    "btn btn-primary",
  );
}

function ContactSteps({
  contact,
  intent,
  offerOpen,
  refusal,
  after,
}: Readonly<{
  contact: Contact;
  intent: string;
  offerOpen: boolean;
  refusal: string | undefined;
  after: ReactNode;
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
      {after}
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

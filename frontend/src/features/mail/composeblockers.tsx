// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { decidingRecipient, sendStatus } from "../../components/sendpermission";
import type { MessageKey } from "../../i18n/en";
import { asksWhy } from "../../lib/compose-context";
import { remedyCategoryOf } from "../../lib/sendstatus";
import type { Activity } from "./compose";
import { ComposeOutcome, ComposeWhy, MailSendNotices } from "./compose.parts";
import { RecordConversationFact } from "./composecontextfact";
import { ComposeDeal, type DealChoiceState } from "./composedeal";
import type { ComposeFields } from "./composefields";
import {
  alternativeReasonOf,
  ReasonSwitch,
  type ReasonTarget,
} from "./composereason";
import type { ComposeSend } from "./composesend";

type Preview = components["schemas"]["SendAuthorizationPreview"];

// The subject and category whose missing request can be recorded without
// changing the purpose or copying the statement to another recipient.
export function reasonTargetOf(
  permission: ComposeSend["permission"],
  contactOf: (address: string) => string | undefined,
): ReasonTarget | undefined {
  if (permission.asking || permission.unanswered) {
    return undefined;
  }
  const preview: Preview | undefined = permission.preview;
  const { state, recipient } = decidingRecipient(preview);
  if (state !== "unproven" || !recipient) {
    return undefined;
  }
  const category = remedyCategoryOf({
    category: recipient.resolved_category,
    requestedCategory: recipient.requested_category,
  });
  if (
    category !== "reply_to_inbound" &&
    category !== "requested_followup" &&
    category !== "precontract_quote" &&
    category !== "active_deal_followup"
  )
    return undefined;
  if (
    recipient.reason_code !== "no_compatible_evidence" &&
    recipient.reason_code !== "deal_not_requested"
  )
    return undefined;
  const contactId = contactOf(recipient.address);
  if (!contactId) {
    return undefined;
  }
  return {
    contactId,
    address: recipient.address,
    category,
  };
}

/**
 * The addresses the send would add as new contacts, by the engine's settled
 * answer. An answer still out, or one that failed, names none.
 */
export function newContactsOf(permission: ComposeSend["permission"]): string[] {
  if (permission.asking || permission.unanswered) {
    return [];
  }
  return (permission.preview?.recipients ?? [])
    .filter((recipient) => recipient.new_contact === true)
    .map((recipient) => recipient.address);
}

/**
 * What stands between the message and its send, in a band above the foot.
 * That is a reason for contact to choose, the engine's answer, a marketing
 * send to two addressees, or a refused send.
 * Each offers its own fix where it is read. Nothing in the way, no band.
 */
export function ComposeBlockers({
  isChannelReply,
  fields,
  send,
  anchorActivity,
  dealChoice,
  contactOf,
  bodyKey,
  permission: drawPermission,
}: Readonly<{
  isChannelReply: boolean;
  fields: ComposeFields;
  send: ComposeSend;
  anchorActivity: Activity | undefined;
  /** Which deal a new message to a contact is about, where that is asked. */
  dealChoice: DealChoiceState;
  /** The contact an address belongs to, where the records behind the draft say. */
  contactOf: (address: string) => string | undefined;
  bodyKey: MessageKey;
  /** The engine's answer, drawn by the surface that asks for it, with the
   *  reason pick this band offers where a reason may be recorded. */
  permission: (override: ReactNode, picksReason: boolean) => ReactNode;
}>) {
  const target = isChannelReply
    ? undefined
    : reasonTargetOf(send.permission, contactOf);
  // A reason the record already carries comes before recording a new one.
  const alternative = isChannelReply
    ? undefined
    : alternativeReasonOf(send.permission);
  // Until the message says why it is written, the engine has judged no reason.
  // Its missing-reason refusal would then ask the same question twice.
  const whyMissing = asksWhy(anchorActivity) && fields.context === "";
  const asksWhyAgain =
    whyMissing &&
    (decidingRecipient(send.permission.preview).state === "unproven" ||
      sendStatus(send.permission).kind === "missingContext");
  return (
    <div className="compose-blockers">
      <ComposeWhy
        anchorActivity={anchorActivity}
        flagged={send.flagged}
        context={fields.context}
        onContextChange={fields.setContext}
      />
      <ComposeDeal
        choice={dealChoice}
        onDealChange={fields.account.setDealId}
        flagged={send.flagged}
      />
      {!isChannelReply && (
        <MailSendNotices
          to={fields.to}
          cc={fields.cc}
          context={send.claimedContext}
          newContacts={newContactsOf(send.permission)}
        />
      )}
      {/* A channel reply asks no preview, so its status is only ever a
          refused Send. */}
      {(!isChannelReply || send.permission.refusal) && !asksWhyAgain && (
        <RecordConversationFact
          target={alternative ? undefined : target}
          chosen={send.claimedContext}
          dealId={send.contextDealId}
          renderOpener={(opener) =>
            drawPermission(
              alternative ? (
                <ReasonSwitch
                  reason={alternative}
                  onSwitch={fields.setContext}
                />
              ) : (
                opener
              ),
              asksWhy(anchorActivity),
            )
          }
        />
      )}
      <ComposeOutcome
        isChannelReply={isChannelReply}
        bodyKey={bodyKey}
        sendUnavailable={fields.sendUnavailable}
        send={send}
      />
    </div>
  );
}

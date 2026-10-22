// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { decidingRecipient } from "../../components/sendpermission";
import type { MessageKey } from "../../i18n/en";
import { asksWhy } from "../../lib/compose-context";
import type { Activity } from "./compose";
import { ComposeOutcome, ComposeWhy, MailSendNotices } from "./compose.parts";
import type { ComposeFields } from "./composefields";
import { ContactReasonPick, type ReasonTarget } from "./composereason";
import type { ComposeSend } from "./composesend";

type Preview = components["schemas"]["SendAuthorizationPreview"];

/**
 * Who a vouch for the deciding recipient would be recorded on. That is known
 * when the engine says one may be and the composer knows the contact.
 *
 * Absent otherwise, and the refusal then renders without the control. A form
 * that could only answer 404 is a dead button, which `SendPermission` refuses.
 * An answer still out, or one that failed, names no one either. A vouch
 * resting on the answer before it would be recorded for the wrong message.
 */
export function reasonTargetOf(
  permission: ComposeSend["permission"],
  contactOf: (address: string) => string | undefined,
): ReasonTarget | undefined {
  if (permission.asking || permission.unanswered) {
    return undefined;
  }
  const preview: Preview | undefined = permission.preview;
  const { state, recipient } = decidingRecipient(preview);
  if (state !== "unproven" || !recipient?.resolved_category) {
    return undefined;
  }
  const contactId = contactOf(recipient.address);
  if (!contactId) {
    return undefined;
  }
  return {
    contactId,
    address: recipient.address,
    category: recipient.resolved_category,
  };
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
  contactOf,
  contactId,
  bodyKey,
  permission: drawPermission,
}: Readonly<{
  isChannelReply: boolean;
  fields: ComposeFields;
  send: ComposeSend;
  anchorActivity: Activity | undefined;
  /** The contact an address belongs to, where the records behind the draft say. */
  contactOf: (address: string) => string | undefined;
  contactId: string | undefined;
  bodyKey: MessageKey;
  /** The engine's answer, drawn by the surface that asks for it, with the
   *  reason pick this band offers where a reason may be recorded. */
  permission: (override: ReactNode) => ReactNode;
}>) {
  const target = isChannelReply
    ? undefined
    : reasonTargetOf(send.permission, contactOf);
  // Until the message says why it is written, the engine has judged no reason.
  // Its missing-reason refusal would then ask the same question twice.
  const whyMissing = asksWhy(anchorActivity) && fields.context === "";
  const unproven =
    decidingRecipient(send.permission.preview).state === "unproven";
  return (
    <div className="compose-blockers">
      <ComposeWhy
        anchorActivity={anchorActivity}
        flagged={send.flagged}
        context={fields.context}
        onContextChange={fields.setContext}
      />
      {!isChannelReply && (
        <>
          <MailSendNotices
            to={fields.to}
            cc={fields.cc}
            context={send.claimedContext}
          />
          {!(whyMissing && unproven) &&
            drawPermission(
              target && (
                <ContactReasonPick
                  key={`${target.contactId}:${target.category}`}
                  target={target}
                  chosen={send.claimedContext}
                />
              ),
            )}
        </>
      )}
      <ComposeOutcome
        isChannelReply={isChannelReply}
        bodyKey={bodyKey}
        sendUnavailable={fields.sendUnavailable}
        send={send}
        contactId={contactId}
      />
    </div>
  );
}

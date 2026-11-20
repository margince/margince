// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Activity } from "./compose";
import { ComposeDeal, type DealChoiceState } from "./composedeal";
import type { ComposeFields } from "./composefields";
import type { Meeting, MeetingSubject } from "./composemeeting";
import type { ComposeSend } from "./composesend";
import { ComposeWhy } from "./composewhy";
import { pickReason } from "./reasonanchor";

// What the message must say before Send: why it is written, and the deal a
// follow-up is about. Nothing is checked until Send is pressed.
export function ComposeQuestions({
  meeting,
  fields,
  send,
  anchorActivity,
  dealChoice,
}: Readonly<{
  meeting: Meeting;
  fields: ComposeFields;
  send: ComposeSend;
  anchorActivity: Activity | undefined;
  dealChoice: DealChoiceState;
}>) {
  return (
    <>
      <ComposeWhy
        anchorActivity={anchorActivity}
        needsReason={fields.context !== ""}
        flagged={send.flagged}
        context={fields.context}
        onContextChange={pickReason(fields)}
        meeting={meeting}
      />
      <ComposeDeal
        choice={dealChoice}
        onDealChange={fields.account.setDealId}
        flagged={send.flagged}
      />
    </>
  );
}

// The record a meeting is noted on: the contact or lead the composer was
// opened from, and only while the message goes to that record's own address
// alone. A meeting with one contact is never noted on another, so a changed or
// added recipient takes "Met in person" off the list.
export function meetingSubjectOf(
  entityType: string,
  entityId: string,
  recordAddress: string | undefined,
  recipients: Readonly<{ to: string[]; cc: string[]; bcc: string[] }>,
): MeetingSubject | null {
  if (entityId === "" || !recordAddress) return null;
  if (entityType !== "contact" && entityType !== "lead") return null;
  const [only, ...others] = recipients.to;
  const alone =
    others.length === 0 &&
    recipients.cc.length === 0 &&
    recipients.bcc.length === 0 &&
    only?.trim().toLowerCase() === recordAddress.trim().toLowerCase();
  return alone ? { kind: entityType, id: entityId } : null;
}

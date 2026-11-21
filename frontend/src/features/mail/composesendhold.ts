// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MessageKey } from "../../i18n/en";

/** What the engine's answer does to the Send press, and the line under it. */
export type SendHold = Readonly<{
  /** Send cannot be pressed. */
  blocks: boolean;
  /** Every refusal may be lifted for this one message with the rep's reason. */
  withReason: boolean;
  /** Why Send is off, or what pressing it does; absent for a plain Send. */
  line?: MessageKey;
}>;

const FREE: SendHold = { blocks: false, withReason: false };

/**
 * The hold on Send when the send itself is the check. Nothing is asked while
 * the rep writes, so only a refusal no press can lift holds it: an older
 * server's footer refusal, or no mailbox to send from.
 */
export function inlineSendHold(unliftable: boolean): SendHold {
  return unliftable ? { blocks: true, withReason: false } : FREE;
}

// What the dialog promises. A scheduled message waits and can be withdrawn,
// so its title and body say so; a channel reply has no send-later field.
export function sendWording(
  isChannelReply: boolean,
  scheduling: boolean,
): Readonly<{ title: MessageKey; body: MessageKey }> {
  if (isChannelReply) {
    return {
      title: "compose.sendMessageConfirmTitle",
      body: "compose.sendMessageBody",
    };
  }
  if (scheduling) {
    return {
      title: "compose.scheduleConfirmTitle",
      body: "compose.scheduleBody",
    };
  }
  return { title: "compose.sendConfirmTitle", body: "compose.sendBody" };
}

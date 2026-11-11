// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import type { MessageKey } from "../../i18n/en";
import { previewStatus } from "../../lib/sendstatus";
import type { SendReview } from "./sendreview";

type Preview = components["schemas"]["SendAuthorizationPreview"];

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
const REFUSED: SendHold = {
  blocks: true,
  withReason: false,
  line: "compose.holdRefused",
};
const WAITING: SendHold = {
  blocks: true,
  withReason: false,
  line: "compose.holdForCheck",
};
const WITH_REASON: SendHold = {
  blocks: false,
  withReason: true,
  line: "compose.holdWithReason",
};

/**
 * The hold the permission answer puts on Send. Only an answer for the message
 * as it stands frees it. A running or failed check holds it, and so does a
 * refusal no reason may lift. After a refused Send, what the server offers
 * this caller decides, since that is what the next press can do.
 */
export function sendHoldOf(
  permission: Readonly<{
    preview: Preview | undefined;
    asking: boolean;
    unanswered: boolean;
    refusal: unknown;
  }>,
  review: SendReview | null,
): SendHold {
  if (permission.refusal) {
    return review?.actions.includes("direct_send") ? WITH_REASON : REFUSED;
  }
  const status = previewStatus(permission);
  switch (status.kind) {
    case "checking":
    case "unavailable":
      return WAITING;
    case "idle":
    case "ready":
      return FREE;
    default:
      return status.refused.every((refused) => refused.canBeOverruled)
        ? WITH_REASON
        : REFUSED;
  }
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

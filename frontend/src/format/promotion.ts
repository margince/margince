import type { components } from "../api/schema";
import type { MessageKey } from "../i18n/en";

type Trigger = components["schemas"]["PromoteLeadRequest"]["trigger"];

export const PROMOTION_TRIGGER_LABEL: Readonly<Record<Trigger, MessageKey>> = {
  inbound_reply: "lead.trigger.inboundReply",
  meeting_booked: "lead.trigger.meetingBooked",
  meeting_held: "lead.trigger.meetingHeld",
  human_qualify: "lead.trigger.humanQualify",
};

// The recorded trigger, as the outcome card names it. The wire token never
// reaches the reader.
export function promotionTriggerLabel(
  trigger: string | null | undefined,
): MessageKey | null {
  const named = Object.entries(PROMOTION_TRIGGER_LABEL).find(
    ([code]) => code === trigger,
  );
  return named ? named[1] : null;
}

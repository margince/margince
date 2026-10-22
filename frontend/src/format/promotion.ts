import type { MessageKey } from "../i18n/en";

// The recorded trigger, as the outcome card names it. The wire token never
// reaches the reader.
export function promotionTriggerLabel(
  trigger: string | null | undefined,
): MessageKey | null {
  switch (trigger) {
    case "inbound_reply":
      return "lead.trigger.inboundReply";
    case "meeting_booked":
      return "lead.trigger.meetingBooked";
    case "meeting_held":
      return "lead.trigger.meetingHeld";
    case "human_qualify":
      return "lead.trigger.humanQualify";
    default:
      return null;
  }
}

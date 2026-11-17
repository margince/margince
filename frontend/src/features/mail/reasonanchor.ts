import { useState } from "react";
import type { components } from "../../api/schema";
import type { ComposeFields } from "./composefields";

type Evidence = components["schemas"]["CommunicationEvidence"];

/**
 * A reason belongs to the message it was picked or drafted for. When the same
 * composer moves onto another anchor, it starts without one.
 */
export function useReasonForAnchor(
  answering: string | undefined,
  fields: Pick<ComposeFields, "setContext" | "setMeetingEvidence">,
): void {
  const [reasonAnchor, setReasonAnchor] = useState(answering);
  if (reasonAnchor !== answering) {
    setReasonAnchor(answering);
    fields.setContext("");
    fields.setMeetingEvidence(undefined);
  }
}

/** The deal among a message's links, which a deal follow-up rests on. */
export function dealOf(
  links:
    | readonly Readonly<{ entity_type: string; entity_id: string }>[]
    | undefined,
): string | undefined {
  return links?.find((link) => link.entity_type === "deal")?.entity_id;
}

/** The records the reason rests on: the deal it follows, the meeting it moves. */
export function reasonEvidence(
  dealId: string | undefined,
  meetingId: string | undefined,
): Evidence | undefined {
  if (!dealId && !meetingId) return undefined;
  return {
    ...(dealId ? { deal_id: dealId } : {}),
    ...(meetingId ? { activity_id: meetingId } : {}),
  };
}

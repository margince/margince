import { useQuery } from "@tanstack/react-query";
import { useActivity } from "../../lib/activityread";
import type { CommunicationContext } from "../../lib/compose-context";
import { contact360Query } from "../../lib/contact.queries";
import { dealChoicesOf } from "../mail/composedeal";

export function useProposalContext(
  contactId: string,
  dealId?: string,
  replyToActivityId?: string,
) {
  const contact = useQuery({
    ...contact360Query(contactId),
    enabled: Boolean(contactId),
  });
  const reply = useActivity(replyToActivityId);
  // The contact page's own seat read decides whether the contact still sits on
  // the starting deal. An End date ends the claim the day the server says.
  const seated = dealChoicesOf(contact.data?.deal_roles?.data ?? []).some(
    (deal) => deal.id === dealId,
  );
  const activeDealId =
    (dealId && seated ? dealId : undefined) ??
    (contact.isError ? undefined : contact.data?.commercial?.deal?.deal_id);
  const answersContact =
    reply.data?.direction === "inbound" &&
    reply.data.kind === "email" &&
    reply.data.email_summary?.counterparty_contact_id === contactId;
  const initialReason: CommunicationContext | undefined = activeDealId
    ? "active_deal_followup"
    : answersContact
      ? "requested_followup"
      : undefined;
  const contextPending =
    contact.isPending || (Boolean(replyToActivityId) && reply.isPending);
  return { activeDealId, initialReason, contextPending };
}

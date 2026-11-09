import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import { useActivity } from "../../lib/activityread";
import type { CommunicationContext } from "../../lib/compose-context";
import { contact360Query } from "../../lib/contact.queries";
import { unwrap } from "../../lib/problem";

async function readBookingDeal(contactId: string, dealId: string) {
  const deal = unwrap(
    await api.GET("/deals/{id}", { params: { path: { id: dealId } } }),
  );
  if (deal.status !== "open" || deal.archived_at) return null;
  let cursor: string | undefined;
  do {
    const page = unwrap(
      await api.GET("/relationships", {
        params: {
          query: {
            contact_id: contactId,
            deal_id: dealId,
            kind: "deal_stakeholder",
            limit: 100,
            cursor,
          },
        },
      }),
    );
    if (page.data.some((edge) => !edge.ended_at && !edge.archived_at))
      return dealId;
    cursor = page.page.next_cursor ?? undefined;
  } while (cursor);
  return null;
}

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
  const selected = useQuery({
    queryKey: ["booking-deal-context", dealId, contactId],
    enabled: Boolean(dealId),
    retry: false,
    queryFn: () => readBookingDeal(contactId, dealId ?? ""),
  });
  const activeDealId =
    (selected.isError ? undefined : selected.data) ??
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
    contact.isPending ||
    (Boolean(dealId) && selected.isPending) ||
    (Boolean(replyToActivityId) && reply.isPending);
  return { activeDealId, initialReason, contextPending };
}

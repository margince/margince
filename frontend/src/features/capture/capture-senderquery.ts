import type { QueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { MessageKey } from "../../i18n/en";
import { unwrap } from "../../lib/problem";

export const SENDERS_KEY = ["capture-senders"] as const;
export const AWAITING_KEY = ["capture-contacts-awaiting-decision"] as const;

export type SenderAnswer = "business" | "keep_out";

// The one write of a sender decision, which the Senders page and the waiting
// drawer both make.
export async function putSenderDecision(
  address: string,
  decision: SenderAnswer,
) {
  unwrap(
    await api.PUT("/capture/senders/{address}/decision", {
      params: { path: { address } },
      body: { decision },
    }),
  );
}

// A decision on a sender also settles the contacts that waited on it.
export function invalidateSenderLists(queryClient: QueryClient) {
  queryClient.invalidateQueries({ queryKey: SENDERS_KEY });
  queryClient.invalidateQueries({ queryKey: AWAITING_KEY });
  for (const key of [
    "held-threads",
    "capture-health",
    "system-health",
    "worklist",
    "contacts",
  ])
    queryClient.invalidateQueries({ queryKey: [key] });
}
export const kindLabel: Record<string, MessageKey> = {
  contact: "senders.kind.contact",
  role_mailbox: "senders.kind.roleMailbox",
  company_sender: "senders.kind.companySender",
  newsletter: "senders.kind.newsletter",
  transactional: "senders.kind.transactional",
  spam: "senders.kind.spam",
  personal: "senders.kind.personal",
  advisor: "senders.kind.advisor",
};

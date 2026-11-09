import type { MessageKey } from "../../i18n/en";
export const SENDERS_KEY = ["capture-senders"] as const;
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

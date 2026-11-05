import type { components } from "../api/schema";
import type { MessageKey } from "../i18n/en";

export const LEAD_STATUS_LABEL: Record<
  components["schemas"]["Lead"]["status"],
  MessageKey
> = {
  new: "lead.statusNew",
  contacted: "lead.statusContacted",
  engaged: "lead.statusEngaged",
  promoted: "lead.statusPromoted",
  disqualified: "lead.statusDisqualified",
};

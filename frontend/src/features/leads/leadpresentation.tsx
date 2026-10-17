import type { components } from "../../api/schema";
import { Badge } from "../../design-system/atoms";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { isOpenStatus } from "./leadpresentation.board";

export {
  isOpenStatus,
  LEAD_OPEN_STATUSES,
  LeadBoard,
  SlaBadge,
  scoreTone,
} from "./leadpresentation.board";

export type Lead = components["schemas"]["Lead"];

// The word for every rung, TOTAL over the contract's union: a status the server
// can send has a word here or this file does not compile, so no surface is left
// with a branch for a raw enum to reach a reader through.
const STATUS_LABEL: Record<Lead["status"], MessageKey> = {
  new: "lead.statusNew",
  contacted: "lead.statusContacted",
  engaged: "lead.statusEngaged",
  promoted: "lead.statusPromoted",
  disqualified: "lead.statusDisqualified",
};

// The picker's ORDER, the one thing the map above does not carry: the ladder
// reads as a progression, not an alphabet. Annotated because inferring
// `MessageKey` five times over is more than the compiler will serialise.
type StatusOption = Readonly<{ value: Lead["status"]; label: MessageKey }>;

export const LEAD_STATUS_FILTER_OPTIONS: readonly StatusOption[] = (
  ["new", "contacted", "engaged", "promoted", "disqualified"] as const
).map((value) => ({ value, label: STATUS_LABEL[value] }));

/**
 * The catalogue key for a status, shared by the badge and the readings strip,
 * so one lead cannot read "Qualified" in a pill and "promoted" beside it.
 */
export function leadStatusLabel(status: Lead["status"]): MessageKey {
  return STATUS_LABEL[status];
}

// The ladder's colours: a new lead is quiet, contact is in motion, engaged
// is the signal worth noticing; the terminal pair is muted.
function statusTone(status: Lead["status"]): "accent" | "success" | undefined {
  switch (status) {
    case "contacted":
      return "accent";
    case "engaged":
      return "success";
    default:
      return undefined;
  }
}

export function StatusBadge({ status }: Readonly<{ status: Lead["status"] }>) {
  const t = useT();
  return <Badge tone={statusTone(status)}>{t(leadStatusLabel(status))}</Badge>;
}

export function promoteEligible(lead: Lead): boolean {
  return isOpenStatus(lead.status) && Boolean(lead.email);
}

// The message key for one §3 factor name, falling back to the raw name for
// a factor the catalog does not know yet.
export function scoreFactorLabel(
  factor: string,
  t: ReturnType<typeof useT>,
): string {
  const key = `lead.factor.${factor}` as MessageKey;
  const label = t(key);
  return label === key ? factor : label;
}

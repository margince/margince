import type { components } from "../api/schema";
import type { MessageKey } from "../i18n/en";

/**
 * The closed vocabulary for winning a deal with no contract behind it.
 *
 * Its OWN module rather than an export from the deal screen, because two
 * unrelated surfaces ask the question — the deal's own win dialog and the stage
 * proposal card in the approvals queue — and a screen is a heavy thing to
 * import for a five-member list: pulling deals.tsx into the queue would drag
 * the whole design system in behind it.
 *
 * The type comes from the generated contract, and the labels are a Record over
 * it, so adding a member to `crm.yaml` stops this file compiling until the new
 * member has a label — rather than leaving a choice the server accepts and no
 * screen offers.
 */
export type WonReason = NonNullable<
  NonNullable<
    components["schemas"]["AdvanceDealRequest"]["won_without_contract_reason"]
  >
>;

export const WON_REASON_LABELS: Record<WonReason, MessageKey> = {
  purchase_order: "deals.winReasonPurchaseOrder",
  verbal: "deals.winReasonVerbal",
  renewal_by_email: "deals.winReasonRenewalByEmail",
  imported: "deals.winReasonImported",
  other: "deals.winReasonOther",
};

// Display order — deliberately not the contract's, which is a storage list.
// This one puts the answers a rep reaches for first. The Record above is what
// guarantees the set is complete; this only decides the sequence.
export const WON_REASONS: readonly WonReason[] = [
  "purchase_order",
  "verbal",
  "renewal_by_email",
  "imported",
  "other",
];

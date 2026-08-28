import type { AttentionItem } from "./today.queries";

/** What the decision lane can do with an item of a given source. */
export type FocusSurface = "merge" | "approval" | "report";

// Which body the decision lane draws for each source it can be handed.
//
// Spelled as a total map rather than a chain of ifs with an assumed else. The
// else was the defect: every source that was not a duplicate pair was handed to
// the staged-proposal card, which fetches `/approvals/{id}` with the item's id.
// For an item that is not an approval that read is a 404, so a card the reader
// cannot act on renders as a failed one, and the lane that exists to be
// finishable holds an item nothing can finish.
//
// `satisfies` is what keeps this honest: a source added to the contract has no
// entry here, and the build stops. That is the frontend half of the mirror —
// backend/gates/frontendattentionsources_test.go holds the other half, so a
// source can neither arrive unrendered nor be listed here without being emitted.
//
// "report" is a real answer, not a fallback: a source the lane cannot decide
// still gets drawn, with its headline and its detail, and without a read that
// asks the wrong endpoint about it.
export const FOCUS_SURFACE = {
  approval: "approval",
  dedupe_candidate: "merge",
  task: "report",
  brief_item: "report",
  conversation_claim: "report",
  deal_at_risk: "report",
  meeting: "report",
  relationship_decay: "report",
  failed_approval: "report",
  dsr: "report",
} as const satisfies Record<AttentionItem["source"], FocusSurface>;

export function focusSurfaceOf(source: AttentionItem["source"]): FocusSurface {
  // Indexed through the map's own key type: an unknown string cannot reach
  // here from the contract, and a missing key would have failed the build.
  return FOCUS_SURFACE[source];
}

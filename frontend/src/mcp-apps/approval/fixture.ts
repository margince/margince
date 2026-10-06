import type { Envelope } from "../types";

/**
 * One read_approval answer, shaped as the tool seals it.
 *
 * MIRRORED BY HAND from agents.StagedApproval.
 */
export const approvalFixture: Envelope = {
  data: {
    staged_action_id: "0195c3a0-0000-7000-8000-0000000000b1",
    kind: "advance_deal",
    status: "pending",
    summary: "Move Acme renewal to Negotiation",
    proposed_by: "agent:claude",
    created_at: "2026-10-06T08:00:00Z",
    proposed_change: { stage: "Negotiation", amount_minor: 1200000 },
  },
  warnings: [],
};

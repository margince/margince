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
    proposed_change: { to_stage_id: "0195c3a0-0000-7000-8000-0000000000d2" },
    on_behalf_of: {
      id: "0195c3a0-0000-7000-8000-0000000000a2",
      name: "Sam Rep",
      is_you: false,
    },
    changes: [
      {
        field: "stage_id",
        label: "Stage",
        before: { kind: "stage", text: "Proposal" },
        after: { kind: "stage", text: "Negotiation" },
      },
    ],
  },
  warnings: [],
};

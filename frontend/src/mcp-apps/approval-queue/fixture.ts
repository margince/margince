import type { Envelope } from "../types";

/**
 * One list_approvals page, shaped as the tool seals it: a proposal on its own
 * and two that one act staged together.
 *
 * MIRRORED BY HAND from agents.ApprovalPage. The deadlines are far ahead, so the
 * page still has something to decide whatever day it is read.
 */
export const approvalQueueFixture: Envelope = {
  data: {
    approvals: [
      {
        staged_action_id: "0195c3a0-0000-7000-8000-0000000000b1",
        kind: "advance_deal",
        status: "pending",
        summary: "Move Acme renewal to Negotiation",
        target_label: "Acme renewal",
        proposed_by: "agent:claude",
        created_at: "2026-10-06T08:00:00Z",
        expires_at: "2099-01-01T00:00:00Z",
        on_behalf_of: {
          id: "0195c3a0-0000-7000-8000-0000000000a2",
          is_you: true,
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
      {
        staged_action_id: "0195c3a0-0000-7000-8000-0000000000b2",
        bundle_id: "0195c3a0-0000-7000-8000-0000000000c1",
        kind: "relink_activity",
        status: "pending",
        summary: "File the call with Jonas under the Rollout project",
        target_label: "Rollout",
        proposed_by: "agent:claude",
        created_at: "2026-10-06T08:00:00Z",
        expires_at: "2099-01-01T00:00:00Z",
        on_behalf_of: {
          id: "0195c3a0-0000-7000-8000-0000000000a2",
          is_you: true,
        },
      },
      {
        staged_action_id: "0195c3a0-0000-7000-8000-0000000000b3",
        bundle_id: "0195c3a0-0000-7000-8000-0000000000c1",
        kind: "relink_activity",
        status: "pending",
        summary: "File the kickoff notes under the Rollout project",
        target_label: "Rollout",
        proposed_by: "agent:claude",
        created_at: "2026-10-06T08:00:00Z",
        expires_at: "2099-01-01T00:00:00Z",
        on_behalf_of: {
          id: "0195c3a0-0000-7000-8000-0000000000a2",
          is_you: true,
        },
      },
    ],
  },
  warnings: [],
};

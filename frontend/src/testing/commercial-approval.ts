import type { components } from "../api/schema";

type Approval = components["schemas"]["Approval"];

export function commercialApproval(over: Partial<Approval> = {}): Approval {
  return {
    id: "ap-1",
    kind: "advance_deal",
    status: "pending",
    proposed_by: "agent:runner",
    on_behalf_of: "u-1",
    requested_decider_id: "u-1",
    target_entity_type: "deal",
    target_entity_id: "11111111-1111-4111-8111-111111111111",
    target_label: "Northwind · Service renewal",
    target_version: 3,
    summary:
      "Mark the service renewal as won after the buyer confirmed acceptance.",
    proposed_change: {
      deal_id: "11111111-1111-4111-8111-111111111111",
      to_stage_id: "33333333-3333-4333-8333-333333333333",
      won_without_contract_reason: "verbal",
    },
    preview: {
      version: 1,
      state: "ready",
      current_version: 3,
      reviewer_reason: "requested_decider",
      changes: [
        {
          field: "stage_id",
          before: { kind: "stage", text: "Negotiation" },
          after: { kind: "stage", text: "Won" },
        },
        {
          field: "status",
          before: { kind: "text", text: "open" },
          after: { kind: "text", text: "won" },
        },
        {
          field: "won_without_contract_reason",
          before: { kind: "empty", text: "" },
          after: { kind: "text", text: "verbal" },
        },
        {
          field: "closed_at",
          before: { kind: "empty", text: "" },
          after: { kind: "execution", text: "" },
        },
      ],
      effects: ["classify_correspondence", "start_project_delivery"],
    },
    created_at: "2026-08-20T05:00:00Z",
    ...over,
  };
}

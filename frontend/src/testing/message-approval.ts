import type { components } from "../api/schema";

type Approval = components["schemas"]["Approval"];

export function messageApproval(): Approval {
  return {
    id: "message-approval",
    kind: "send_company_email",
    status: "pending",
    proposed_by: "agent:runner",
    created_at: "2026-09-01T09:00:00Z",
    target_entity_type: "company",
    target_entity_id: "11111111-1111-4111-8111-111111111111",
    target_label: "Harbor account",
    summary: "Send the revised delivery agreement to the buyer.",
    preview: {
      version: 2,
      state: "ready",
      reviewer_reason: "requested_decider",
      changes: [],
      evidence: [],
      effects: ["authorize_message_send"],
      details: [
        { field: "to", value: { kind: "text", text: "buyer@example.test" } },
        { field: "cc", value: { kind: "text", text: "finance@example.test" } },
        { field: "bcc", value: { kind: "text", text: "archive@example.test" } },
        {
          field: "subject",
          value: { kind: "text", text: "Revised delivery agreement" },
        },
        {
          field: "body",
          value: {
            kind: "text",
            text: "Dear Dana,\n\nThe attached agreement includes the revised delivery date.\n\nPlease confirm acceptance by Friday.",
          },
        },
        {
          field: "filed_under",
          value: { kind: "reference", text: "Harbor account" },
        },
      ],
      files: [
        {
          id: "22222222-2222-4222-8222-222222222222",
          filename: "Delivery agreement.pdf",
          content_type: "application/pdf",
          byte_size: 25000,
        },
      ],
    },
  };
}

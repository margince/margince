import type { MagicLine } from "../lib/magic.queries";

export const wonProposal: MagicLine = {
  id: "00000000-0000-7000-8000-0000000c0101",
  occurred_at: "2026-09-13T06:20:00Z",
  lane: "needs_you",
  actor: { type: "agent", id: "sales" },
  entity: {
    type: "deal",
    id: "00000000-0000-7000-8000-0000000c0102",
    label: "Nordwind renewal",
  },
  summary: {
    key: "magic.action.approval_summary",
    values: { summary: 'Close deal "Nordwind renewal" as won' },
  },
  approval_kind: "advance_deal",
  approval_preview: {
    version: 2,
    state: "ready",
    reviewer_reason: "requested_decider",
    changes: [
      {
        field: "stage_id",
        before: { kind: "stage", text: "Qualified" },
        after: { kind: "stage", text: "Won" },
      },
      {
        field: "status",
        before: { kind: "text", text: "open" },
        after: { kind: "text", text: "won" },
      },
      {
        field: "closed_at",
        before: { kind: "empty", text: "" },
        after: { kind: "execution", text: "" },
      },
    ],
    details: [
      {
        field: "company_id",
        value: { kind: "reference", text: "Nordwind Logistics" },
      },
      { field: "owner_id", value: { kind: "reference", text: "Dana Weiss" } },
      {
        field: "amount_minor",
        value: { kind: "money", text: "12500000", currency: "EUR" },
      },
    ],
    evidence: [
      {
        evidence_snippet: "We confirm the renewal. Please send the agreement.",
        source_type: "activity",
      },
    ],
  },
};

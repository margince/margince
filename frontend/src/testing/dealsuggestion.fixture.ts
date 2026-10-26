import type { DealSuggestion } from "../components/dealsuggestions.queries";

export const suggestion: DealSuggestion = {
  id: "01a0e9f1-0000-7000-8000-000000000001",
  kind: "open_deal",
  state: "open",
  company_id: "01a0e9f1-0000-7000-8000-0000000000c1",
  company_name: "Acme GmbH",
  pipeline_id: "01a0e9f1-0000-7000-8000-0000000000p1",
  stage_id: "01a0e9f1-0000-7000-8000-0000000000s1",
  name_hint: "proposal_sent",
  amount_minor: 1250000,
  currency: "EUR",
  confidence: 0.9,
  created_at: "2026-09-27T09:00:00Z",
  evidence: [
    {
      kind: "signal",
      signal_id: "01a0e9f1-0000-7000-8000-0000000000b1",
      occurred_at: "2026-09-23T10:00:00Z",
      title: "The buyer requested a platform migration proposal",
    },
    {
      kind: "attachment",
      attachment_id: "01a0e9f1-0000-7000-8000-0000000000a1",
      occurred_at: "2026-09-26T15:00:00Z",
      title: "Angebot_2026.pdf",
    },
    {
      kind: "meeting",
      activity_id: "01a0e9f1-0000-7000-8000-0000000000m1",
      occurred_at: "2026-09-24T10:00:00Z",
      title: "Scoping workshop",
    },
  ],
};

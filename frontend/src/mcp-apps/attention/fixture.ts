import type { Envelope } from "../types";

/**
 * The three answers read_attention gives, shaped as each routed tool seals it.
 *
 * MIRRORED BY HAND from agents.WhatsSlippingResult, agents.AtRiskReport and
 * agents.ReviewCommitmentsResult.
 */
export const slippingFixture: Envelope = {
  data: {
    deals: [
      {
        rank: 1,
        deal_id: "0195c3a0-0000-7000-8000-0000000000d1",
        name: "Acme renewal",
        amount_minor: 7_200_000,
        currency: "EUR",
        evidence: [
          {
            source: "deal.last_activity_at",
            snippet: "no recorded activity since 2026-06-16",
          },
          { source: "activity.task", snippet: "no open next step" },
        ],
      },
      {
        rank: 2,
        deal_id: "0195c3a0-0000-7000-8000-0000000000d2",
        name: "Nordwind rollout",
        amount_minor: 2_500_000,
        currency: "EUR",
        evidence: [
          {
            source: "deal.expected_close_date",
            snippet: "expected to close on 2026-09-30",
          },
        ],
      },
    ],
  },
  warnings: [],
};

export const atRiskFixture: Envelope = {
  data: {
    deals_scanned: 42,
    truncated: false,
    coverage_withheld: false,
    deals: [
      {
        deal_id: "0195c3a0-0000-7000-8000-0000000000d2",
        name: "Nordwind rollout",
        risks: [
          {
            kind: "single_threaded_theirs",
            summary: "Only one engaged contact on their side",
            contacts: [
              {
                contact_id: "0195c3a0-0000-7000-8000-0000000000e1",
                name: "Lena Vogt",
              },
            ],
          },
          {
            kind: "going_cold",
            summary: "No touch for 41 days",
            days_since_touch: 41,
          },
        ],
      },
    ],
  },
  warnings: [],
};

export const commitmentsFixture: Envelope = {
  data: {
    as_of: "2026-10-30T00:00:00Z",
    commitments: [
      {
        source: "conversation",
        claim_id: "0195c3a0-0000-7000-8000-0000000000f1",
        subject: "Send the revised pricing sheet",
        quote: "I'll get you the revised pricing by Friday.",
        due_at: "2026-10-24T00:00:00Z",
        state: "overdue",
        days_overdue: 6,
        assignee_id: "0195c3a0-0000-7000-8000-0000000000a2",
        assignee_name: "Sam Rep",
        about: [
          {
            entity_type: "company",
            entity_id: "0195c3a0-0000-7000-8000-0000000000c2",
            name: "Acme GmbH",
          },
        ],
      },
      {
        source: "task",
        task_id: "0195c3a0-0000-7000-8000-0000000000f2",
        subject: "Introduce their CFO",
        state: "undated",
        about: [],
      },
    ],
  },
  warnings: [],
};

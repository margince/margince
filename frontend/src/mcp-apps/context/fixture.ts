import type { Envelope } from "../types";

const grounded = (text: string) => ({ text, evidence: [] });

/**
 * The answers read_context gives, shaped as each routed tool seals it.
 *
 * MIRRORED BY HAND from agents.PrepForMeetingResult,
 * agents.AssembledContextResult and agents.ReadBriefResult.
 */
export const meetingFixture: Envelope = {
  data: {
    brief: {
      activity_id: "0195c3a0-0000-7000-8000-0000000000a1",
      generated_at: "2026-10-30T07:00:00Z",
      generated_by: "ai",
      sections: [],
      plan: {
        readiness: "prepared",
        meeting_type: "negotiation",
        meeting_type_confidence: "high",
        objective: grounded("Agree the rollout date for the second site."),
        opening: grounded(
          "Thank Ana for the pilot numbers she sent on Monday.",
        ),
        questions: [
          {
            ask: "Who signs off the second site?",
            why: "Procurement changed hands in September.",
            listen_for: "A new name",
            evidence: [],
          },
        ],
        likely_asks: [
          {
            question: "Can the price hold into next year?",
            prepare: "The discount ends on 31 December.",
            relevance: "high",
            basis: grounded("Asked twice in the last call."),
          },
        ],
        top_risk: {
          text: grounded("The budget may move to next year."),
          say: "Ask what would make this year work.",
          avoid: "Offering a discount before they ask.",
          show: "The pilot savings.",
        },
        advance: {
          best: grounded("A signed order."),
          fallback: grounded("A date for the order."),
          minimum: grounded("The name of the signer."),
        },
      },
    },
    briefing: {
      anchor: {
        record_type: "deal",
        record_id: "0195c3a0-0000-7000-8000-0000000000d1",
      },
      sections: [
        {
          name: "recent_touches",
          items: [
            {
              record_type: "activity",
              record_id: "0195c3a0-0000-7000-8000-0000000000e1",
              summary: "Call with Ana about the pilot results",
              occurred_at: "2026-10-27T09:30:00Z",
              evidence: [],
            },
          ],
        },
      ],
    },
    meeting_focus: [
      {
        record_id: "0195c3a0-0000-7000-8000-0000000000f1",
        summary: "Send the security questionnaire back",
      },
    ],
  },
};

export const catchUpFixture: Envelope = {
  data: {
    anchor: {
      record_type: "company",
      record_id: "0195c3a0-0000-7000-8000-0000000000c1",
    },
    sections: [
      {
        name: "recent_touches",
        items: [
          {
            record_type: "activity",
            record_id: "0195c3a0-0000-7000-8000-0000000000e1",
            summary: "Call with Ana about the pilot results",
            occurred_at: "2026-10-27T09:30:00Z",
            evidence: [],
          },
          {
            record_type: "activity",
            record_id: "0195c3a0-0000-7000-8000-0000000000e2",
            summary: "Pilot report sent",
            occurred_at: "2026-10-20T14:00:00Z",
            evidence: [],
          },
        ],
      },
      {
        name: "open_tasks",
        items: [
          {
            record_type: "activity",
            record_id: "0195c3a0-0000-7000-8000-0000000000e3",
            summary: "Send the security questionnaire back",
            evidence: [],
          },
        ],
      },
      { name: "related_deals", items: [] },
    ],
  },
};

const briefItem = (rank: number, state: string) => ({
  item_id: `0195c3a0-0000-7000-8000-00000000010${rank}`,
  deal_id: `0195c3a0-0000-7000-8000-00000000020${rank}`,
  rank,
  composite: 0.9 - rank / 10,
  state,
  evidence_ids: [],
  factors: {
    momentum: 0.5,
    revenue: 0.8,
    timing: 0.6,
    warmth: 0.4,
    winnability: 0.7,
  },
});

export const briefFixture: Envelope = {
  data: {
    brief_id: "0195c3a0-0000-7000-8000-000000000100",
    as_of: "2026-10-30T06:00:00Z",
    generated_at: "2026-10-30T06:00:00Z",
    local_day: "2026-10-30",
    candidate_count: 23,
    factors_omitted: [],
    items: [briefItem(1, "new"), briefItem(2, "new"), briefItem(3, "acted")],
  },
};

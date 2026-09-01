// Meeting briefs as the API serves them, for the three readers that need one:
// the stories, the unit tests and the end-to-end seed.
//
// One module rather than a literal in each, because the three must agree. A
// story showing a shape the seed never serves is a picture of a surface that
// does not exist, and the e2e run that would have caught it is reading its own
// different fixture.
//
// Dates are fixed rather than relative: the suite runs at +200 days under
// `make fe-clock-drift` and must reach the same verdict there.

import type { components } from "../../api/schema";

type MeetingBrief = components["schemas"]["MeetingBrief"];
type BriefSection = components["schemas"]["MeetingBriefSection"];

const ACTIVITY = "3f7c1a90-0000-4000-8000-00000000a001";
const DEAL = "3f7c1a90-0000-4000-8000-00000000d001";
const PERSON = "3f7c1a90-0000-4000-8000-00000000p001";

// The meeting the fixtures are about, for a caller that renders the header
// from what it already holds.
export const meetingFacts = {
  subject: "Retrofit-Abstimmung",
  startsAt: "2026-06-24T13:00:00Z",
  participants: [{ person_id: PERSON, full_name: "Anna Weber" }],
} as const;

export const preparedFor = {
  name: "Anna Weber",
  identity: PERSON,
  organizationName: "Brandt Automotive",
} as const;

function section(
  kind: BriefSection["kind"],
  sentences: BriefSection["sentences"],
): BriefSection {
  return { kind, sentences };
}

// The nine sections, each carrying the citation kind it really carries: a goal
// cites the deal it moves, an attendee line cites the person, everything else
// cites the conversation it was read from.
const NINE: BriefSection[] = [
  section("header", [
    {
      text: "Retrofit-Abstimmung with Anna Weber, 24 June, 15:00.",
      nature: "fact",
      evidence: [{ entity_type: "activity", entity_id: ACTIVITY }],
    },
  ]),
  section("goal", [
    {
      text: "Agree the pilot scope so the retrofit quote can be issued this month.",
      nature: "recommendation",
      evidence: [
        { entity_type: "deal", entity_id: DEAL, name: "Fleet retrofit" },
      ],
    },
  ]),
  section("what_changed", [
    {
      text: "Anna asked for a revised timeline after the depot survey.",
      nature: "fact",
      evidence: [{ entity_type: "activity", entity_id: ACTIVITY }],
    },
  ]),
  section("attendees", [
    {
      text: "Anna Weber, operations lead, is the only attendee from their side.",
      nature: "fact",
      evidence: [
        { entity_type: "person", entity_id: PERSON, name: "Anna Weber" },
      ],
    },
  ]),
  section("commitments", [
    {
      text: "We promised a depot-by-depot rollout plan; it is still open.",
      nature: "fact",
      evidence: [{ entity_type: "activity", entity_id: ACTIVITY }],
    },
  ]),
  section("deal_state", [
    {
      text: "The deal sits at proposal with no decision date captured.",
      nature: "assessment",
      evidence: [
        { entity_type: "deal", entity_id: DEAL, name: "Fleet retrofit" },
      ],
    },
  ]),
  section("risks", [
    {
      text: "The rollout plan we promised is late, and they have asked twice.",
      nature: "assessment",
      evidence: [{ entity_type: "activity", entity_id: ACTIVITY }],
    },
  ]),
  section("talking_points", [
    {
      text: "Bring the depot survey findings before the pricing question.",
      nature: "recommendation",
      evidence: [{ entity_type: "activity", entity_id: ACTIVITY }],
    },
  ]),
  section("company_context", [
    {
      text: "Brandt Automotive runs 240 vehicles across four depots.",
      nature: "fact",
      evidence: [{ entity_type: "organization", entity_id: DEAL }],
    },
  ]),
];

// The everyday brief: every section present, assembled without a model.
export const briefReady: MeetingBrief = {
  activity_id: ACTIVITY,
  generated_at: "2026-06-24T11:00:00Z",
  generated_by: "deterministic",
  sections: NINE,
};

// The same brief with a model lane behind it. The only difference a reader
// should see is which writer is named — the facts are the same facts.
export const briefModel: MeetingBrief = {
  ...briefReady,
  generated_by: "model",
};

// A meeting the records have nothing to say about. Not an error: a brief that
// arrives empty is a true answer about a cold record.
export const briefEmpty: MeetingBrief = {
  ...briefReady,
  sections: [],
};

// A reader whose grants keep a source out. The reason is the server's
// sentence, rendered as given.
export const briefOmitted: MeetingBrief = {
  ...briefReady,
  omitted: [
    {
      source: "deal_room",
      reason:
        "You do not have access to Deal Rooms, so what the buyer did in theirs is not in this brief.",
    },
  ],
};

// A meeting filed under a project: the server reports the scope and the picker
// stands down.
export const briefScoped: MeetingBrief = {
  ...briefReady,
  scope: {
    project_id: "3f7c1a90-0000-4000-8000-00000000e001",
    name: "Depot retrofit 2026",
    key: "RETRO",
  },
};

// A brief carrying the deterministic preparation plan.
//
// `outline` rather than `prepared`: this plan has an objective, an arc and an
// advance but no risk with a response and no ranked questions, which is exactly
// what a deployment with no model lane produces. The drawer must add it ABOVE
// the sections rather than in place of them, and this fixture is what proves it.
export const briefWithPlan: MeetingBrief = {
  ...briefReady,
  plan: {
    generated_by: "deterministic",
    readiness: "outline",
    meeting_type: { value: "commercial", confidence: "high" },
    objective: {
      sentence: {
        text: "Leave with Anna's answer on: the depot-by-depot rollout plan.",
        nature: "recommendation",
        evidence: [{ entity_type: "activity", entity_id: ACTIVITY }],
      },
      caveat: "Do not concede on price before the value is agreed.",
    },
    opening: {
      text: "Open on what has changed since you last spoke, before proposing anything.",
      nature: "recommendation",
      evidence: [{ entity_type: "activity", entity_id: ACTIVITY }],
    },
    likely_asks: [],
    questions: [],
    scenarios: [],
    account_arc: [
      {
        from: "2026-04-02T09:00:00Z",
        to: "2026-04-19T09:00:00Z",
        title: "Depot survey",
        summary: {
          text: "2 Apr–19 Apr: 6 conversations, on Depot survey.",
          evidence: [{ entity_type: "activity", entity_id: ACTIVITY }],
        },
      },
    ],
    advance: {
      minimum: {
        text: "The remaining gap between our number and theirs, stated.",
        nature: "recommendation",
        evidence: [{ entity_type: "activity", entity_id: ACTIVITY }],
      },
      best: {
        text: "Agreed terms and a signature date.",
        nature: "recommendation",
        evidence: [{ entity_type: "activity", entity_id: ACTIVITY }],
      },
      fallback: {
        text: "What has to be true for them to sign, and by when.",
        nature: "recommendation",
        evidence: [{ entity_type: "activity", entity_id: ACTIVITY }],
      },
    },
    unknowns: [
      {
        kind: "decision_route_not_captured",
        question: "Who else has to agree before this can go ahead?",
      },
    ],
  },
};

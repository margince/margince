// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../../testing/story-utils";
import { DealHowItStands, type DealStandingHead } from "./dealhowitstands";
import type { DealStandingInputs } from "./dealstanding";

// How a deal stands, as its Overview draws it. It has the model's call with
// its sources and the stage ladder with the time spent on each rung. It also
// has the three readings, the dated story and the four figures. The second
// story is a deal nobody has read yet: every reading says so in words, never with a zero.

const meta: Meta = {
  title: "Records/Deal 360/How it stands",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;
type Deal = components["schemas"]["Deal"];
type Stage = components["schemas"]["Stage"];
type Activity = components["schemas"]["Activity"];

const DEAL_ID = "01a03000-0000-7000-8000-000000000001";
const MAIL = "01a03000-0000-7000-8000-0000000000aa";
const AS_OF = "2026-08-13T09:00:00Z";

const STAGE_NAMES = [
  "Potential deal",
  "Qualification",
  "Solution discovery",
  "Conversion strategy",
  "Proposal sent",
  "Negotiation",
  "Contract review",
];

const STAGES: Stage[] = [
  ...STAGE_NAMES.map(
    (name, index): Stage => ({
      id: `s${index + 1}`,
      pipeline_id: "pl",
      name,
      position: index + 1,
      semantic: "open",
      win_probability: 10 + index * 10,
    }),
  ),
  {
    id: "won",
    pipeline_id: "pl",
    name: "Closed won",
    position: 8,
    semantic: "won",
    win_probability: 100,
  },
  {
    id: "lost",
    pipeline_id: "pl",
    name: "Closed lost",
    position: 9,
    semantic: "lost",
    win_probability: 0,
  },
];

const DEAL: Deal = {
  id: DEAL_ID,
  name: "Fleet retrofit",
  amount_minor: 4_800_000,
  currency: "EUR",
  pipeline_id: "pl",
  stage_id: "s3",
  status: "open",
  stalled: false,
  last_activity_at: "2026-08-01T10:00:00Z",
  expected_close_date: "2026-09-30",
  forecast_category: "best_case",
  source: "manual",
  captured_by: "human:u1",
  created_at: "2026-06-02T09:00:00Z",
  updated_at: "2026-08-01T10:00:00Z",
};

function activity(
  row: Pick<Activity, "id" | "kind" | "occurred_at"> & Partial<Activity>,
): Activity {
  return {
    occurred_precision: "instant",
    offers_email: false,
    is_done: false,
    source: "manual",
    captured_by: "human:u1",
    created_at: row.occurred_at,
    updated_at: row.occurred_at,
    ...row,
  };
}

const ACTIVITIES: Activity[] = [
  activity({
    id: "a1",
    kind: "meeting",
    meeting_status: "held",
    subject: "Kickoff call",
    occurred_at: "2026-06-03T09:00:00Z",
  }),
  activity({
    id: "a2",
    kind: "meeting",
    meeting_status: "held",
    subject: "Site visit",
    occurred_at: "2026-07-20T09:00:00Z",
  }),
  activity({
    id: MAIL,
    kind: "email",
    direction: "inbound",
    subject: "Re: retrofit timeline",
    occurred_at: "2026-08-01T10:00:00Z",
  }),
  activity({
    id: "a4",
    kind: "meeting",
    meeting_status: "booked",
    subject: "Walkthrough",
    occurred_at: "2026-08-20T09:00:00Z",
  }),
];

const ladder = { advancing: false, advanceRefused: false, onAdvance: () => {} };

const read: DealStandingHead = {
  verdict: {
    word: "Drifting",
    tone: "warning",
    sentence:
      "Offer v2 has gone unanswered and the deal runs through Dana alone, 48 days before close.",
  },
  sources: { by: "model", count: 4 },
  readAt: AS_OF,
};

function stageHistory() {
  const move = (id: string, from: string | null, to: string, at: string) => ({
    id,
    entity_type: "deal",
    entity_id: DEAL_ID,
    field: "stage_id",
    old_value: from,
    new_value: to,
    changed_at: at,
    actor_type: "human",
    actor_id: "u1",
  });
  return jsonResponse({
    data: [
      move("h3", "s2", "s3", "2026-08-07T09:00:00Z"),
      move("h2", "s1", "s2", "2026-06-18T09:00:00Z"),
      move("h1", null, "s1", "2026-06-02T09:00:00Z"),
    ],
    page: { next_cursor: null, has_more: false },
  });
}

const drifting: DealStandingInputs = {
  deal: DEAL,
  stages: STAGES,
  asOf: AS_OF,
  activities: ACTIVITIES,
  offers: [],
  coverage: {
    withheld: false,
    pending: false,
    coverage: {
      deal_id: DEAL_ID,
      stakeholders: [
        {
          contact_id: "c1",
          contact_name: "Dana Buyer",
          role: "champion",
          engaged: true,
        },
      ],
      our_side: [],
      risks: [{ kind: "coverage_gap", summary: "No economic buyer" }],
      sections_omitted: [],
    },
  },
  card: {
    deal_id: DEAL_ID,
    story: { sentences: [] },
    reply_to: MAIL,
    generated_at: AS_OF,
    generated_by: "model",
  },
  ladder,
};

/** A deal the model read as drifting: a reply owed, one seat on the deal. */
export const Drifting: Story = {
  render: () => {
    installFetchStub({ "GET /field-history": stageHistory });
    return (
      <StoryProviders>
        <DealHowItStands head={read} inputs={drifting} />
      </StoryProviders>
    );
  },
};

export const DriftingDark: Story = { ...Drifting, globals: { theme: "dark" } };

/**
 * A deal won while the last reading still had it drifting. The outcome heads
 * the card, and only who sat on the deal stays beside it.
 */
export const Won: Story = {
  render: () => {
    installFetchStub({ "GET /field-history": stageHistory });
    return (
      <StoryProviders>
        <DealHowItStands
          head={read}
          inputs={{
            ...drifting,
            deal: { ...DEAL, status: "won", closed_at: "2026-08-12T12:00:00Z" },
          }}
        />
      </StoryProviders>
    );
  },
};

/** A lost deal says why, in grey, under the outcome. */
export const Lost: Story = {
  render: () => {
    installFetchStub({ "GET /field-history": stageHistory });
    return (
      <StoryProviders>
        <DealHowItStands
          head={read}
          inputs={{
            ...drifting,
            deal: {
              ...DEAL,
              status: "lost",
              closed_at: "2026-08-12T12:00:00Z",
              lost_reason: "Budget moved to next year",
            },
          }}
        />
      </StoryProviders>
    );
  },
};

/**
 * A new deal nobody has read: no verdict, no history the reader may see, no
 * committee, no activity. Every reading names what it lacks.
 */
export const NotAssessed: Story = {
  render: () => {
    installFetchStub({
      "GET /field-history": () => new Response(null, { status: 403 }),
    });
    return (
      <StoryProviders>
        <DealHowItStands
          head={{
            verdict: { word: "Not assessed", tone: "neutral" },
            sources: { by: "records" },
          }}
          inputs={{
            ...drifting,
            deal: {
              ...DEAL,
              stage_id: "s1",
              amount_minor: null,
              expected_close_date: null,
              last_activity_at: null,
            },
            activities: [],
            coverage: { withheld: true, pending: false },
            card: undefined,
          }}
        />
      </StoryProviders>
    );
  },
};

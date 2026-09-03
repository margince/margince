// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { LeadReadings } from "./leadreadings";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

type Lead = components["schemas"]["Lead"];

// The lead's four readings, in the states the record page can only show one of
// at a time.
//
// Every slot on this row states its absence IN WORDS, and that is the whole
// reason the file exists: an empty slot reads as a page that failed to load,
// not as a lead with no company or an installation running no response target.
// The frames below are the four absences and the one full day, so a slot that
// starts drawing a blank has somewhere to be caught.
//
// The status slot is a DOOR — it names the set of leads sharing that status and
// opens `#/leads?status=<status>`, so the card's foot carries the way out. It is
// drawn on every frame, including the terminal one, because a closed lead's
// status is as much an address as an open lead's.
//
// Read every frame in BOTH themes: the first-response slot is the one reading
// that carries tone and a dot, and both are `color-mix()` of a canonical token.

const lead: Lead = {
  id: "l-1",
  full_name: "Jonas Petersen",
  email: "jonas@nordwind.example",
  title: "Head of Logistics",
  company_name: "Nordwind Logistik",
  status: "contacted",
  score: 72,
  score_reason: "decision_maker_title",
  source: "manual",
  captured_by: "human:u1",
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-04T08:00:00Z",
};

// The score read as the server answers it once the factors are retained: the
// figure above, the arithmetic behind it. `explained` is what tells "nothing
// counted" apart from "this client cannot say what counted".
const explained = {
  score: 72,
  explained: true,
  current: {
    score: 72,
    score_computed: 72,
    factors: [
      { factor: "decision_maker_title", points: 15 },
      { factor: "high_intent_source", points: 8 },
      { factor: "reply", points: 22.6, base_points: 25 },
    ],
  },
};

/** One lead's readings, with the score read the card fans out to answered. */
function readings(subject: Lead, score: unknown = explained) {
  const routes: RouteMap = {
    // Routed rather than left to the harness, which refuses to guess a
    // session and says so through the console: the principal a frame is about
    // is stated with the frame. These readings probe none themselves, and a
    // slot that grows one must not be the first to discover that at the gate.
    "GET /me": meRoute({ lead: ["read", "update"] }),
    [`GET /leads/${subject.id}/score`]: () => jsonResponse(score),
  };
  return () => {
    installFetchStub(routes);
    return (
      <StoryProviders>
        <LeadReadings lead={subject} />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof LeadReadings> = {
  title: "Records/Leads/Readings",
  component: LeadReadings,
};
export default meta;

type Story = StoryObj<typeof LeadReadings>;

// A lead with all four readings answered: a scored figure over its meter, a
// first response that went out, a status that opens the pile it belongs to, and
// a company. The row every other frame is a departure from.
export const Answered: Story = {
  render: readings({
    ...lead,
    first_response_at: "2026-06-02T09:12:00Z",
    status: "engaged",
  }),
};

// The receipt under the score, open. It is a popover, so nothing in the catalog
// ever shows the factors unless a frame presses the trigger — and the factors
// are the reading's whole basis: a figure whose working cannot be reached is a
// figure a reader has to take on trust.
export const ScoreReceipt: Story = {
  render: readings({ ...lead, first_response_at: "2026-06-02T09:12:00Z" }),
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", {
        name: "How it stands",
      }),
    );
  },
};

// Nobody has answered and the deadline has passed. The one slot on this row
// that carries a verdict rather than a fact says it in a word, in the danger
// family, with the dot that lets a reader catch it without reading — and names
// the moment it went past rather than the one it was due by.
export const FirstResponseBreached: Story = {
  render: readings({
    ...lead,
    status: "new",
    sla_deadline_at: "2026-06-01T12:00:00Z",
    sla_state: "breached",
  }),
};

// A lead as a web form delivers one: a name, and nothing else yet. Three of the
// four slots are stating an absence — no company, a score nothing has counted
// toward, and a first response owed against no deadline because the
// installation runs no target. None of the three may draw blank, and a 0 that
// reads as a bad prospect is the same defect wearing a figure.
export const DetailsUnset: Story = {
  render: readings(
    {
      ...lead,
      status: "new",
      score: 0,
      score_reason: null,
      title: null,
      company_name: null,
    },
    { score: 0, explained: true },
  ),
};

// A closed lead. The status reads with its TERMINAL wording rather than its
// ladder rung, and the first-response slot reports what happened instead of
// what is due — a disqualified lead owes nobody an answer, and the server sends
// no clock for one, so a deadline here would be a promise nobody made.
export const Disqualified: Story = {
  render: readings({
    ...lead,
    status: "disqualified",
    disqualify_reason: "No budget this year",
    archived_at: "2026-07-13T00:00:00Z",
  }),
};

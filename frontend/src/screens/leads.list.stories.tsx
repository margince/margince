// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { LeadsScreen } from "./leads.list";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

type Lead = components["schemas"]["Lead"];

// The lead queue. A lead is not a person yet, and the screen's whole job is to
// keep that true: the row opens the LEAD's own page, never the person's, and
// the owner column answers "whose lead is this" rather than "who typed it".
//
// Which leads a reader opens on is a ROLE question, not a filter the screen
// remembers — an admin or manager runs the queue and opens on all of it, a rep
// opens on their own. So the two are separate stories with different `/me`
// identities rather than one story with a control pressed.
//
// The score is the figure a reader scans down the column, and it is a magnitude
// like any other: `HighScores` carries four digits, which is the only width at
// which de-DE grouping is visible at all.

function lead(overrides: Partial<Lead> = {}): Lead {
  return {
    id: "l-1",
    full_name: "Jonas Petersen",
    email: "jonas.petersen@brandt-automotive.example",
    title: "Head of Procurement",
    company_name: "Brandt Automotive GmbH",
    status: "new",
    score: 72,
    score_reason: "engagement",
    owner_id: "u-lena",
    sla_state: "within_target",
    source: "manual",
    captured_by: "u-me",
    version: 1,
    created_at: "2026-08-20T09:00:00Z",
    updated_at: "2026-08-24T11:00:00Z",
    ...overrides,
  };
}

const ROSTER = {
  data: [
    { id: "u-me", display_name: "Ada Ops", kind: "user" },
    { id: "u-lena", display_name: "Lena Fischer", kind: "user" },
  ],
  page: { next_cursor: null },
};

// `/me` and the list are routed explicitly; everything else this screen reads —
// the roster, the custom fields, the SoR posture — falls through to the stub's
// own empty page, which is a legitimate answer for each of them and keeps the
// story about the queue rather than about its chrome.
function leads(
  rows: Lead[],
  identity: { roles?: string[]; seat?: "full" | "read" } = {
    roles: ["manager"],
  },
) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({ lead: ["read", "update"] }, identity),
      "GET /leads": () =>
        jsonResponse({ data: rows, page: { next_cursor: null } }),
      "GET /leads/settings": () =>
        jsonResponse({ first_response_enabled: true }),
      "GET /users": () => jsonResponse(ROSTER),
    });
    return (
      <StoryProviders>
        <LeadsScreen />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof LeadsScreen> = {
  title: "Records/Leads/Queue",
  component: LeadsScreen,
};
export default meta;
type Story = StoryObj<typeof LeadsScreen>;

/** A manager's queue: every lead, whoever owns it. */
export const ManagerOpensOnAll: Story = {
  render: leads([
    lead(),
    lead({
      id: "l-2",
      full_name: "Marta Alvarez",
      company_name: "Voss Logistics",
      status: "contacted",
      score: 58,
      sla_state: "breached",
    }),
    lead({
      id: "l-3",
      full_name: "Tobias Krause",
      company_name: "Sindelfingen Werke",
      status: "engaged",
      score: 91,
      owner_id: "u-me",
    }),
  ]),
};

/** A rep's queue: their own leads. Same screen, a different answer from
 *  `/me` — the segregation is a rule about the seat, not a saved filter. */
export const RepOpensOnTheirOwn: Story = {
  render: leads([lead({ owner_id: "u-me" })], { roles: ["rep"] }),
};

/** Nobody has routed a lead here yet. An empty queue is a real state and says
 *  so, rather than drawing a table with no rows. */
export const EmptyQueue: Story = { render: leads([]) };

/**
 * Scores wide enough to be written in a notation. A score column is read DOWN,
 * so a figure grouped in one row and bare in the next is the defect at its most
 * visible — and four digits is where de-DE first groups.
 */
export const HighScores: Story = {
  render: leads([
    lead({ score: 1204 }),
    lead({ id: "l-2", full_name: "Marta Alvarez", score: 48_213 }),
    lead({ id: "l-3", full_name: "Tobias Krause", score: 991 }),
  ]),
};

/** The same queue in German — the score column and the longer status words. */
export const HighScoresGerman: Story = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute({ lead: ["read", "update"] }, { roles: ["manager"] }),
      "GET /leads": () =>
        jsonResponse({
          data: [lead({ score: 1204 }), lead({ id: "l-2", score: 48_213 })],
          page: { next_cursor: null },
        }),
      "GET /leads/settings": () =>
        jsonResponse({ first_response_enabled: true }),
      "GET /users": () => jsonResponse(ROSTER),
    });
    return (
      <StoryProviders locale="de">
        <LeadsScreen />
      </StoryProviders>
    );
  },
};

/**
 * A score somebody overrode by hand. The row says it was overridden rather than
 * quietly showing a number the model did not produce — the reason is the point,
 * and a bare figure would pass a human's judgement off as the system's.
 */
export const OverriddenScore: Story = {
  render: leads([
    lead({
      score: 95,
      score_computed: 41,
      score_override_reason:
        "Met at the trade fair; budget confirmed verbally.",
    }),
  ]),
};

/** A read-only seat: the rows are there and the verbs are not. */
export const ReadOnly: Story = {
  render: leads([lead()], { roles: ["rep"], seat: "read" }),
};

/** Long content: a name and a company that cannot share a line at any width. */
export const LongNames: Story = {
  render: leads([
    lead({
      full_name: "Marta Alvarez de Sotomayor-Whitfield",
      title: "Interim Head of Group Procurement and Supplier Development",
      company_name:
        "Sindelfingen Werke für Fahrzeugtechnik und Systemintegration GmbH & Co. KG",
    }),
  ]),
};

/** At 390px the table becomes a list of cards and the score has to stay beside
 *  the name it belongs to. */
export const Phone: Story = {
  tags: ["uat-phone"],
  render: leads([lead({ score: 1204 }), lead({ id: "l-2", score: 58 })]),
};

/** The read in flight: placeholder rows under a toolbar that is already
 *  usable, rather than an empty grid that reads as "no leads". */
export const Loading: Story = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute({ lead: ["read", "update"] }, { roles: ["manager"] }),
      // Never settles, which is the state this story is about. Storybook tears
      // the story down on navigation, so nothing is left pending afterwards.
      "GET /leads": () => new Promise<Response>(() => undefined),
      "GET /users": () => jsonResponse(ROSTER),
    });
    return (
      <StoryProviders>
        <LeadsScreen />
      </StoryProviders>
    );
  },
};

/** The read refused. The surface says so and offers the retry — an empty
 *  table here would be a claim about the data instead of about the request. */
export const Failed: Story = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute({ lead: ["read", "update"] }, { roles: ["manager"] }),
      "GET /leads": () =>
        jsonResponse(
          {
            type: "about:blank",
            title: "Internal Server Error",
            status: 500,
            detail: "The lead index is rebuilding.",
          },
          500,
        ),
      "GET /users": () => jsonResponse(ROSTER),
    });
    return (
      <StoryProviders>
        <LeadsScreen />
      </StoryProviders>
    );
  },
};

/** More than one page: the count line says what is loaded rather than
 *  inventing a total the cursor cannot know, and the pager offers the next. */
export const MorePages: Story = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute({ lead: ["read", "update"] }, { roles: ["manager"] }),
      "GET /leads": () =>
        jsonResponse({
          data: [
            lead(),
            lead({ id: "l-2", full_name: "Marta Alvarez", score: 58 }),
            lead({ id: "l-3", full_name: "Tobias Krause", score: 91 }),
          ],
          page: { next_cursor: "c-2", has_more: true },
        }),
      "GET /leads/settings": () =>
        jsonResponse({ first_response_enabled: true }),
      "GET /users": () => jsonResponse(ROSTER),
    });
    return (
      <StoryProviders>
        <LeadsScreen />
      </StoryProviders>
    );
  },
};

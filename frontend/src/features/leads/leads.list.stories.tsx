// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { components } from "../../api/schema";
import { playOpenView } from "../../testing/listview.play";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { LeadsScreen } from "./leads.list";

type Lead = components["schemas"]["Lead"];

// Which leads a reader opens on follows the role, so a manager and a rep are
// separate stories with their own `/me`, not one story with a control pressed.

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

// Hoisted, since a literal default is a fresh object on every call.
const MANAGER: { roles?: string[]; seat?: "full" | "read" } = {
  roles: ["manager"],
};

function leads(
  rows: Lead[],
  identity: { roles?: string[]; seat?: "full" | "read" } = MANAGER,
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

/** Four-digit scores: the width at which de-DE first groups, read down one
 *  column. */
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

/** A score overridden by hand says so, so a person's figure never reads as
 *  the model's. */
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

// Table or Board stands at the top of View, above the list's dials.
async function layoutInView({
  canvasElement,
}: Readonly<{ canvasElement: HTMLElement }>) {
  const panel = within(await playOpenView(canvasElement));
  await expect(panel.getByRole("button", { name: "Table" })).toBeVisible();
  await expect(panel.getByRole("button", { name: "Board" })).toBeVisible();
  await expect(panel.getByRole("button", { name: /^Filters,/ })).toBeVisible();
}

export const ViewPanel: Story = {
  render: leads([lead(), lead({ id: "l-2", status: "contacted" })]),
  play: layoutInView,
};

export const ViewPanelPhone: Story = {
  tags: ["uat-phone"],
  render: leads([lead(), lead({ id: "l-2", status: "contacted" })]),
  play: layoutInView,
};

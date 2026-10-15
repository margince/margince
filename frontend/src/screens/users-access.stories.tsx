// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { viewerZone } from "../format/timezone";
import type { GrantSpec } from "../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { TeamsCard } from "./users-access";

// One row per team: its name (and parent), who is on it, and its verbs. The
// roster's `team_ids` are what the faces are drawn from.

const TEAMS = [
  { id: "t-1", name: "DACH Sales", member_count: 4 },
  { id: "t-2", name: "Benelux", member_count: 1 },
  { id: "t-3", name: "Enterprise (forming)", member_count: 0 },
  {
    id: "t-4",
    name: "Strategic accounts for the German speaking region and Benelux, renewals and expansion",
    member_count: 13,
    parent_team_id: "t-1",
  },
];

const USERS = Array.from({ length: 16 }, (_, i) => ({
  id: `u-${i}`,
  email: `member${i}@acme.test`,
  display_name:
    i === 0 ? "Maximiliane von Habsburg-Lothringen" : `Colleague ${i}`,
  timezone: viewerZone(),
  status: "active",
  is_agent: false,
  team_ids: [
    ...(i < 4 ? ["t-1"] : []),
    ...(i === 4 ? ["t-2"] : []),
    ...(i < 13 ? ["t-4"] : []),
  ],
}));

const ADMIN: GrantSpec = {
  team_admin: ["read", "create", "update"],
  user_admin: ["read", "update"],
};

type TeamsRead = () => Response | Promise<Response>;

const listed = (teams: Record<string, unknown>[]) => () =>
  jsonResponse({ data: teams, page: { next_cursor: null, has_more: false } });

function story(teams: TeamsRead, allow: GrantSpec = ADMIN) {
  return () => {
    installFetchStub({
      "GET /me": meRoute(allow),
      "GET /teams": teams,
      "GET /users": () =>
        jsonResponse({
          data: USERS,
          page: { next_cursor: null, has_more: false },
        }),
    });
    return (
      <StoryProviders>
        <TeamsCard />
      </StoryProviders>
    );
  };
}

const meta = {
  title: "Settings/People/Teams/Teams",
  component: TeamsCard,
} satisfies Meta<typeof TeamsCard>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Teams: Story = { render: story(listed(TEAMS)) };

// The intro and the empty line share one body; the create verb stays below.
export const NoTeams: Story = { render: story(listed([])) };

export const Loading: Story = {
  render: story(() => new Promise<Response>(() => undefined)),
};

// A failed read offers a retry and no create verb.
export const Failed: Story = {
  render: story(() =>
    jsonResponse(
      { title: "Internal Server Error", status: 500, detail: "Read failed." },
      500,
    ),
  ),
};

// Membership readable, nothing changeable: no create verb, no row menu.
export const ReadOnly: Story = {
  render: story(listed(TEAMS), { team_admin: ["read"], user_admin: ["read"] }),
};

// Below 36rem a row folds: the name and its menu on line one, the faces and
// count on line two.
export const TeamsPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story(listed(TEAMS)),
};

export const NewTeamDialog: Story = {
  render: story(listed(TEAMS)),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "New team" }),
    );
    await within(document.body).findByRole("dialog");
  },
};

// A seat without `user_admin:read` sees the teams and their counts, and no
// faces and no door to the members.
export const MembershipWithheld: Story = {
  render: story(listed(TEAMS), { team_admin: ["read"] }),
};

export const MembersDialog: Story = {
  render: story(listed(TEAMS)),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "DACH Sales" }),
    );
    await within(document.body).findByRole("dialog");
  },
};

export const RowMenu: Story = {
  render: story(listed(TEAMS)),
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", {
        name: "Actions for DACH Sales",
      }),
    );
  },
};

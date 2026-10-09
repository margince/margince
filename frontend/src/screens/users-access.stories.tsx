// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { GrantSpec } from "../app/mefixture";
import { viewerZone } from "../format/timezone";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";
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
  email: `person${i}@acme.test`,
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

function story(teams: Record<string, unknown>[], allow: GrantSpec = ADMIN) {
  return () => {
    installFetchStub({
      "GET /me": meRoute(allow),
      "GET /teams": () =>
        jsonResponse({
          data: teams,
          page: { next_cursor: null, has_more: false },
        }),
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

const meta: Meta<typeof TeamsCard> = {
  title: "Settings/People/Teams/Teams",
  component: TeamsCard,
};
export default meta;
type Story = StoryObj<typeof TeamsCard>;

export const Teams: Story = { render: story(TEAMS) };

export const NoTeams: Story = { render: story([]) };

// Below 36rem a row folds: the name and its menu on line one, the faces and
// count on line two.
export const TeamsPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story(TEAMS),
};

export const NewTeamDialog: Story = {
  render: story(TEAMS),
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
  render: story(TEAMS, { team_admin: ["read"] }),
};

export const MembersDialog: Story = {
  render: story(TEAMS),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "DACH Sales" }),
    );
    await within(document.body).findByRole("dialog");
  },
};

export const RowMenu: Story = {
  render: story(TEAMS),
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", {
        name: "Actions for DACH Sales",
      }),
    );
  },
};

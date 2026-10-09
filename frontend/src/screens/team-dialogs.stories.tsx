// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";
import {
  RenameTeamAction,
  TeamMembersModal,
  type TeamUser,
} from "./team-dialogs";

const TEAM = { id: "t-1", name: "DACH Sales", member_count: 2 };
const LONG_TEAM = {
  id: "t-1",
  name: "Enterprise accounts for the German speaking region and Benelux",
  member_count: 13,
};

function person(index: number, name: string, onTeam: boolean): TeamUser {
  return {
    id: `u-${index}`,
    email: `person${index}@acme.test`,
    display_name: name,
    timezone: "Europe/Berlin",
    status: "active",
    is_agent: false,
    team_ids: onTeam ? ["t-1"] : [],
  };
}

const USERS = [
  person(1, "Ada Lovelace", true),
  person(2, "Bo Andersen", true),
  person(3, "Chiara Rossi", false),
  person(4, "Dang Thi Mai", false),
];
const MANY = [
  person(0, "Maximiliane Theodora von Habsburg-Lothringen", true),
  ...Array.from({ length: 12 }, (_, i) =>
    person(i + 1, `Colleague ${i + 1}`, true),
  ),
  person(20, "Not yet on the team", false),
];

const PROBLEM = {
  type: "about:blank",
  title: "Forbidden",
  status: 403,
  code: "team_membership_requires_admin",
  detail: "Only an admin changes who is on a team.",
};

function story(
  team: typeof TEAM,
  users: TeamUser[],
  options: Readonly<{ canEdit?: boolean; refuse?: boolean }> = {},
) {
  return () => {
    const answer = () =>
      options.refuse
        ? jsonResponse(PROBLEM, 403)
        : new Response(null, { status: 204 });
    installFetchStub({
      "PUT /teams/t-1/members/u-3": answer,
      "DELETE /teams/t-1/members/u-1": answer,
    });
    return (
      <StoryProviders>
        <TeamMembersModal
          team={team}
          users={users}
          usersPartial={false}
          canEdit={options.canEdit ?? true}
          open
          onClose={() => {}}
        />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof TeamMembersModal> = {
  title: "Settings/People/Teams/Team members",
  component: TeamMembersModal,
};
export default meta;
type Story = StoryObj<typeof TeamMembersModal>;

export const Members: Story = { render: story(TEAM, USERS) };

// Thirteen faces and a name too long for one line: the list scrolls inside the
// dialog while the title and Done stay put.
export const ManyMembers: Story = { render: story(LONG_TEAM, MANY) };

// A seat that reads membership and may not change it keeps the list and loses
// the verbs, with the reason said once.
export const ReadOnly: Story = {
  render: story(TEAM, USERS, { canEdit: false }),
};

export const NoMembers: Story = {
  render: story({ ...TEAM, member_count: 0 }, [
    person(3, "Chiara Rossi", false),
  ]),
};

// The write is refused: the dialog says so and the list stays as it was.
export const RemoveRefused: Story = {
  render: story(TEAM, USERS, { refuse: true }),
  play: async () => {
    const dialog = within(await within(document.body).findByRole("dialog"));
    await userEvent.click(
      dialog.getByRole("button", {
        name: "Remove Ada Lovelace from DACH Sales",
      }),
    );
    await expect(
      await dialog.findByText("Membership not changed"),
    ).toBeInTheDocument();
    await expect(dialog.getByText("Ada Lovelace")).toBeInTheDocument();
  },
};

export const ManyMembersPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story(LONG_TEAM, MANY),
};

export const RenameDialog: Story = {
  render: () => (
    <StoryProviders>
      <RenameTeamAction team={TEAM} />
    </StoryProviders>
  ),
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", { name: "Rename" }),
    );
    await within(document.body).findByRole("dialog");
  },
};

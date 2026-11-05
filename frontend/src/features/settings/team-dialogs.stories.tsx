// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { browserZone } from "../../format/timezone";
import { en } from "../../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
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

function member(index: number, name: string, onTeam: boolean): TeamUser {
  return {
    id: `u-${index}`,
    email: `member${index}@acme.test`,
    display_name: name,
    timezone: browserZone(),
    status: "active",
    is_agent: false,
    team_ids: onTeam ? ["t-1"] : [],
  };
}

const USERS = [
  member(1, "Ada Lovelace", true),
  member(2, "Bo Andersen", true),
  member(3, "Chiara Rossi", false),
  member(4, "Dang Thi Mai", false),
];
const MANY = [
  member(0, "Maximiliane Theodora von Habsburg-Lothringen", true),
  ...Array.from({ length: 12 }, (_, i) =>
    member(i + 1, `Colleague ${i + 1}`, true),
  ),
  member(20, "Not yet on the team", false),
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

// A team of 120, searched. The box narrows the list to the names that hold
// the typed words, and says how many of the team that is.
const LARGE = Array.from({ length: 120 }, (_, i) =>
  member(i + 1, `Roster Member ${String(i).padStart(3, "0")}`, true),
);
export const SearchedLargeTeam: Story = {
  render: story({ ...TEAM, member_count: 120 }, LARGE),
  play: async () => {
    const dialog = within(await within(document.body).findByRole("dialog"));
    await userEvent.type(
      dialog.getByRole("searchbox", { name: "Search team members" }),
      "Member 07",
    );
    // Names and emails both match, so the story asks only that the count of
    // the whole team is stated. The unit test pins the exact rows.
    await expect(await dialog.findByText(/ of 120 members$/)).toBeTruthy();
  },
};

// A seat that reads membership and may not change it keeps the list and loses
// the verbs, with the reason said once.
export const ReadOnly: Story = {
  render: story(TEAM, USERS, { canEdit: false }),
};

export const NoMembers: Story = {
  render: story({ ...TEAM, member_count: 0 }, [
    member(3, "Chiara Rossi", false),
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

// A name another team holds is refused on the field, in the reader's words.
export const RenameDuplicate: Story = {
  render: () => {
    installFetchStub({
      "PATCH /teams/t-1": () =>
        jsonResponse(
          {
            type: "about:blank",
            title: "Conflict",
            status: 409,
            code: "conflict",
            detail: 'conflict: a team named "Nord" already exists',
          },
          409,
        ),
    });
    return (
      <StoryProviders>
        <RenameTeamAction team={TEAM} />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", { name: "Rename" }),
    );
    const dialog = within(await within(document.body).findByRole("dialog"));
    const field = dialog.getByLabelText(en["users.teamNameLabel"]);
    await userEvent.clear(field);
    await userEvent.type(field, "Nord{Enter}");
    await expect(
      await dialog.findByText(en["users.teamDuplicate"]),
    ).toBeInTheDocument();
  },
};

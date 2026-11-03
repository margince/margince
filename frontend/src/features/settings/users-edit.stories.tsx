// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { UTC_ZONE } from "../../format/timezone";
import { meFixture } from "../../testing/mefixture";
import { SEEDED_ASSIGNABLE_ROLES } from "../../testing/roles.testkit";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { EditMember } from "./users-edit";
import type { User } from "./users-members";

const IMPORTED_MEMBER: User = {
  id: "10000000-0000-4000-8000-000000000001",
  display_name: "Robin Example",
  email: "robin@example.test",
  timezone: UTC_ZONE,
  status: "invited",
  email_editable: true,
  is_agent: false,
  roles: ["rep"],
  team_ids: [],
  updated_at: "2026-10-01T09:00:00Z",
  allowed_actions: ["edit", "change_role"],
};

const REGIONS = ["DACH", "Nordics", "Benelux", "UK & Ireland", "Iberia"];
const KINDS = ["Sales", "Delivery", "Customer Success", "Partners", "Presales"];
// Twenty-five teams: the size where a list of ticks stops fitting a dialog.
const MANY_TEAMS = REGIONS.flatMap((region, row) =>
  KINDS.map((kind, column) => ({
    id: `20000000-0000-4000-8000-0000000000${row}${column}`,
    name: `${region} ${kind}`,
    member_count: ((row + column) % 6) + 2,
  })),
);

function installMemberEditorRoutes(
  teams: readonly { id: string; name: string }[] = [
    { id: "20000000-0000-4000-8000-000000000001", name: "Delivery" },
  ],
) {
  installFetchStub({
    "GET /me": () =>
      jsonResponse(
        meFixture({
          roles: ["admin"],
          allow: {
            user_admin: ["read", "update", "delete"],
            team_admin: ["read", "update"],
          },
        }),
      ),
    "GET /teams": () =>
      jsonResponse({
        data: teams,
        page: { has_more: false },
      }),
    "GET /users/assignable-roles": () =>
      jsonResponse({ roles: SEEDED_ASSIGNABLE_ROLES }),
  });
}
const meta = {
  title: "Settings/People/Members/Edit member",
  component: EditMember,
  args: { member: IMPORTED_MEMBER, onClose: () => undefined },
} satisfies Meta<typeof EditMember>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Invited: Story = {
  render: () => {
    installMemberEditorRoutes();
    return (
      <StoryProviders>
        <EditMember member={IMPORTED_MEMBER} onClose={() => undefined} />
      </StoryProviders>
    );
  },
};
export const Deactivated: Story = {
  render: () => {
    installMemberEditorRoutes();
    return (
      <StoryProviders>
        <EditMember
          member={{ ...IMPORTED_MEMBER, status: "deactivated" }}
          onClose={() => undefined}
        />
      </StoryProviders>
    );
  },
};

export const ManyTeams: Story = {
  render: () => {
    installMemberEditorRoutes(MANY_TEAMS);
    return (
      <StoryProviders>
        <EditMember
          member={{
            ...IMPORTED_MEMBER,
            team_ids: [MANY_TEAMS[0]?.id ?? "", MANY_TEAMS[7]?.id ?? ""],
          }}
          onClose={() => undefined}
        />
      </StoryProviders>
    );
  },
};

export const NoTeamsYet: Story = {
  render: () => {
    installMemberEditorRoutes([]);
    return (
      <StoryProviders>
        <EditMember member={IMPORTED_MEMBER} onClose={() => undefined} />
      </StoryProviders>
    );
  },
};

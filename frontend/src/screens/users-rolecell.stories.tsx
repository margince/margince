// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { SEEDED_ASSIGNABLE_ROLES } from "./roles.testkit";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";
import { ChangeRoleDialog } from "./users-rolechange";

// The role column's one verb: the change-role dialog a row's menu opens. A
// seeded role, a custom role under the name its maker gave it, a member holding
// several roles (whose "Role today" names everything a pick replaces), a change
// in flight and a refused one.

const FIELD_SALES = {
  key: "custom_field_sales",
  name: "Field sales DACH",
  is_system: false,
};
const ROLES = [...SEEDED_ASSIGNABLE_ROLES, FIELD_SALES];

const GRANT = { create: true, read: true, update: true, delete: false };

function member(roles: string[]): components["schemas"]["User"] {
  return {
    id: "u-1",
    email: "dana@brandt.example",
    display_name: "Dana Kessler",
    timezone: "",
    status: "active",
    is_agent: false,
    roles,
    team_ids: [],
  };
}

function dialog(roles: string[], pending = false, error: string | null = null) {
  return () => {
    installFetchStub({
      "GET /users/access-preview": () =>
        jsonResponse({
          role: "rep",
          row_scope: "team",
          objects: { contact: GRANT, deal: GRANT },
          field_masks: [],
          teams: [],
        }),
    });
    return (
      <StoryProviders>
        <ChangeRoleDialog
          member={member(roles)}
          roles={ROLES}
          open
          pending={pending}
          error={error}
          onClose={() => undefined}
          onConfirm={() => undefined}
          returnFocusTo={() => null}
        />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof ChangeRoleDialog> = {
  title: "Settings/People/Members/Change role",
  component: ChangeRoleDialog,
};
export default meta;
type Story = StoryObj<typeof ChangeRoleDialog>;

export const SeededRole: Story = { render: dialog(["rep"]) };

export const SeededRoleDark: Story = {
  render: dialog(["rep"]),
  globals: { theme: "dark" },
};

export const CustomRole: Story = { render: dialog([FIELD_SALES.key]) };

export const SeveralRoles: Story = {
  render: dialog(["manager", FIELD_SALES.key]),
};

export const Applying: Story = { render: dialog(["rep"], true) };

export const RefusedDark: Story = {
  render: dialog(
    ["admin"],
    false,
    "Demoting this member would leave the company with no active admin.",
  ),
  globals: { theme: "dark" },
};

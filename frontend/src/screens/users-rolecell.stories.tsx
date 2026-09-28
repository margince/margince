// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { SEEDED_ASSIGNABLE_ROLES } from "./roles.testkit";
import { StoryProviders } from "./story-utils";
import { RoleCell } from "./users-rolecell";

// The member row's role picker on its own: a seeded role under its translated
// label, a custom role under the name its maker gave it, and a member holding
// several roles, whose placeholder names what a choice would replace.

const FIELD_SALES = {
  key: "custom_field_sales",
  name: "Field sales DACH",
  is_system: false,
};
const ROLES = [...SEEDED_ASSIGNABLE_ROLES, FIELD_SALES];

function member(roles: string[]): components["schemas"]["User"] {
  return {
    id: "u-1",
    email: "dana@brandt.example",
    display_name: "Dana Kessler",
    timezone: "",
    status: "active",
    is_agent: false,
    roles,
  };
}

function cell(roles: string[], pending = false) {
  return () => (
    <StoryProviders>
      <RoleCell
        member={member(roles)}
        roles={ROLES}
        pending={pending}
        onPick={() => undefined}
      />
    </StoryProviders>
  );
}

const meta: Meta<typeof RoleCell> = {
  title: "Settings/People/Members/Role picker",
  component: RoleCell,
};
export default meta;
type Story = StoryObj<typeof RoleCell>;

export const SeededRole: Story = { render: cell(["rep"]) };

export const CustomRole: Story = { render: cell([FIELD_SALES.key]) };

export const SeveralRoles: Story = {
  render: cell(["manager", FIELD_SALES.key]),
};

export const Applying: Story = { render: cell(["rep"], true) };

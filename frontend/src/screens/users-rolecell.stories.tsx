// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { StoryProviders } from "../testing/story-utils";
import { SEEDED_ASSIGNABLE_ROLES } from "./roles.testkit";
import { MemberRole, type RoleRefusal } from "./users-rolecell";

// The member row's role alone. Plain words at rest, a chooser only on press,
// and a refusal whose reason stays off the row.

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

function cell(roles: string[], refusal: RoleRefusal | null = null) {
  return () => (
    <StoryProviders>
      <MemberRole member={member(roles)} roles={ROLES} refusal={refusal} />
    </StoryProviders>
  );
}

const meta = {
  title: "Settings/People/Members/Role picker",
  component: MemberRole,
  args: { member: member(["rep"]), roles: ROLES, refusal: null },
} satisfies Meta<typeof MemberRole>;
export default meta;
type Story = StoryObj<typeof meta>;

export const SeededRole: Story = {
  render: cell(["rep"]),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: /Dana Kessler’s role/ }),
    );
    const body = within(canvasElement.ownerDocument.body);
    await expect(await body.findByRole("listbox")).toBeVisible();
  },
};

export const CustomRole: Story = { render: cell([FIELD_SALES.key]) };

// Any choice replaces the whole set, so the resting face names every held role.
export const SeveralRoles: Story = {
  render: cell(["manager", FIELD_SALES.key]),
};

export const NoRole: Story = { render: cell([]) };

// The last admin: plain words, the reason on hover and for a screen reader.
export const RefusedWithReason: Story = {
  render: cell(["admin"], {
    reason: "This is the only admin. Make another member an admin first.",
  }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByText("Admin");
    await expect(canvas.queryByRole("button")).toBeNull();
  },
};

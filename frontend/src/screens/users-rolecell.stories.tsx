// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { StoryProviders } from "../testing/story-utils";
import { SEEDED_ASSIGNABLE_ROLES } from "./roles.testkit";
import { MemberRole, type RoleRefusal } from "./users-rolecell";

// The member row's role alone. Plain words at rest, a chooser only on press,
// and a refusal that says its reason under the role.

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
    const held = await body.findByRole("option", { name: "User" });
    await expect(held).toHaveAttribute("aria-selected", "true");
  },
};

export const CustomRole: Story = { render: cell([FIELD_SALES.key]) };

// At rest the names, and a line saying a pick replaces them. Open, the held set
// comes first.
export const SeveralRoles: Story = {
  render: cell(["manager", FIELD_SALES.key]),
};

export const SeveralRolesChooser: Story = {
  render: cell(["manager", FIELD_SALES.key]),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: /Dana Kessler’s role/ }),
    );
    const body = within(canvasElement.ownerDocument.body);
    const held = await body.findByRole("option", {
      name: "Team lead, Field sales DACH",
    });
    await expect(held).toHaveAttribute("aria-selected", "true");
    await expect(
      await canvas.findByText(/replaces all current roles/, {
        selector: ".t-caption",
      }),
    ).toBeVisible();
  },
};

export const SeveralRolesPhone: Story = {
  ...SeveralRolesChooser,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

export const NoRole: Story = { render: cell([]) };

// The last admin: plain words, and the reason under them on every screen.
export const RefusedWithReason: Story = {
  render: cell(["admin"], {
    reason: "This is the only admin. Make another member an admin first.",
  }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByText("Admin");
    await expect(canvas.queryByRole("button")).toBeNull();
    await expect(canvas.getByText(/only admin/)).toBeVisible();
  },
};

export const RefusedWithReasonPhone: Story = {
  ...RefusedWithReason,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

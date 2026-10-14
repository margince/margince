// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { Role } from "./roles.queries";
import { RoleDetail, useRoleWrites } from "./roles-detail";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The body a role's drawer hosts, drawn without the drawer around it.

const ALL = { read: true, create: true, update: true, delete: true };
const READ = { read: true, create: false, update: false, delete: false };

const FIELD_SALES: Role = {
  key: "custom_field_sales",
  name: "Field sales",
  is_system: false,
  version: 7,
  row_scope: "team",
  objects: { contact: READ, deal: ALL, audit_log: READ, ext_gone_note: READ },
};

function Body({
  role,
  canUpdate,
}: Readonly<{ role: Role; canUpdate: boolean }>) {
  const writes = useRoleWrites();
  return (
    <RoleDetail
      role={role}
      directory={[role]}
      units={[]}
      canUpdate={canUpdate}
      canWiden={canUpdate}
      writes={writes}
    />
  );
}

function stubPreview(role: Role) {
  installFetchStub({
    "GET /users/access-preview": () =>
      jsonResponse({
        role: role.key,
        row_scope: role.row_scope,
        objects: role.objects,
        field_masks: [],
        teams: [],
      }),
  });
}

const meta = {
  title: "Settings/People/Roles and permissions/Role detail",
  component: Body,
  parameters: { layout: "padded" },
  args: { role: FIELD_SALES, canUpdate: true },
  render: (args) => {
    stubPreview(args.role);
    return (
      <StoryProviders>
        <Body {...args} />
      </StoryProviders>
    );
  },
} satisfies Meta<typeof Body>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Editable: Story = {};

export const Archived: Story = {
  args: { role: { ...FIELD_SALES, archived_at: "2026-09-01T09:00:00Z" } },
};

export const ReadOnly: Story = { args: { canUpdate: false } };

/** Each row with its raw key under the name, for an operator who asked. */
export const WithKeys: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("switch", { name: "Show API names" }),
    );
    await expect(
      canvas.getByRole("rowheader", { name: /Audit log/ }).textContent,
    ).toContain("audit_log");
  },
};

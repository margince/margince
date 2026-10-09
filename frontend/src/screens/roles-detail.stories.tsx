// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Role } from "./roles.queries";
import { RoleDetail } from "./roles-detail";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

const ALL = { read: true, create: true, update: true, delete: true };
const READ = { read: true, create: false, update: false, delete: false };

const FIELD_SALES: Role = {
  key: "custom_field_sales",
  name: "Field sales",
  is_system: false,
  version: 7,
  row_scope: "team",
  objects: { contact: READ, deal: ALL },
};

function story(role: Role, canUpdate: boolean) {
  return () => {
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
    return (
      <StoryProviders>
        <RoleDetail
          role={role}
          directory={[role]}
          units={[]}
          canUpdate={canUpdate}
          canMove={canUpdate}
          canRestore={canUpdate}
          canWiden={canUpdate}
        />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof RoleDetail> = {
  title: "Settings/People/Roles and permissions/Role detail",
  component: RoleDetail,
};
export default meta;
type Story = StoryObj<typeof RoleDetail>;

export const Editable: Story = { render: story(FIELD_SALES, true) };

export const Archived: Story = {
  render: story({ ...FIELD_SALES, archived_at: "2026-09-01T09:00:00Z" }, true),
};

export const ReadOnly: Story = { render: story(FIELD_SALES, false) };

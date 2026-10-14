// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { Role } from "./roles.queries";
import { RoleDrawer } from "./roles-drawer";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// One role open in its drawer. The head holds the name, the badges and the
// verb that archives or restores it; the body scrolls the editor.

const ALL = { read: true, create: true, update: true, delete: true };
const READ = { read: true, create: false, update: false, delete: false };
const NONE = { read: false, create: false, update: false, delete: false };

const FIELD_SALES: Role = {
  key: "custom_field_sales",
  name: "Field sales",
  is_system: false,
  version: 7,
  row_scope: "team",
  objects: {
    activity: READ,
    company: READ,
    contact: READ,
    deal: ALL,
    lead: ALL,
    product: READ,
    project: NONE,
    role_admin: NONE,
    ext_notes_note: NONE,
  },
};

const ADMIN: Role = {
  key: "admin",
  name: "Admin",
  is_system: true,
  version: 4,
  row_scope: "all",
  objects: { contact: ALL, deal: ALL, role_admin: ALL },
};

function stub(role: Role, refuse?: { status: number; code: string }) {
  installFetchStub({
    "GET /users/access-preview": () =>
      jsonResponse({
        role: role.key,
        row_scope: role.row_scope,
        objects: role.objects,
        field_masks: [],
        teams: [],
      }),
    [`PATCH /roles/${role.key}/objects/deal`]: () =>
      refuse
        ? jsonResponse({ title: "Refused", code: refuse.code }, refuse.status)
        : jsonResponse({ ...role, version: role.version + 1 }),
  });
}

const meta = {
  title: "Settings/People/Roles and permissions/Role drawer",
  component: RoleDrawer,
  args: {
    role: FIELD_SALES,
    open: true,
    onClose: () => undefined,
    returnFocusTo: () => null,
    directory: [FIELD_SALES, ADMIN],
    units: [],
    canUpdate: true,
    canMove: true,
    canRestore: true,
    canWiden: true,
  },
  render: (args) => {
    stub(args.role);
    return (
      <StoryProviders>
        <RoleDrawer {...args} />
      </StoryProviders>
    );
  },
} satisfies Meta<typeof RoleDrawer>;
export default meta;
type Story = StoryObj<typeof meta>;

/** A custom role: Archive in the head, every control live. */
export const Custom: Story = {};

/** An archived role: Restore in the head, the editor read-only. */
export const Archived: Story = {
  args: { role: { ...FIELD_SALES, archived_at: "2026-09-01T09:00:00Z" } },
};

/** A built-in role offers no archive. */
export const BuiltIn: Story = { args: { role: ADMIN } };

/** A reader holding only the read: no verb, no rename, every switch held. */
export const ReadOnly: Story = {
  args: {
    canUpdate: false,
    canMove: false,
    canRestore: false,
    canWiden: false,
  },
};

/** A refused write: the reason stands in the head, in view of the grid. */
export const Refused: Story = {
  render: (args) => {
    stub(args.role, { status: 409, code: "version_skew" });
    return (
      <StoryProviders>
        <RoleDrawer {...args} />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const body = within(canvasElement.ownerDocument.body);
    await userEvent.click(
      await body.findByRole("switch", {
        name: /allow field sales to delete deal/i,
      }),
    );
    await body.findByText(/someone else changed this role/i);
  },
};

export const Phone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

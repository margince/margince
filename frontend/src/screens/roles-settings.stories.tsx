// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { GrantSpec } from "../app/mefixture";
import { RolesSettings } from "./roles-settings";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// The role list with one role open beneath it: its name, whose records it
// reaches, the object grid with an extension section, and the preview.

const ALL = { read: true, create: true, update: true, delete: true };
const READ = { read: true, create: false, update: false, delete: false };
const NONE = { read: false, create: false, update: false, delete: false };

const ROLES = [
  {
    key: "admin",
    name: "Admin",
    is_system: true,
    version: 4,
    row_scope: "all",
    objects: { contact: ALL, deal: ALL, role_admin: ALL, ext_notes_note: ALL },
  },
  {
    key: "rep",
    name: "User",
    is_system: true,
    version: 3,
    row_scope: "own",
    objects: {
      contact: ALL,
      deal: ALL,
      role_admin: NONE,
      ext_notes_note: READ,
    },
  },
  {
    key: "custom_field_sales",
    name: "Field sales",
    is_system: false,
    version: 7,
    row_scope: "team",
    objects: {
      contact: READ,
      deal: ALL,
      role_admin: NONE,
      ext_notes_note: NONE,
    },
  },
];

// An archived custom role, and one whose name runs past a phone's line.
const MORE_ROLES = [
  ...ROLES,
  {
    key: "custom_partner_enablement",
    name: "Regional field sales and channel partner enablement, DACH and Benelux",
    is_system: false,
    version: 2,
    row_scope: "team",
    objects: { contact: READ, deal: READ },
  },
  {
    key: "custom_seasonal",
    name: "Seasonal temps",
    is_system: false,
    version: 5,
    row_scope: "own",
    archived_at: "2026-09-01T09:00:00Z",
    objects: { contact: READ, deal: NONE },
  },
];

function member(email: string, roles: string[]) {
  return {
    id: `id-${email}`,
    email,
    display_name: email,
    status: "active",
    is_agent: false,
    roles,
  };
}

const ROSTER = [
  member("ada@brandt.example", ["admin"]),
  member("bo@brandt.example", ["custom_field_sales"]),
  member("cy@brandt.example", ["custom_field_sales"]),
  member("di@brandt.example", ["rep"]),
];

const ROLE_ADMIN: GrantSpec = {
  role_admin: ["read", "create", "update", "delete"],
  user_admin: ["read"],
};

function story(
  allow: GrantSpec,
  roles: string[],
  directory: readonly unknown[] = ROLES,
) {
  return () => {
    installFetchStub({
      "GET /me": meRoute(allow, { roles }),
      "GET /roles": () => jsonResponse({ roles: directory }),
      "GET /users": () =>
        jsonResponse({ data: ROSTER, page: { has_more: false } }),
      "GET /users/access-preview": () =>
        jsonResponse({
          role: "custom_field_sales",
          row_scope: "team",
          objects: { contact: READ, deal: ALL },
          field_masks: [],
          teams: [],
        }),
    });
    return (
      <StoryProviders>
        <RolesSettings />
      </StoryProviders>
    );
  };
}

// Opens the custom role, so the detail is what the frame shows.
async function openFieldSales({
  canvasElement,
}: Readonly<{ canvasElement: HTMLElement }>) {
  const canvas = within(canvasElement);
  await userEvent.click(
    await canvas.findByRole("button", { name: "Field sales" }),
  );
}

const meta: Meta<typeof RolesSettings> = {
  title: "Settings/People/Roles and permissions/Roles",
  component: RolesSettings,
};
export default meta;
type Story = StoryObj<typeof RolesSettings>;

export const RoleList: Story = { render: story(ROLE_ADMIN, ["admin"]) };

export const RoleOpen: Story = {
  play: openFieldSales,
  render: story(ROLE_ADMIN, ["admin"]),
};

// Dark is where the grid's hairlines and the off and on switch tracks either
// stay apart or collapse into one grey.
export const RoleOpenDark: Story = {
  globals: { theme: "dark" },
  play: openFieldSales,
  render: story(ROLE_ADMIN, ["admin"]),
};

// A reader holding only the read: every switch legible and none pressable,
// no rename, no archive, no new role.
export const ReadOnly: Story = {
  play: openFieldSales,
  render: story({ role_admin: ["read"] }, ["ops"]),
};

export const NewRoleDialog: Story = {
  render: story(ROLE_ADMIN, ["admin"]),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "New role" }),
    );
    await within(document.body).findByRole("dialog");
  },
};

// Archived roles shown: the archived one keeps its type and says it is archived.
export const ArchivedShown: Story = {
  render: story(ROLE_ADMIN, ["admin"], MORE_ROLES),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("switch", { name: /show archived/i }),
    );
    await canvas.findByText("Seasonal temps");
  },
};

// A reader without the member grant: no roster read, so no counts.
export const WithoutMemberCounts: Story = {
  render: story({ role_admin: ["read"] }, ["ops"]),
};

// On a phone the name keeps the width and the count stands over the badge.
export const Phone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story(ROLE_ADMIN, ["admin"], MORE_ROLES),
};

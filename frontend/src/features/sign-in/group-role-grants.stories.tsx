// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { en } from "../../i18n/en";
import { SEEDED_ASSIGNABLE_ROLES } from "../../testing/roles.testkit";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "../../testing/story-utils";
import { GroupRoleGrants } from "./group-role-grants";

function story(
  initial: Record<string, string>,
  canManage = true,
  routes: RouteMap = {},
) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({ authentication_policy: ["read", "update"] }),
      "GET /users/assignable-roles": () =>
        jsonResponse({ roles: SEEDED_ASSIGNABLE_ROLES }),
      ...routes,
    });
    return (
      <StoryProviders>
        <GroupRoleGrants
          initial={initial}
          canManage={canManage}
          lock={{ disabled: false }}
        />
      </StoryProviders>
    );
  };
}

const MAPPED = { "crm-admins": "admin", "field-sales": "rep" };

const meta = {
  title: "Settings/Company/Sign-in and apps/Group role grants",
  component: GroupRoleGrants,
  parameters: { layout: "padded" },
  args: { initial: {}, canManage: true, lock: { disabled: false } },
} satisfies Meta<typeof GroupRoleGrants>;
export default meta;
type Story = StoryObj<typeof meta>;

// No mapping: corporate sign-in grants nothing beyond what a member holds.
export const Empty: Story = { render: story({}) };

export const MappedDark: Story = {
  render: story(MAPPED),
  globals: { theme: "dark" },
};

// A reader who may not grant roles sees the map and no verbs.
export const ReadOnly: Story = { render: story(MAPPED, false) };

// A drafted group that repeats a mapped one: Save refuses and says why.
export const DuplicateRefused: Story = {
  render: story(MAPPED),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const add = await canvas.findByRole("button", { name: /add group/i });
    await waitFor(() => expect(add).toBeEnabled());
    await userEvent.click(add);
    const groups = canvas.getAllByRole("textbox", { name: "IdP group" });
    const drafted = groups[groups.length - 1];
    if (!drafted) throw new Error("the drafted row has no group field");
    await userEvent.type(drafted, "crm-admins");
    await canvas.findByText(en["groupRoles.duplicateGroup"]);
  },
};

// The server refused the save: the panel says so above the rows.
export const SaveRefused: Story = {
  render: story(MAPPED, true, {
    "PATCH /installation/settings": () =>
      new Response(
        JSON.stringify({ status: 503, code: "service_unavailable" }),
        {
          status: 503,
          headers: { "content-type": "application/problem+json" },
        },
      ),
  }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const add = await canvas.findByRole("button", { name: /add group/i });
    await waitFor(() => expect(add).toBeEnabled());
    await userEvent.click(add);
    const groups = canvas.getAllByRole("textbox", { name: "IdP group" });
    const drafted = groups[groups.length - 1];
    if (!drafted) throw new Error("the drafted row has no group field");
    await userEvent.type(drafted, "partners");
    await userEvent.click(
      canvas.getByRole("button", { name: /save group grants/i }),
    );
    await canvas.findByText(en["signInMethods.saveFailed"]);
  },
};

export const SaveRefusedPhone: Story = {
  ...SaveRefused,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

/** A new Admin mapping asks for the typed phrase before it is saved. */
export const AdminConfirm: Story = {
  render: story({ "crm-admins": "admin", Sales: "rep" }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("combobox", { name: "Role for the Sales group" }),
    );
    await userEvent.click(
      await within(canvasElement.ownerDocument.body).findByRole("option", {
        name: "Admin",
      }),
    );
    await userEvent.click(
      canvas.getByRole("button", { name: "Save group roles" }),
    );
    await within(canvasElement.ownerDocument.body).findByRole("dialog");
  },
};

export const AdminConfirmDark: Story = {
  ...AdminConfirm,
  globals: { theme: "dark" },
};

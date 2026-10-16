// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { GroupRoleGrants } from "./group-role-grants";
import { SEEDED_ASSIGNABLE_ROLES } from "./roles.testkit";

function story(initial: Record<string, string>, canManage = true) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({ authentication_policy: ["read", "update"] }),
      "GET /users/assignable-roles": () =>
        jsonResponse({ roles: SEEDED_ASSIGNABLE_ROLES }),
    });
    return (
      <StoryProviders>
        <GroupRoleGrants initial={initial} canManage={canManage} />
      </StoryProviders>
    );
  };
}

const MAPPED = { "crm-admins": "admin", "field-sales": "rep" };

const meta = {
  title: "Settings/Company/Sign-in and apps/Group role grants",
  component: GroupRoleGrants,
  parameters: { layout: "padded" },
  args: { initial: {}, canManage: true },
} satisfies Meta<typeof GroupRoleGrants>;
export default meta;
type Story = StoryObj<typeof meta>;

// No mapping: corporate sign-in grants nothing beyond what a member holds.
export const Empty: Story = { render: story({}) };

// Mapped groups, with the grant-only warning above the rows.
export const Mapped: Story = { render: story(MAPPED) };

export const MappedDark: Story = {
  render: story(MAPPED),
  globals: { theme: "dark" },
};

// On a phone each grant folds: the group over its role, the remove verb at the
// end of the group's line.
export const MappedPhone: Story = {
  render: story({
    "crm-admins": "admin",
    "sales-dach-benelux-regional-field-and-channel-partner-enablement": "rep",
  }),
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

// A reader who may not grant roles sees the map and no verbs.
export const ReadOnly: Story = { render: story(MAPPED, false) };

// The first drafted row brings the warning, and Save stands while the draft
// differs from what is stored.
export const Draft: Story = {
  render: story({}),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const add = await canvas.findByRole("button", { name: /add group/i });
    await waitFor(() => expect(add).toBeEnabled());
    await userEvent.click(add);
    await canvas.findByText(/grants the mapped roles and never removes any/i);
    await canvas.findByRole("button", { name: /save group grants/i });
  },
};

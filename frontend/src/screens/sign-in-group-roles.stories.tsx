// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent, within } from "storybook/test";
import { SEEDED_ASSIGNABLE_ROLES } from "./roles.testkit";
import { GroupRoleGrants } from "./sign-in-group-roles";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// Directory groups that hand a role to the members invited from them, behind a
// disclosure that stays closed until somebody asks for it.

const meta: Meta<typeof GroupRoleGrants> = {
  title: "Settings/Company/Sign-in and apps/Group roles",
  component: GroupRoleGrants,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof GroupRoleGrants>;

const MAPPED = { Sales: "rep", "Sales leads": "manager" };

function served(map: Record<string, string>, canManage = true) {
  installFetchStub({
    "GET /users/assignable-roles": () =>
      jsonResponse({ roles: SEEDED_ASSIGNABLE_ROLES }),
  });
  return (
    <StoryProviders>
      <GroupRoleGrants
        initial={map}
        canManage={canManage}
        // The card that holds the editor says a refusal; see Sign-in methods.
        onRefusal={() => undefined}
      />
    </StoryProviders>
  );
}

async function openAdvanced(canvasElement: HTMLElement) {
  await userEvent.click(
    await within(canvasElement).findByText(
      "Advanced: roles from directory groups",
    ),
  );
}

/** Closed by default, with the count in its summary. */
export const Closed: Story = {
  render: () => served(MAPPED),
};

/** Opened: the map as a table, saved whole. */
export const Mapped: Story = {
  render: () => served(MAPPED),
  play: async ({ canvasElement }) => {
    await openAdvanced(canvasElement);
    await within(canvasElement).findByDisplayValue("Sales leads");
  },
};

export const MappedDark: Story = {
  ...Mapped,
  globals: { theme: "dark" },
};

/** A new Admin mapping asks for the typed phrase before it is saved. */
export const AdminConfirm: Story = {
  render: () => served({ "crm-admins": "admin", Sales: "rep" }),
  play: async ({ canvasElement }) => {
    await openAdvanced(canvasElement);
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("combobox", { name: "Role for the Sales group" }),
    );
    await userEvent.click(await screen.findByRole("option", { name: "Admin" }));
    await userEvent.click(
      canvas.getByRole("button", { name: "Save group roles" }),
    );
    await screen.findByRole("dialog");
  },
};

export const AdminConfirmDark: Story = {
  ...AdminConfirm,
  globals: { theme: "dark" },
};

/** A reader who may not grant roles sees the map, locked, with the reason. */
export const ReadOnly: Story = {
  render: () => served(MAPPED, false),
  play: Mapped.play,
};

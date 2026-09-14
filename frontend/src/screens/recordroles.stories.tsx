// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { RecordRolesCard } from "./recordroles";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// Settings › the responsibilities somebody can hold on a record.
//
// Two states are worth seeing beyond admin-versus-reader. The RETIRED role
// stays in the list with its switch off, because assignments still carry it
// and a vocabulary that hid it would leave those rows naming something the
// settings page says does not exist. And the applicability line beside each
// role is read-only: narrowing it is refused while a live assignment depends
// on what would go, so the page shows the rule rather than offering a control
// whose save usually fails.

function role(
  key: string,
  label: string,
  recordTypes: string[],
  assigneeKinds: string[],
  extra: Record<string, unknown> = {},
) {
  return {
    id: `role-${key}`,
    key,
    label,
    record_types: recordTypes,
    assignee_kinds: assigneeKinds,
    sort_order: 10,
    active: true,
    system: true,
    version: 1,
    created_at: "2026-08-01T08:00:00Z",
    updated_at: "2026-08-01T08:00:00Z",
    ...extra,
  };
}

const ROLES = {
  data: [
    role(
      "account_manager",
      "Account manager",
      ["company", "deal", "project"],
      ["user"],
    ),
    role("sales_engineer", "Sales engineer", ["deal"], ["user"]),
    role("delivery_lead", "Delivery lead", ["project"], ["user"]),
    role(
      "technical_contact",
      "Technical contact",
      ["company", "deal", "project"],
      ["user", "team"],
    ),
    role("support_team", "Support team", ["company", "project"], ["team"]),
    // Added by this workspace and since withdrawn. Still listed, because
    // assignments made while it was live still carry it.
    role("launch_buddy", "Launch buddy", ["project"], ["user"], {
      system: false,
      active: false,
    }),
  ],
};

const ADMIN = { custom_field: ["read", "create", "update", "delete"] } as const;
const READER = { custom_field: ["read"] } as const;

function story(allow: Parameters<typeof meRoute>[0]) {
  return () => {
    installFetchStub({
      "GET /me": meRoute(allow),
      "GET /record-roles": () => jsonResponse(ROLES),
    });
    return (
      <StoryProviders>
        <RecordRolesCard />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof RecordRolesCard> = {
  title: "Settings/Sales/Responsibility roles/Roles",
  component: RecordRolesCard,
};
export default meta;
type Story = StoryObj<typeof RecordRolesCard>;

export const Admin: Story = { render: story(ADMIN) };

// Every control refused, the list still readable: the page is a report to a
// holder who may not change it, not a wall.
export const Reader: Story = { render: story(READER) };

export const AdminDark: Story = {
  globals: { theme: "dark" },
  render: story(ADMIN),
};

// The add form is a dialog behind the header verb rather than a row under the
// list, where its own label would have read as one of the roles.
export const AddingRole: Story = {
  render: story(ADMIN),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Add role" }),
    );
  },
};

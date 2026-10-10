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
// The retired role stays listed with its switch off, because assignments still
// carry it. Applies to is read-only: the server refuses narrowing a live role.

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

const DUPLICATE = {
  type: "about:blank",
  title: "Conflict",
  status: 409,
  code: "conflict",
  detail: "conflict",
};

function story(allow: Parameters<typeof meRoute>[0], refuseName = false) {
  return () => {
    installFetchStub({
      "GET /me": meRoute(allow),
      "GET /record-roles": () => jsonResponse(ROLES),
      ...(refuseName && {
        "POST /record-roles": () => jsonResponse(DUPLICATE, 409),
      }),
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

// At 390 each row folds: name and key over where it applies and the switch.
export const AdminPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story(ADMIN),
};

export const RenamingRole: Story = {
  render: story(ADMIN),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Actions for Sales engineer" }),
    );
    const page = within(canvasElement.ownerDocument.body);
    await userEvent.click(page.getByRole("button", { name: "Rename" }));
    await page.findByRole("dialog");
  },
};

// The server refuses a name whose key is taken: the dialog says so on the field.
export const DuplicateRole: Story = {
  render: story(ADMIN, true),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Add role" }),
    );
    const page = within(canvasElement.ownerDocument.body);
    const dialog = within(await page.findByRole("dialog"));
    await userEvent.type(dialog.getByLabelText("Name"), "Sales engineer");
    await userEvent.click(dialog.getByRole("checkbox", { name: "Deal" }));
    await userEvent.click(dialog.getByRole("checkbox", { name: "Colleague" }));
    await userEvent.click(dialog.getByRole("button", { name: "Add role" }));
    await dialog.findByText(
      "A role with this name already exists. Choose another name.",
    );
  },
};

// The press both add-form frames make, named rather than inherited.
//
// A story that picks up its `play` through a spread of another story is indexed
// WITHOUT the `play-fn` tag — the indexer reads the object literal in front of
// it, not what the spread resolves to — and the capture gate keys its settle on
// that tag. The dark frame was therefore screenshotted 250ms after paint rather
// than 1.5s, which is before the dialog this story is named for has opened.
const openTheAddForm: Story["play"] = async ({ canvasElement }) => {
  const canvas = within(canvasElement);
  await userEvent.click(
    await canvas.findByRole("button", { name: "Add role" }),
  );
};

// The add form is a dialog behind the header verb rather than a row under the
// list, where its own label would have read as one of the roles.
export const AddingRole: Story = {
  render: story(ADMIN),
  play: openTheAddForm,
};

export const AddingRoleDark: Story = {
  render: story(ADMIN),
  play: openTheAddForm,
  globals: { theme: "dark" },
};

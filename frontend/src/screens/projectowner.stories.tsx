// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent, within } from "storybook/test";
import { AssignProjectOwnerAction } from "./projectowner";
import type { Project } from "./projects.form";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The one path that hands a project directly to a NAMED colleague, via the
// server's existing owner_id field rather than the bulk transfer endpoint.
// The state worth looking at is the open list: the workspace's colleagues by
// name, the current owner marked, narrowed as the reader types.

const meta: Meta = {
  title: "Records/Project 360/Assign to a colleague",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const project: Project = {
  id: "proj-1",
  name: "Pallet Handling Programme",
  phase: "delivering",
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
  version: 5,
  owner_id: null,
};

function Action({ owner = null }: Readonly<{ owner?: string | null }>) {
  installFetchStub({
    "GET /users": () =>
      jsonResponse({
        data: [
          { id: "u-42", display_name: "Jane Doe", email: "jane@example.test" },
          {
            id: "u-7",
            display_name: "Omar Haddad",
            email: "omar@example.test",
          },
          {
            id: "u-9",
            display_name: "Lena Brandt",
            email: "lena@example.test",
          },
        ],
        page: { has_more: false },
      }),
    "PATCH /projects/proj-1": (body) =>
      jsonResponse({ ...project, ...(body as object), version: 6 }),
  });
  return (
    <StoryProviders>
      <AssignProjectOwnerAction project={{ ...project, owner_id: owner }} />
    </StoryProviders>
  );
}

/** The trigger as it sits beside Archive and Share — closed. */
export const Default: Story = { render: () => <Action /> };

async function openList(canvasElement: HTMLElement) {
  await userEvent.click(
    await within(canvasElement).findByRole("button", {
      name: "Assign to a colleague",
    }),
  );
  return within(await screen.findByRole("dialog"));
}

/** Opened: the colleagues by name, the current owner marked. */
export const OpenOnTheRoster: Story = {
  render: () => <Action owner="u-7" />,
  play: async ({ canvasElement }) => {
    const panel = await openList(canvasElement);
    await panel.findByRole("option", { name: "Omar Haddad" });
  },
};

/** Opened and narrowed to the named colleague. */
export const SearchingForAColleague: Story = {
  render: () => <Action />,
  play: async ({ canvasElement }) => {
    const panel = await openList(canvasElement);
    await userEvent.type(
      panel.getByRole("combobox", { name: "Search colleagues" }),
      "Jane",
    );
    await panel.findByRole("option", { name: "Jane Doe" });
  },
};

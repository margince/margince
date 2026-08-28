// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { AssignProjectOwnerAction } from "./projectowner";
import type { Project } from "./projects.form";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The one path that hands a project directly to a NAMED colleague, via the
// server's existing owner_id field rather than the bulk transfer endpoint.
// The state worth looking at is the search itself: a candidate found by a
// real name, not a fixed three-entry dropdown.

const meta: Meta = {
  title: "Records/Project 360/Assign to a colleague",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const project = {
  id: "proj-1",
  name: "Pallet Handling Programme",
  version: 5,
  owner_id: null,
} as unknown as Project;

function Action() {
  installFetchStub({
    "GET /users": () =>
      jsonResponse({
        data: [
          { id: "u-42", display_name: "Jane Doe", email: "jane@example.test" },
        ],
        page: { has_more: false },
      }),
    "PATCH /projects/proj-1": (body) =>
      jsonResponse({ ...project, ...(body as object), version: 6 }),
  });
  return (
    <StoryProviders>
      <AssignProjectOwnerAction project={project} />
    </StoryProviders>
  );
}

/** The trigger as it sits beside Archive and Share — closed. */
export const Default: Story = { render: () => <Action /> };

/** Opened, searched, and the named colleague found. */
export const SearchingForAColleague: Story = {
  render: () => <Action />,
  play: async ({ canvasElement }) => {
    const body = within(canvasElement.ownerDocument.body);
    await userEvent.click(await body.findByTestId("assign-project-owner"));
    await userEvent.type(
      await body.findByRole("searchbox", { name: "Search colleagues" }),
      "Jane",
    );
    await body.findByRole("button", { name: "Jane Doe" });
  },
};

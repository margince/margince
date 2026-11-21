// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import { COMPANY, project, project360 } from "../../testing/projects.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ProjectDetails, ProjectSubtitle } from "./projectdetails";
import { deliveringProject360 } from "./projectstanding.fixtures";

// The project's details column (About, Ownership, the closing rule) and the
// lines under its name. A date nobody set and an account the grant withholds
// are both stated, never dropped.

const meta: Meta = {
  title: "Records/Project 360/Details",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;

// The roster and the company name resolve through the reads every EntityRef
// uses; without them the frames would show the unresolved fallbacks.
function installReferenceReads(grant: ("read" | "update")[] = ["read"]) {
  installFetchStub({
    "GET /me": meRoute({ project: grant }),
    "GET /users": () =>
      jsonResponse({
        data: [
          {
            id: "u-me",
            display_name: "Lena Fischer",
            email: "lena@example.com",
          },
        ],
        page: { next_cursor: null },
      }),
    "GET /companies": () =>
      jsonResponse({
        data: [{ id: COMPANY.id, display_name: COMPANY.display_name }],
        page: { next_cursor: null },
      }),
  });
}

function column(
  view: ReturnType<typeof project360>,
  grant?: ("read" | "update")[],
) {
  return () => {
    installReferenceReads(grant);
    return (
      <StoryProviders>
        <div className="record-aside">
          <div className="record-split-lines">
            <ProjectSubtitle view={view} />
          </div>
          <ProjectDetails view={view} />
        </div>
      </StoryProviders>
    );
  };
}

/** A project in delivery: Day 166 of 275 under the name, every fact set. */
export const Delivering: Story = { render: column(deliveringProject360()) };

/** No dates set and the account withheld: both said, neither dropped. */
export const UnsetAndWithheld: Story = {
  render: column(
    project360({
      sections_omitted: ["company"],
      company: undefined,
      project: project({ company_id: null, owner_id: null }),
    }),
  ),
};

// A reader who may change the project gets one Edit per section, and Escape
// hands focus back to it.
export const SectionEdits: Story = {
  render: column(deliveringProject360(), ["read", "update"]),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const user = userEvent.setup();
    await expect(
      await canvas.findByRole("button", { name: "Edit Ownership" }),
    ).toBeVisible();
    const about = canvas.getByRole("button", { name: "Edit About" });
    await user.click(about);
    const dialog = await screen.findByRole("dialog", { name: "About" });
    await waitFor(() => expect(dialog).toBeVisible());
    await expect(
      within(dialog).getByRole("textbox", { name: "Description" }),
    ).toBeVisible();
    await expect(
      within(dialog).queryByRole("textbox", { name: /^Project name/ }),
    ).toBeNull();
    await user.keyboard("{Escape}");
    await waitFor(() => expect(about).toHaveFocus());
  },
};

export const SectionEditsDark: Story = {
  ...SectionEdits,
  globals: { theme: "dark" },
};

export const SectionEditsPhone: Story = {
  ...SectionEdits,
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

// A reader without the write grant reads both sections and is offered no Edit.
export const SectionEditsReadOnly: Story = {
  render: column(deliveringProject360()),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByText("Ownership");
    await expect(canvas.queryByRole("button", { name: /^Edit / })).toBeNull();
  },
};

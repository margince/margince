// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { COMPANY, project, project360 } from "../../testing/projects.fixtures";
import {
  installFetchStub,
  jsonResponse,
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
function installReferenceReads(): void {
  installFetchStub({
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

function column(view: ReturnType<typeof project360>) {
  return () => {
    installReferenceReads();
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

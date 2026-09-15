// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { project360 } from "./projects.fixtures";
import { ProjectDealsCard, StakeholdersCard } from "./projectsections";
import { installFetchStub, meRoute, StoryProviders } from "./story-utils";

// The project page's cards that carry a badge per row: a deal's status (won in
// the success tone, anything else in the default) and a stakeholder's seat.
// The whole page is `Records/Project/Screen`; this file draws the two cards on
// their own so the rows can be read without the rest of the page around them.
//
// The 360 comes from `projects.fixtures.ts`, the one the project suites build
// from, so these rows cannot drift from what the page itself renders.

const VIEW = project360({
  deals: {
    data: [
      {
        id: "d-1",
        name: "Phase one licence",
        pipeline_id: "pl",
        stage_id: "s3",
        status: "won",
        amount_minor: 450_000,
        currency: "EUR",
        source: "manual",
        captured_by: "u-me",
        created_at: "2026-06-02T09:00:00Z",
        updated_at: "2026-06-02T09:00:00Z",
      },
      {
        id: "d-2",
        name: "Phase two services",
        pipeline_id: "pl",
        stage_id: "s1",
        status: "open",
        amount_minor: 1_200_000,
        currency: "EUR",
        source: "manual",
        captured_by: "u-me",
        created_at: "2026-07-02T09:00:00Z",
        updated_at: "2026-07-02T09:00:00Z",
      },
    ],
    page: { next_cursor: null, has_more: false },
  },
  stakeholders: {
    data: [
      {
        relationship_id: "rel-1",
        contact_id: "p-1",
        contact_name: "Anna Weber",
        role: "project_lead",
      },
      {
        relationship_id: "rel-2",
        contact_id: "p-2",
        contact_name: "Jonas Brandt",
        role: "sponsor",
      },
    ],
    page: { next_cursor: null, has_more: false },
  },
});

const meta: Meta = {
  title: "Records/Project/Sections",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;

/** A won deal beside an open one, each status as its own badge. */
export const Deals: Story = {
  render: () => {
    installFetchStub({ "GET /me": meRoute({ deal: ["read"] }) });
    return (
      <StoryProviders>
        <ProjectDealsCard view={VIEW} />
      </StoryProviders>
    );
  },
};

/** The seats on the project, read-only so the rows carry only the badge. */
export const Stakeholders: Story = {
  render: () => {
    installFetchStub({ "GET /me": meRoute({ project: ["read"] }) });
    return (
      <StoryProviders>
        <StakeholdersCard view={VIEW} projectId="pr-1" readOnly />
      </StoryProviders>
    );
  },
};

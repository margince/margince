// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { viewerZone } from "../format/timezone";
import {
  ProjectCommitmentsTable,
  ProjectsByPhaseTable,
  ProjectsGoneQuietTable,
} from "./analytics.delivery";
import { StoryProviders } from "./story-utils";

// The delivery section's three tables on their own: projects per phase with
// their count drawn as a bar, each project's promises, and the projects nobody
// has spoken into lately — with the empty answer each one gives in words.
const meta: Meta = { title: "Records/Reports/Delivery tables" };
export default meta;

type Story = StoryObj;

export const ProjectsByPhase: Story = {
  render: () => (
    <StoryProviders>
      <ProjectsByPhaseTable
        locale="en"
        baseCurrency="EUR"
        rows={[
          {
            phase: "planning",
            projects: 2,
            open_deal_value_minor: 18_000_00,
            won_deal_value_minor: 0,
          },
          {
            phase: "delivering",
            projects: 5,
            open_deal_value_minor: 4_000_00,
            won_deal_value_minor: 96_000_00,
          },
        ]}
      />
    </StoryProviders>
  ),
};

export const Commitments: Story = {
  render: () => (
    <StoryProviders>
      <ProjectCommitmentsTable
        locale="en"
        rows={[
          {
            project_id: "prj-1",
            name: "Rollout Nord",
            phase: "delivering",
            open_commitments: 5,
            overdue_commitments: 2,
          },
        ]}
      />
    </StoryProviders>
  ),
};

// Nothing has gone quiet, which is the good answer and says so rather than
// drawing headers over blank space.
export const NothingGoneQuiet: Story = {
  render: () => (
    <StoryProviders>
      <ProjectsGoneQuietTable locale="en" timezone={viewerZone()} rows={[]} />
    </StoryProviders>
  ),
};

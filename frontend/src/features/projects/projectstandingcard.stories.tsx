// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { project, project360 } from "../../testing/projects.fixtures";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ProjectNext } from "./projectnext";
import { deliveringProject360 } from "./projectstanding.fixtures";
import { ProjectStanding } from "./projectstandingcard";

// The top of a project's Overview: the next task, then How it stands. The
// word is a user's health reading, so a project nobody judged reads "Not
// assessed" in neutral, never "On track".

const meta: Meta<typeof ProjectStanding> = {
  title: "Records/Project 360/How it stands",
  component: ProjectStanding,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof ProjectStanding>;

function overview(view: ReturnType<typeof project360>) {
  return () => {
    installFetchStub({ "GET /me": meRoute({ project: ["read", "update"] }) });
    return (
      <StoryProviders>
        <div className="record-stack">
          <ProjectNext view={view} onOpenTask={() => {}} />
          <ProjectStanding
            view={view}
            onMove={() => {}}
            onOpenDocuments={() => {}}
          />
        </div>
      </StoryProviders>
    );
  };
}

/** At risk mid-delivery: a late task next, Day 166 of 275, one deal won of
 *  two, a quiet week before today on the axis. */
export const AtRisk: Story = { render: overview(deliveringProject360()) };

/** Nobody has recorded a reading: the word is "Not assessed" and the ladder's
 *  current bar is neutral. Nothing is due, so the next bar says so. */
export const NotAssessed: Story = { render: overview(project360()) };

/** On track, with no note: the sentence says who judged it and when. */
export const OnTrack: Story = {
  render: overview(
    deliveringProject360({
      health: {
        current: {
          id: "h-2",
          project_id: "pr-1",
          state: "on_track",
          assessed_at: "2026-08-12T09:00:00Z",
          source: "human",
          source_author: "Lena Fischer",
          created_at: "2026-08-12T09:00:00Z",
          superseded: false,
        },
      },
    }),
  ),
};

/** A closed project: Closed is the way out taken, and no target end is ahead. */
export const Closed: Story = {
  render: overview(
    deliveringProject360({
      project: project({
        name: "Depot rollout",
        phase: "closed",
        started_at: "2026-03-01",
        target_end_date: "2026-11-30",
      }),
    }),
  ),
};

/** The health reading withheld from this reader's role. */
export const Withheld: Story = {
  render: overview(
    deliveringProject360({ sections_omitted: ["health"], health: undefined }),
  ),
};

export const AtRiskDark: Story = {
  globals: { theme: "dark" },
  render: overview(deliveringProject360()),
};

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { ProjectsSection } from "./companyrailprojects";
import { installFetchStub, meRoute, StoryProviders } from "./story-utils";

// The rail's projects section on its own: the account's work in flight, each
// row naming the project, where it stands and who holds it. It sits under the
// deals section, so it is read in the same narrow column and against the same
// card face — which is why both frames below are drawn at the rail's width.
//
// The 360 already orders the rows work-in-motion first, so nothing here sorts
// them a second time; what the frames differ in is whether the account HAS
// projects, because the empty arm carries the section's one verb.

type View = components["schemas"]["Company360"];
type Project = components["schemas"]["Company360Project"];

const company: components["schemas"]["Company"] = {
  id: "o-1",
  display_name: "Brandt Automotive GmbH",
  source: "manual",
  captured_by: "human:u1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

const projects: Project[] = [
  {
    project_id: "pr-1",
    name: "Line 4 retrofit",
    phase: "delivering",
    owner_name: "Mira Voss",
    quiet: false,
    last_activity_at: "2026-08-29T15:00:00Z",
  },
  {
    project_id: "pr-2",
    name: "Depot charging study",
    phase: "pursuing",
    owner_name: "Jan Keller",
    quiet: false,
  },
  {
    project_id: "pr-3",
    name: "Spare-parts portal",
    phase: "initiative",
    quiet: true,
  },
];

function view(data: Project[]): View {
  return {
    as_of: "2026-09-01T09:00:00Z",
    company,
    sections_omitted: [],
    projects: data,
    projects_page: { has_more: false, next_cursor: null },
  };
}

function Section({ data }: Readonly<{ data: View }>) {
  installFetchStub({ "GET /me": meRoute({ company: ["read"] }) });
  return (
    <StoryProviders>
      <div style={{ maxWidth: 340 }}>
        <ProjectsSection view={data} loading={false} onTab={() => {}} />
      </div>
    </StoryProviders>
  );
}

const meta: Meta = {
  title: "Records/Company rail/Projects",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

// Three deliveries in flight, counted in the summary because the page is the
// whole set, with the way to the Deals tab under them.
export const InFlight: Story = {
  render: () => <Section data={view(projects)} />,
};

// An account nobody has opened a project against: the section says so and
// offers the tab where one is started, on the same margin as the sentence.
export const NoProjects: Story = {
  render: () => <Section data={view([])} />,
};

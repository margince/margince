// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { CompanyProjectsPanel } from "./companyprojects";

type View = components["schemas"]["Company360"];
type Project = components["schemas"]["Company360Project"];

const projects: Project[] = [
  {
    project_id: "pr-1",
    name: "Line 4 retrofit",
    key: "RET-4",
    phase: "delivering",
    quiet: false,
  },
  {
    project_id: "pr-2",
    name: "Depot charging study",
    key: "DEP-12",
    phase: "pursuing",
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
    company: {
      id: "o-1",
      display_name: "Brandt Automotive GmbH",
      source: "manual",
      captured_by: "human:u1",
      created_at: "2026-06-01T08:00:00Z",
      updated_at: "2026-06-01T08:00:00Z",
    },
    sections_omitted: [],
    projects: data,
    projects_page: { has_more: false, next_cursor: null },
  };
}

function Panel({
  data,
  allow,
}: Readonly<{
  data: View;
  allow: Parameters<typeof meRoute>[0];
}>) {
  installFetchStub({ "GET /me": meRoute(allow) });
  return (
    <StoryProviders>
      <div style={{ maxWidth: 640 }}>
        <CompanyProjectsPanel companyId="o-1" view={data} />
      </div>
    </StoryProviders>
  );
}

const meta: Meta = {
  title: "Records/Company 360/Projects",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

export const WithProjects: Story = {
  render: () => (
    <Panel data={view(projects)} allow={{ project: ["read", "create"] }} />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(await canvas.findByText("Line 4 retrofit")).toBeTruthy();
    await expect(
      await canvas.findByRole("button", { name: /New project/ }),
    ).toBeTruthy();
  },
};

export const Empty: Story = {
  render: () => (
    <Panel data={view([])} allow={{ project: ["read", "create"] }} />
  ),
};

export const ReaderMayNotCreate: Story = {
  render: () => <Panel data={view(projects)} allow={{ project: ["read"] }} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(await canvas.findByText("Line 4 retrofit")).toBeTruthy();
    await expect(
      canvas.queryByRole("button", { name: /New project/ }),
    ).toBeNull();
  },
};

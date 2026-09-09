// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { ProjectsScreen } from "./projects";
import { ORG, project } from "./projects.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// The projects list in its two readings — rows, and the instructional
// first-run plate. The page a row leads to is the other screen, and its
// stories live beside it in `project360.stories.tsx`.

const meta: Meta<typeof ProjectsScreen> = {
  title: "Records/Projects",
  component: ProjectsScreen,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof ProjectsScreen>;

const rows = [
  project({ id: "pr-1", name: "CRM rollout", key: "ACME-CRM" }),
  project({
    id: "pr-2",
    name: "Fleet retrofit",
    key: "FLEET",
    phase: "delivering",
    last_activity_at: "2026-07-20T09:00:00Z",
  }),
  project({
    id: "pr-3",
    name: "Warehouse pilot",
    key: null,
    phase: "closed",
    owner_id: null,
    last_activity_at: null,
  }),
];

// The list itself plus the directory reads its cells resolve names through: an
// unrouted path answers with an empty page, so a missing route here reads as a
// row whose owner and company are simply blank.
function installList(projects: unknown[]) {
  installFetchStub({
    "GET /me": meRoute({}),
    "GET /projects": () =>
      jsonResponse({
        data: projects,
        page: { next_cursor: null, has_more: false },
      }),
    "GET /organizations": () =>
      jsonResponse({ data: [ORG], page: { next_cursor: null } }),
    [`GET /organizations/${ORG.id}`]: () => jsonResponse(ORG),
    "GET /users": () =>
      jsonResponse({
        data: [{ id: "u-me", display_name: "Me", status: "active" }],
        page: { next_cursor: null },
      }),
  });
}

export const List: Story = {
  render: () => {
    installList(rows);
    return (
      <StoryProviders>
        <ProjectsScreen />
      </StoryProviders>
    );
  },
};

export const FirstRun: Story = {
  render: () => {
    installList([]);
    return (
      <StoryProviders>
        <ProjectsScreen />
      </StoryProviders>
    );
  },
};

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, within } from "storybook/test";
import { phoneRow } from "../../testing/phonerow";
import { COMPANY, project } from "../../testing/projects.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ProjectsScreen } from "./projects";

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
    "GET /companies": () =>
      jsonResponse({ data: [COMPANY], page: { next_cursor: null } }),
    [`GET /companies/${COMPANY.id}`]: () => jsonResponse(COMPANY),
    "GET /records/references": () =>
      jsonResponse({
        data: [
          {
            id: COMPANY.id,
            name: COMPANY.display_name,
            archived: false,
            logo: null,
          },
        ],
      }),
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

/** At 390px line one ends in the phase and line two names the customer, when there is one. */
export const ListPhone: Story = {
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: () => {
    installList([
      ...rows,
      project({ id: "pr-4", name: "Internal tooling", company_id: null }),
    ]);
    return (
      <StoryProviders>
        <ProjectsScreen />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findAllByText(COMPANY.display_name);
    const fleet = phoneRow(canvas.getByText("Fleet retrofit"));
    await expect([fleet.end, fleet.caption]).toEqual([
      ["Delivering"],
      [COMPANY.display_name],
    ]);
    const internal = phoneRow(canvas.getByText("Internal tooling"));
    await expect([internal.end, internal.caption]).toEqual([
      ["Initiative"],
      [],
    ]);
  },
};

/** `#/projects/new`: the list with its create form open, as `C` opens it. */
export const CreateFormOpen: Story = {
  render: () => {
    installList(rows);
    return (
      <StoryProviders>
        <ProjectsScreen startCreating />
      </StoryProviders>
    );
  },
  play: async () => {
    await expect(
      await screen.findByRole("dialog", { name: "New project" }),
    ).toBeInTheDocument();
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

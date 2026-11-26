// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen } from "storybook/test";
import { project, project360 } from "../../testing/projects.fixtures";
import {
  emptyPage,
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { type ProjectDrawerKind, ProjectDrawers } from "./project360.drawers";

// The project page's reading drawers. All tasks opens the history narrowed to
// tasks under its own title; the companies and contacts drawer holds each
// link's menu.

const meta: Meta<typeof ProjectDrawers> = {
  title: "Records/Project 360/Drawers",
  component: ProjectDrawers,
};
export default meta;
type Story = StoryObj<typeof ProjectDrawers>;

const VIEW = project360({
  project: {
    ...project(),
    companies: [
      {
        company_id: "c-1",
        display_name: "Brandt Automotive",
        role: "customer",
      },
      { company_id: "c-2", display_name: "Relay Systems", role: "partner" },
    ],
  },
  stakeholders: {
    data: [
      {
        relationship_id: "rel-1",
        contact_id: "p-1",
        contact_name: "Mai Trần",
        role: "champion",
      },
    ],
    page: { next_cursor: null, has_more: false },
  },
});

function drawer(open: ProjectDrawerKind) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({ project: ["read", "update"], activity: ["read"] }),
      "GET /field-history": () => jsonResponse(emptyPage),
    });
    return (
      <StoryProviders>
        <ProjectDrawers
          view={VIEW}
          drawer={open}
          readOnly={false}
          onClose={() => {}}
        />
      </StoryProviders>
    );
  };
}

export const AllTasks: Story = {
  render: drawer("tasks"),
  play: async () => {
    await screen.findByRole("dialog", { name: "All tasks" });
  },
};

export const CompaniesAndContacts: Story = {
  render: drawer("contacts"),
  play: async () => {
    await screen.findByRole("button", {
      name: "More actions for Relay Systems",
    });
  },
};

export const CompaniesAndContactsPhone: Story = {
  tags: ["uat-phone"],
  render: drawer("contacts"),
};

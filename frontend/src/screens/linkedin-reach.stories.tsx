// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { within } from "storybook/test";
import { Panel } from "../design-system/panel";
import { meFixture } from "../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { LinkedInReachSection } from "./linkedin-reach";

// The reach section, inside the panel it renders in, for the fe-uat render
// gate: a list that stops short of the whole (so the caveat says where),
// nothing resolved (the unresolved count replaces the empty sentence), nothing
// imported, and a failed read, which must not look like an empty network.

function reachStory(body: unknown, status = 200) {
  return () => {
    installFetchStub({
      "GET /me": () => jsonResponse(meFixture()),
      "GET /me/linkedin-reach": () => jsonResponse(body, status),
    });
    return (
      <StoryProviders>
        <Panel title="LinkedIn network">
          <LinkedInReachSection />
        </Panel>
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof LinkedInReachSection> = {
  title: "Settings/You/Connections/LinkedIn reach",
  component: LinkedInReachSection,
};
export default meta;
type Story = StoryObj<typeof LinkedInReachSection>;

const REACHED = {
  accounts: [
    {
      company_id: "018f3a1b-0000-7000-8000-0000000000a1",
      display_name: "Nordwind Logistik GmbH",
      connections: 14,
      contacts_on_file: 3,
    },
    {
      company_id: "018f3a1b-0000-7000-8000-0000000000a2",
      display_name: "Havelmann & Söhne",
      connections: 6,
      contacts_on_file: 6,
    },
  ],
  accounts_total: 9,
  unresolved_connections: 1420,
};

export const Reaches: Story = { render: reachStory(REACHED) };

export const NothingResolvedYet: Story = {
  render: reachStory({
    accounts: [],
    accounts_total: 0,
    unresolved_connections: 5210,
  }),
};

export const NothingImportedYet: Story = {
  render: reachStory({
    accounts: [],
    accounts_total: 0,
    unresolved_connections: 0,
  }),
};

export const ReadFailed: Story = {
  render: reachStory(
    { title: "Internal Server Error", detail: "the reach index is rebuilding" },
    500,
  ),
};

export const ReachesDark: Story = {
  globals: { theme: "dark" },
  render: reachStory(REACHED),
};

// At 390 the table folds each account onto two lines: the name and its open
// chevron, then the two figures in words.
export const ReachesPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: reachStory(REACHED),
  play: async ({ canvasElement }) => {
    await within(canvasElement).findByRole("table", {
      name: "Companies you reach",
    });
  },
};

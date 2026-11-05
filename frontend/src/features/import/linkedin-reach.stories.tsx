// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { meFixture } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { ConnectionOutcomesCard } from "../capture/connection-outcomes";

// Three cases read differently: accounts reached, nothing resolved yet (still
// counting the unresolved), and a failed read, which is not an empty one.

function reachStory(body: unknown, status = 200) {
  return () => {
    installFetchStub({
      "GET /me": () => jsonResponse(meFixture()),
      "GET /me/linkedin-reach": () => jsonResponse(body, status),
      "GET /capture/senders": () => jsonResponse({ data: [] }),
      "GET /capture/contacts-awaiting-decision": () =>
        jsonResponse({ data: [] }),
      "GET /capture/held-threads": () => jsonResponse({ data: [] }),
    });
    return (
      <StoryProviders>
        <ConnectionOutcomesCard />
      </StoryProviders>
    );
  };
}

// The reach opens from its row, in a drawer portalled to the body.
async function openReach({
  canvasElement,
}: Readonly<{ canvasElement: HTMLElement }>) {
  await userEvent.click(
    await within(canvasElement).findByRole("button", {
      name: "View LinkedIn network",
    }),
  );
  await within(canvasElement.ownerDocument.body).findByRole("table", {
    name: "Companies you reach",
  });
}

const meta = {
  title: "Settings/You/Connections/LinkedIn reach",
  component: ConnectionOutcomesCard,
} satisfies Meta<typeof ConnectionOutcomesCard>;
export default meta;
type Story = StoryObj<typeof meta>;

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

export const Reaches: Story = { render: reachStory(REACHED), play: openReach };

// Nothing resolved yet, so the unresolved count matters most. Five thousand
// connections that matched no account is not "none yet": the row says so.
export const NothingResolvedYet: Story = {
  render: reachStory({
    accounts: [],
    accounts_total: 0,
    unresolved_connections: 5210,
  }),
  play: async ({ canvasElement }) => {
    await expect(
      await within(canvasElement).findByText(/Connections: 5,210/),
    ).toBeVisible();
  },
};

// Nothing imported yet: no caveat to state, and no list to open.
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
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(await canvas.findByText("Could not load")).toBeVisible();
    await expect(
      canvas.getByText("the reach index is rebuilding"),
    ).toBeVisible();
  },
};

export const ReachesDark: Story = {
  globals: { theme: "dark" },
  render: reachStory(REACHED),
  play: openReach,
};

// At 390 the table folds each account onto two lines: the name and its open
// chevron, then the two figures in words.
export const ReachesPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: reachStory(REACHED),
  play: openReach,
};

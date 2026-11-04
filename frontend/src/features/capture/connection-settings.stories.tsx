// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { components } from "../../api/schema";
import { SavePageProvider } from "../../design-system/savepage";
import { heldThreadList } from "../../testing/heldthreads.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { AccountSettings } from "./connection-settings";

// One connected mailbox's standing decisions. The held pointer counts the
// whole held list, not the first page the drawer loads.

type CaptureConnection = components["schemas"]["CaptureConnection"];

const MAILBOX: CaptureConnection = {
  id: "c1",
  provider: "gmail",
  can_backfill: true,
  status: "connected",
  scopes: ["read"],
  mail_posture: "classified",
};

function story(list: ReturnType<typeof heldThreadList>) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /capture/settings": () =>
        jsonResponse({ shared_posture_allowed: true }),
      "GET /capture/held-threads": () => jsonResponse(list),
    });
    return (
      <StoryProviders>
        <SavePageProvider>
          <AccountSettings mailbox={MAILBOX} others={[]} accountName="Ada" />
        </SavePageProvider>
      </StoryProviders>
    );
  };
}

const meta = {
  title: "Settings/You/Connections/Mailbox settings",
  component: AccountSettings,
} satisfies Meta<typeof AccountSettings>;
export default meta;

type Story = StoryObj<typeof meta>;

// More held than one page carries: the count is the list's, from its counts.
export const ManyHeld: Story = {
  args: { mailbox: MAILBOX, others: [], accountName: "Ada" },
  render: story(
    heldThreadList([], {
      page: { next_cursor: "page-2", has_more: true },
      counts: {
        pending: 212,
        unsure: 18,
        held: 940,
        held_by_owner: 3,
        stalled: 0,
      },
    }),
  ),
  play: async ({ canvasElement }) => {
    await within(canvasElement).findByText(/1,173 threads are held back/);
  },
};

export const NothingHeld: Story = {
  args: { mailbox: MAILBOX, others: [], accountName: "Ada" },
  render: story(heldThreadList([])),
  play: async ({ canvasElement }) => {
    await expect(
      await within(canvasElement).findByText(/Nothing is held back/),
    ).toBeTruthy();
  },
};

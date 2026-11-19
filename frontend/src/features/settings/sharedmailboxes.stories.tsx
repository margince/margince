// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../../testing/story-utils";
import { SharedMailboxesCard } from "./sharedmailboxes";

const meta: Meta<typeof SharedMailboxesCard> = {
  title: "Settings/Data/Integrations/Shared mailboxes",
  component: SharedMailboxesCard,
};
export default meta;
type Story = StoryObj<typeof SharedMailboxesCard>;

const SALES_TEAM = "018f3a1b-0000-7000-8000-0000000000e1";

const sales = {
  id: "018f3a1b-0000-7000-8000-0000000000f1",
  provider: "imap",
  address: "sales@acme.example",
  reader_team_id: SALES_TEAM,
  mail_visibility: "held",
  status: "connected",
  connected_by: null,
  connected_at: "2026-11-12T09:00:00Z",
};

// The list answers as given; the roster names the one team the rows point at.
function cardStory(list: () => Response) {
  return () => {
    stubWithSession(
      {
        "GET /shared-mailboxes": list,
        "GET /teams": () =>
          jsonResponse({
            data: [{ id: SALES_TEAM, name: "Sales desk" }],
            page: { next_cursor: null },
          }),
      },
      {},
      { roles: ["admin"] },
    );
    return (
      <StoryProviders>
        <SharedMailboxesCard />
      </StoryProviders>
    );
  };
}

export const NoneConnected: Story = {
  render: cardStory(() => jsonResponse({ items: [] })),
};

export const Connected: Story = {
  render: cardStory(() =>
    jsonResponse({
      items: [
        sales,
        {
          ...sales,
          id: "018f3a1b-0000-7000-8000-0000000000f2",
          provider: "graph",
          address: "support@acme.example",
          mail_visibility: "shared",
          status: "disconnected",
        },
      ],
    }),
  ),
};

export const SyncError: Story = {
  render: cardStory(() =>
    jsonResponse({ items: [{ ...sales, status: "error" }] }),
  ),
};

export const NoPermission: Story = {
  render: cardStory(() => jsonResponse({ code: "permission_denied" }, 403)),
};

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import type { components } from "../../api/schema";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../../testing/story-utils";
import { FromMailboxRow } from "./compose.from";

const meta: Meta = {
  title: "Patterns/Compose mail/From mailbox",
};
export default meta;

type Story = StoryObj;
type CaptureConnection = components["schemas"]["CaptureConnection"];

const work: CaptureConnection = {
  id: "018f3a1b-0000-7000-8000-0000000000c1",
  provider: "gmail",
  can_backfill: true,
  status: "connected",
  scopes: [],
  account_label: "rep@acme.test",
};

function Picker() {
  const [from, setFrom] = useState("");
  return <FromMailboxRow value={from} onChange={setFrom} />;
}

/** Two mailboxes can send, so the composer offers the choice. */
export const TwoMailboxes: Story = {
  render: () => {
    stubWithSession(
      {
        "GET /connectors": () =>
          jsonResponse({
            data: [
              work,
              {
                ...work,
                id: "018f3a1b-0000-7000-8000-0000000000c2",
                provider: "graph",
                account_label: "rep@second.test",
              },
            ],
          }),
      },
      {},
    );
    return (
      <StoryProviders>
        <Picker />
      </StoryProviders>
    );
  },
};

/** One mailbox sends, so the row draws nothing. */
export const OneMailbox: Story = {
  render: () => {
    stubWithSession(
      { "GET /connectors": () => jsonResponse({ data: [work] }) },
      {},
    );
    return (
      <StoryProviders>
        <Picker />
      </StoryProviders>
    );
  },
};

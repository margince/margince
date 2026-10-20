// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { viewerZone } from "../format/timezone";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { NotificationBell } from "./notificationbell";
import { OutboxButton } from "./outboxbutton";

// The clock beside the bell: what a rep has scheduled and not yet sent. A story
// here is an ANSWER of the queue endpoint, mounted beside the bell in a strip,
// because the pair is read together and the count sits on the glyph's corner.

type Status = "scheduled" | "held" | "sent";

function row(id: string, status: Status) {
  return {
    id,
    status,
    scheduled_at: "2026-09-01T07:00:00Z",
    scheduled_tz: viewerZone(),
    subject: `Message ${id}`,
    to: ["ceo@acme.example"],
    version: 1,
    created_at: "2026-08-20T09:00:00Z",
    updated_at: "2026-08-20T09:00:00Z",
  };
}

function story(statuses: Status[]) {
  return () => {
    installFetchStub({
      "GET /scheduled-sends": () =>
        jsonResponse(statuses.map((status, i) => row(String(i), status))),
      "GET /notices": () => jsonResponse({ items: [], unread_count: 0 }),
    });
    return (
      <StoryProviders>
        <div className="topbar">
          <div className="topbar-lead" />
          <div />
          <div className="topbar-trail">
            <OutboxButton />
            <NotificationBell />
          </div>
        </div>
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof OutboxButton> = {
  title: "Shell/Outbox button",
  component: OutboxButton,
  parameters: { layout: "fullscreen" },
};
export default meta;
type Story = StoryObj<typeof OutboxButton>;

// Nothing queued: the clock stays, without a figure, so the way in is still
// there for a rep who wants to look.
export const Empty: Story = { render: story(["sent"]) };

export const Waiting: Story = {
  render: story(["scheduled", "scheduled", "scheduled", "sent"]),
};

// A held message is the one that will not send itself, so the count turns amber.
export const Held: Story = {
  render: story(["scheduled", "scheduled", "held"]),
};

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent } from "storybook/test";
import type { components } from "../../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../../testing/story-utils";
import { ManageRoomDialog } from "./dealroommanage";

// The room's settings dialog: who may still enter, until when, the buyer's
// contact, and the words a buyer reads first.

type DealRoom = components["schemas"]["DealRoom"];

const ROOM: DealRoom = {
  id: "room-1",
  deal_id: "deal-1",
  title: "Nordwind — Fleet telematics",
  welcome_message:
    "Everything we agreed is in here. Add a comment on anything you want changed.",
  state: "live",
  source: "manual",
  captured_by: "u-me",
  version: 1,
  created_at: "2026-08-22T09:00:00Z",
  updated_at: "2026-08-24T14:02:00Z",
};

const meta: Meta = {
  title: "Records/Deal 360/Manage room dialog",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

function dialog(over: Partial<DealRoom> = {}) {
  installFetchStub({
    "GET /me": meRoute({ deal_room: ["read", "update"] }),
    "GET /users": () =>
      jsonResponse({
        data: [{ id: "u-me", display_name: "Ada Admin" }],
        page: { next_cursor: null },
      }),
  });
  return (
    <StoryProviders>
      <ManageRoomDialog
        room={{ ...ROOM, ...over }}
        open
        onClose={() => undefined}
      />
    </StoryProviders>
  );
}

/** A live room, as the dialog opens. */
export const Live: Story = { render: () => dialog() };

/** Choosing Closed says what closing does before anything is saved. */
export const Closing: Story = {
  render: () => dialog(),
  play: async () => {
    await userEvent.click(await screen.findByRole("radio", { name: /Closed/ }));
  },
};

/** A paused room with an end date. */
export const PausedUntil: Story = {
  render: () => dialog({ state: "paused", expires_at: "2026-12-31T23:59:59Z" }),
};

/** A closed room: every field stands, and the save says why it is refused. */
export const Finished: Story = { render: () => dialog({ state: "closed" }) };

export const LiveDark: Story = {
  globals: { theme: "dark" },
  render: () => dialog(),
};

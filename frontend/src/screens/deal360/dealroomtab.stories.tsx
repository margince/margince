// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../story-utils";
import { RoomFacts, RoomText, ViewAsBuyerButton } from "./dealroomtab";

// The deal's own door into its room: who has been in, the one way to see what
// they see, and the text the buyer reads when they arrive.
//
// The text panel is the part with states worth looking at. It is editable in
// place and its head is a title and nothing else — the sentence that used to
// hang under that title now belongs to the body, where it can run as long as it
// needs to. A room nobody may change keeps the panel and loses the Save verb,
// with one sentence in its place saying why.

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
  title: "Records/Deal 360/Deal room tab",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

function room(over: Partial<DealRoom> = {}): DealRoom {
  return { ...ROOM, ...over };
}

function stub() {
  installFetchStub({
    "GET /me": meRoute({ deal_room: ["read", "update"] }),
    "GET /deal-rooms/room-1/participants": () =>
      jsonResponse({
        data: [
          {
            id: "part-1",
            room_id: ROOM.id,
            full_name: "Dana Buyer",
            email: "dana.buyer@nordwind.example",
            capability: "read",
            delivery_state: "delivered",
            has_signed_in: true,
            last_seen_at: "2026-08-24T14:02:00Z",
            source: "manual",
            captured_by: "u-me",
            created_at: "2026-08-22T09:10:00Z",
            updated_at: "2026-08-24T14:02:00Z",
          },
        ],
        page: { next_cursor: null },
      }),
  });
}

/** The head: who has been in, and the verb that sees what they see. */
export const Head: Story = {
  render: () => {
    stub();
    return (
      <StoryProviders>
        <div className="roomtab-head">
          <RoomFacts room={room()} />
          <ViewAsBuyerButton room={room()} />
        </div>
      </StoryProviders>
    );
  },
};

/** The buyer-facing text, editable. The head is a title; the rest is body. */
export const Text: Story = {
  render: () => {
    stub();
    return (
      <StoryProviders>
        <RoomText room={room()} refusal={undefined} />
      </StoryProviders>
    );
  },
};

/**
 * A finished room. The panel keeps its fields and loses the Save verb — one
 * sentence stands where the verb was, which is what a refusal owes a reader.
 */
export const TextRefused: Story = {
  render: () => {
    stub();
    return (
      <StoryProviders>
        <RoomText
          room={room({ state: "closed" })}
          refusal="This room has ended. Its shared content is kept as a record."
        />
      </StoryProviders>
    );
  },
};

/** An archived room: the preview is refused with the reason beside it. */
export const PreviewRefused: Story = {
  render: () => {
    stub();
    return (
      <StoryProviders>
        <ViewAsBuyerButton room={room({ state: "archived" })} />
      </StoryProviders>
    );
  },
};

export const TextDark: Story = {
  globals: { theme: "dark" },
  render: () => {
    stub();
    return (
      <StoryProviders>
        <RoomText room={room()} refusal={undefined} />
      </StoryProviders>
    );
  },
};

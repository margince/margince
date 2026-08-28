// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { DealRoomPage } from "./dealroompage";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// The seller's whole Deal Room: the header and its verbs, the state banner, the
// editable welcome text, the conversation, and the access card beside them.
//
// The three states below are the three the page's own query has — still asking,
// answered with nothing, answered with a room — and the reason all three are
// stories is the page gutter. `.wrap` sits OUTSIDE the query states, so the
// skeleton, the "no room" callout and a loaded room have to be judged against
// the same left edge; two of them are invisible to any story that only renders
// the happy one.

type DealRoom = components["schemas"]["DealRoom"];
type Participant = components["schemas"]["DealRoomParticipant"];

const ROOM: DealRoom = {
  id: "room-1",
  deal_id: "deal-1",
  title: "Brandt Automotive — retrofit programme",
  state: "live",
  source: "manual",
  captured_by: "u-me",
  version: 3,
  // A live room with an end date, because that is the banner with a value in
  // it: every other state's banner is a fixed sentence.
  expires_at: "2026-11-30T23:59:59Z",
  welcome_message:
    "Everything we agreed in Stuttgart is here. Ask in the thread under any document.",
  created_at: "2026-08-22T09:00:00Z",
  updated_at: "2026-08-24T14:02:00Z",
};

const BUYER: Participant = {
  id: "part-1",
  room_id: ROOM.id,
  full_name: "Dana Buyer",
  email: "dana.buyer@brandt-automotive.example",
  capability: "read",
  delivery_state: "delivered",
  has_signed_in: true,
  last_seen_at: "2026-08-24T14:02:00Z",
  source: "manual",
  captured_by: "u-me",
  created_at: "2026-08-22T09:10:00Z",
  updated_at: "2026-08-24T14:02:00Z",
};

/**
 * The page's own reads, at the network edge.
 *
 * `deal_room:update` is the seat every story below is about: without it the
 * header loses both verbs and the welcome text goes read-only, which is a real
 * state of this page but not the one any of these is named for.
 *
 * The documents and threads under the conversation take the stub's empty-page
 * fallback deliberately. They are a board with stories of its own
 * (dealroomdocuments, dealroomthreads), and an empty board is what a room looks
 * like on the day it is opened.
 */
function served(rooms: DealRoom[], pending = false) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({ deal_room: ["read", "update"] }),
      "GET /deal-rooms": pending
        ? () => new Promise<Response>(() => {})
        : () => jsonResponse({ data: rooms, page: { next_cursor: null } }),
      "GET /deal-rooms/room-1/participants": () =>
        jsonResponse({ data: [BUYER], page: { next_cursor: null } }),
    });
    return (
      <StoryProviders>
        <DealRoomPage dealId="deal-1" />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof DealRoomPage> = {
  title: "Records/Deal room/Page",
  component: DealRoomPage,
  parameters: { layout: "fullscreen" },
};
export default meta;
type Story = StoryObj<typeof DealRoomPage>;

/** The read has not landed. The skeleton stands in the page gutter, not against
 *  the scroller's edge. */
export const Loading: Story = { render: served([], true) };

/** The deal has no room. The page says so in one line and offers nothing —
 *  opening a room is the deal page's verb, not this page's. */
export const NoRoom: Story = { render: served([]) };

/** A live room with an end date: the state chip and the two verbs in the
 *  header, the banner naming the day access stops, the welcome text a buyer
 *  reads first, and the buyer's own row beside it. */
export const Live: Story = { render: served([ROOM]) };

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import type { GrantSpec } from "../app/mefixture";
import { ContactDealRooms } from "./contactdealrooms";
import {
  jsonResponse,
  type RouteMap,
  StoryProviders,
  stubWithSession,
} from "./story-utils";

// The rooms a contact can still enter, on the contact's own page. An admin
// removing "Max, who left the buyer" knows the NAME, not the deals — the room
// page's Revoke is reachable only from a deal — so this card is the path that
// starts from the person.
//
// The frames are the two standings a reader can hold over a seat and the one
// honest limit: a card that lists 50 rooms and stops says it is cut rather than
// pretending it is whole. A contact with no seat renders nothing at all, so
// there is no frame for it — an empty panel on a record page is a claim that
// the reader should look here, and there is nothing to look at.

type DealRoom = components["schemas"]["DealRoom"];

const EMAIL = "dana@buyer.example";

function room(over: Partial<DealRoom> = {}): DealRoom {
  return {
    id: "11111111-1111-4111-8111-111111111111",
    deal_id: "22222222-2222-4222-8222-222222222222",
    title: "Acme expansion",
    state: "live",
    source: "admin",
    captured_by: "33333333-3333-4333-8333-333333333333",
    created_at: "2026-05-04T09:00:00Z",
    updated_at: "2026-05-04T09:00:00Z",
    ...over,
  } as DealRoom;
}

const ROOMS: readonly DealRoom[] = [
  room(),
  room({
    id: "11111111-1111-4111-8111-111111111112",
    deal_id: "22222222-2222-4222-8222-222222222223",
    title: "Brandt retrofit",
    state: "paused",
  }),
];

function rooms(list: readonly DealRoom[], cut = false): RouteMap {
  return {
    "GET /deal-rooms": () =>
      jsonResponse({ data: list, page: { next_cursor: null, has_more: cut } }),
  };
}

function card(
  routes: RouteMap,
  seat: { roles?: string[]; seat?: "full" | "read" } = {},
  allow: GrantSpec = { deal_room: ["read", "update"] },
) {
  return () => {
    stubWithSession(routes, allow, seat);
    return (
      <StoryProviders>
        <ContactDealRooms emails={[EMAIL]} />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof ContactDealRooms> = {
  title: "Records/Contact record/Deal rooms",
  component: ContactDealRooms,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof ContactDealRooms>;

/** Two seats, each with the room's own state beside it and the two verbs an
 *  admin has: open the room, or end the seat. */
export const SeatsThisContactHolds: Story = { render: card(rooms(ROOMS)) };

/**
 * A reader who may not manage seats. The revoke verb is absent rather than
 * refused — the way in stays, because reading the room is a different grant
 * from ending somebody's access to it.
 */
export const NoRightToRevoke: Story = {
  render: card(rooms(ROOMS), { roles: ["rep"], seat: "read" }, {}),
};

/** More rooms than the card lists. It says the list is cut rather than letting
 *  a reader believe they have seen every seat this contact holds. */
export const MoreRoomsThanWeList: Story = {
  render: card(rooms(ROOMS, true)),
};

/**
 * Ending a seat, at the moment of asking. The dialog names the room, repeats
 * the address and the room's state, and says what revoking does — an admin who
 * opened this from a NAME cannot otherwise tell which of two rooms they are
 * about to close.
 *
 * `ConfirmModal` portals to document.body, so the play names what it expects:
 * a dialog that never mounted fails the render gate instead of passing it.
 */
export const RevokingASeat: Story = {
  render: card(rooms([room()])),
  play: async () => {
    await userEvent.click(
      await screen.findByRole("button", { name: "Revoke access" }),
    );
    const dialog = within(await screen.findByRole("dialog"));
    await dialog.findByText(EMAIL, { exact: false });
  },
};

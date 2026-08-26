// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { DealRoomAside } from "./dealroom";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// The states a rep meets this surface in, so each can be judged without
// arranging a room, a buyer and a close on a running stack.

const meta: Meta<typeof DealRoomAside> = {
  title: "Screens/Deal Room aside",
  component: DealRoomAside,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof DealRoomAside>;

function room(state: string) {
  return {
    id: "room-1",
    deal_id: "deal-1",
    title: "Acme Expansion — Deal Room",
    state,
    source: "manual",
    version: 1,
    created_at: "2026-08-22T09:00:00Z",
    updated_at: "2026-08-22T09:00:00Z",
  };
}

/**
 * Answers the reads the aside makes, at the NETWORK edge rather than by seeding
 * a cache.
 *
 * Seeding was the first spelling and it is only as complete as its list of
 * query keys: the aside also probes `/me` for `deal_room:create`, and a key
 * nobody thought of is a request that leaves the page for whatever host serves
 * the iframe, 404s, and resolves to an answer that looks legitimate. Routing
 * says what this surface is allowed to ask for, and `installFetchStub`'s own
 * fallback answers an empty page for anything else instead of the network.
 */
function Served({
  rooms,
  children,
}: Readonly<{ rooms: unknown[]; children: ReactNode }>) {
  installFetchStub({
    // The aside offers the create verb, so the seat that may press it is part
    // of every state below — a story routed without it would document the
    // refused form rather than the state it is named for.
    "GET /me": meRoute({ deal_room: ["read", "create"] }),
    "GET /deal-rooms": () => jsonResponse({ data: rooms, page: {} }),
  });
  return <StoryProviders>{children}</StoryProviders>;
}

/** A live room mid-negotiation. */
export const Live: Story = {
  render: () => (
    <Served rooms={[room("live")]}>
      <DealRoomAside dealId="deal-1" dealName="Acme Expansion" />
    </Served>
  ),
};

/** A room nobody has published yet — a draft the buyer cannot see. */
export const Draft: Story = {
  render: () => (
    <Served rooms={[room("draft")]}>
      <DealRoomAside dealId="deal-1" dealName="Acme Expansion" />
    </Served>
  ),
};

/**
 * A closed room. Every control states why it refuses, and the add form is gone
 * rather than present and refusing — the state this surface most needs a second
 * look at, because it is the one a rep meets months after the deal.
 */
export const Closed: Story = {
  render: () => (
    <Served rooms={[room("closed")]}>
      <DealRoomAside dealId="deal-1" dealName="Acme Expansion" />
    </Served>
  ),
};

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { BuyerRoomScreen } from "./buyerroom";
import {
  installFetchStub,
  jsonResponse,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

// The buyer's Deal Room, one story per access state, so the five screens an
// outside person can land on are exercised without a backend in any state.

const PARTICIPANT = {
  id: "p-1",
  full_name: "Laura Buyer",
  email: "laura@buyer.example",
  capability: "comment",
};

const ROOM = {
  title: "Acme rollout",
  welcome_message: "Welcome, Laura. Everything about the rollout is here.",
  release_no: 2,
  released_at: "2026-08-22T09:00:00Z",
  steward_name: "Ada Admin",
};

// The railless shell frame, reproduced because the bug this screen shipped
// lived ENTIRELY in it. `room` is in RAIL_LESS_SCREENS, so Shell renders
// `.app.railless > .main > .scroll` around it: `.main` is a flex column with
// `overflow: hidden` and `.scroll` is `flex: 1`, which hands the page a
// definite height. Under that height the buyer column's panels were shrunk to
// fit and `.panel { overflow: hidden }` discarded the rest — the documents
// panel drew 155px of 575px, so the buyer saw a filename and none of the
// threads or composer beneath it.
//
// Mounted bare in StoryProviders, as this file used to, there is no bounded
// height, nothing shrinks, and the story renders a page the product cannot
// produce. The height is fixed rather than viewport-relative so the constraint
// exists in a docs frame too.
function RaillessFrame({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <div className="app railless" style={{ height: "640px" }}>
      <main className="main">
        <div className="scroll">{children}</div>
      </main>
    </div>
  );
}

function room(routes: RouteMap, session = true) {
  return () => {
    installFetchStub(routes);
    if (session) {
      globalThis.sessionStorage.setItem("margince.room.session", "mdrs_story");
    } else {
      globalThis.sessionStorage.removeItem("margince.room.session");
    }
    return (
      <StoryProviders>
        <RaillessFrame>
          <BuyerRoomScreen />
        </RaillessFrame>
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof BuyerRoomScreen> = {
  title: "Signed out/Deal Room (buyer)",
  component: BuyerRoomScreen,
};
export default meta;

type Story = StoryObj<typeof BuyerRoomScreen>;

export const Live: Story = {
  render: room({
    "GET /public/rooms/me": () =>
      jsonResponse({
        access: "live",
        participant: PARTICIPANT,
        steward_name: "Ada Admin",
        room: ROOM,
      }),
  }),
};

export const Closed: Story = {
  render: room({
    "GET /public/rooms/me": () =>
      jsonResponse({
        access: "closed",
        participant: PARTICIPANT,
        steward_name: "Ada Admin",
        room: { ...ROOM, closed_at: "2026-08-22T10:00:00Z" },
      }),
  }),
};

export const Paused: Story = {
  render: room({
    "GET /public/rooms/me": () =>
      jsonResponse({
        access: "paused",
        participant: PARTICIPANT,
        steward_name: "Ada Admin",
      }),
  }),
};

export const Expired: Story = {
  render: room({
    "GET /public/rooms/me": () =>
      jsonResponse({
        access: "expired",
        participant: PARTICIPANT,
        steward_name: "Ada Admin",
      }),
  }),
};

export const DeadLink: Story = {
  render: room({}, false),
};

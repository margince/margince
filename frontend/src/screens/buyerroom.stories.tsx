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

// The refused states. They exist because a rep uses `View as buyer` to
// check what their buyer sees, and a refused reader who is shown NO write
// control reads exactly like a buyer who may comment — the pages are
// identical, which is the opposite of what a preview is for. So each of these
// has to show the affordance in its disabled state, carrying the reason.

const DOCUMENT = {
  id: "doc-1",
  group_key: "commercial",
  title: "Rahmenvertrag",
  position: 1,
  filename: "rahmenvertrag.pdf",
  content_type: "application/pdf",
  byte_size: 182_000,
};

const SELLER = { side: "seller", name: "Ada Admin" };

const THREAD = {
  id: "th-1",
  room_id: "room-1",
  document_id: "doc-1",
  required_change: false,
  state: "open",
  author: SELLER,
  created_at: "2026-08-22T10:00:00Z",
  comments: [
    {
      id: "c-1",
      thread_id: "th-1",
      body: "Which clause covers the notice period?",
      author: SELLER,
      created_at: "2026-08-22T10:00:00Z",
    },
  ],
};

function previewRoutes(documents: unknown[], threads: unknown[]): RouteMap {
  return {
    "GET /public/rooms/me": () =>
      jsonResponse({
        access: "live",
        // A preview seat as the server actually mints it: read-only. The
        // capability matters as well as the flag — a fixture that left this
        // `comment` would draw a live composer and quietly document a page
        // the product never serves.
        participant: { ...PARTICIPANT, capability: "view" },
        preview: true,
        steward_name: "Ada Admin",
        room: ROOM,
      }),
    "GET /public/rooms/documents": () => jsonResponse({ data: documents }),
    "GET /public/rooms/threads": () => jsonResponse({ data: threads }),
  };
}

// A rep previewing a room that HAS documents: every card carries a refused
// "Ask about this document", and the reason is stated once on the room panel
// below rather than repeated under each file.
export const PreviewWithDocuments: Story = {
  render: room(previewRoutes([DOCUMENT], [THREAD])),
};

// A rep previewing a room with nothing in it yet — the first thing anyone
// previews, and the state that shipped with no control at all: the refusal
// sentence with nothing to attach it to. The room composer draws its own
// button here, because there is no document card to carry one.
export const PreviewEmptyRoom: Story = {
  render: room(previewRoutes([], [])),
};

// Not a preview: a real buyer invited with the read-only `view` capability.
// Same refused controls, a different sentence.
export const ViewOnlySeat: Story = {
  render: room({
    "GET /public/rooms/me": () =>
      jsonResponse({
        access: "live",
        participant: { ...PARTICIPANT, capability: "view" },
        steward_name: "Ada Admin",
        room: ROOM,
      }),
    "GET /public/rooms/documents": () => jsonResponse({ data: [DOCUMENT] }),
    "GET /public/rooms/threads": () => jsonResponse({ data: [THREAD] }),
  }),
};

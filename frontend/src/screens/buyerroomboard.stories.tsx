// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { BuyerBoard } from "./buyerroomboard";
import {
  installFetchStub,
  jsonResponse,
  type RouteMap,
  StoryProviders,
} from "./story-utils";
// The buyer page's own sheet: `.buyer-doc-actions` is reached BY CLASS from
// this file, and the module graph of a story here stops short of buyerroom.tsx
// which loads it.
import "./buyerroom.css";

// The buyer's half of a Deal Room: the documents the seller shared, the
// questions under each, and the one verb a buyer has on a file — take a copy.
//
// Everything arrives on the room session's Bearer, so each frame routes the two
// public reads the board makes and nothing else. What differs between them is
// what the SEAT may do: a commenting buyer, a read-only one, and a room with
// nothing in it yet.

type BuyerRoomDocument = components["schemas"]["BuyerRoomDocument"];
type DealRoomThread = components["schemas"]["DealRoomThread"];

const TOKEN = "mdrs_story";

const DOCUMENTS: readonly BuyerRoomDocument[] = [
  {
    id: "doc-1",
    group_key: "commercial",
    title: "Commercial terms v4",
    position: 1,
    filename: "commercial-terms-v4.pdf",
    content_type: "application/pdf",
    byte_size: 412_000,
  },
  {
    id: "doc-2",
    group_key: "delivery_operations",
    title: "Implementation plan",
    position: 2,
    filename: "implementation-plan.xlsx",
    byte_size: 88_000,
  },
];

const THREADS: readonly DealRoomThread[] = [
  {
    id: "thread-1",
    room_id: "room-1",
    document_id: "doc-1",
    required_change: true,
    state: "open",
    author: { side: "buyer", name: "Dana Buyer" },
    created_at: "2026-08-24T10:00:00Z",
    comments: [
      {
        id: "thread-1-c1",
        thread_id: "thread-1",
        body: "Clause 7 conflicts with our standard terms — this one we do need changed before signing.",
        author: { side: "buyer", name: "Dana Buyer" },
        created_at: "2026-08-24T10:00:00Z",
      },
      {
        id: "thread-1-c2",
        thread_id: "thread-1",
        body: "Understood. v5 restates it; I will share it this week.",
        author: { side: "seller", name: "Lena Fischer" },
        created_at: "2026-08-24T16:29:00Z",
      },
    ],
  },
];

function routes(
  documents: readonly BuyerRoomDocument[],
  threads: readonly DealRoomThread[],
): RouteMap {
  return {
    "GET /public/rooms/documents": () => jsonResponse({ data: documents }),
    "GET /public/rooms/threads": () => jsonResponse({ data: threads }),
  };
}

// The refusal sentence is the product's, not this file's: the buyer screen
// builds it with `t()` from whichever refusal it met, so a frame about a
// refused seat asks for the same key rather than typing English into the board.
function Board({
  mayWrite,
  refusalKey,
}: Readonly<{ mayWrite: boolean; refusalKey?: MessageKey }>) {
  const t = useT();
  return (
    <BuyerBoard
      token={TOKEN}
      onSessionLost={() => {}}
      mayWrite={mayWrite}
      refusal={refusalKey ? t(refusalKey) : undefined}
    />
  );
}

function board(
  documents: readonly BuyerRoomDocument[],
  threads: readonly DealRoomThread[],
  seat: { mayWrite: boolean; refusalKey?: MessageKey } = { mayWrite: true },
) {
  return () => {
    installFetchStub(routes(documents, threads));
    return (
      <StoryProviders>
        <Board mayWrite={seat.mayWrite} refusalKey={seat.refusalKey} />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof BuyerBoard> = {
  title: "Signed out/Deal room board",
  component: BuyerBoard,
};
export default meta;
type Story = StoryObj<typeof BuyerBoard>;

/** A buyer who may comment: a question already asked, and the place to ask another. */
export const CommentingBuyer: Story = { render: board(DOCUMENTS, THREADS) };

/** The room the moment it opens: shared files and nothing said about them yet. */
export const NothingAskedYet: Story = { render: board(DOCUMENTS, []) };

/** A room with nothing in it. The empty sentence is the board's, and the room
 *  composer still draws — a buyer can ask before the first file arrives. */
export const NoDocumentsYet: Story = { render: board([], []) };

/** A read-only seat. The composer's verbs are ABSENT rather than disabled, and
 *  the refusal is the sentence that says why — the board points every refused
 *  control at it instead of repeating it under each file. */
export const ReadOnlySeat: Story = {
  render: board(DOCUMENTS, THREADS, {
    mayWrite: false,
    refusalKey: "threads.readOnly",
  }),
};

/** At 390px the document tiles stack and a comment's author, time and body
 *  cannot share a line. */
export const Phone: Story = {
  tags: ["uat-phone"],
  render: board(DOCUMENTS, THREADS),
};

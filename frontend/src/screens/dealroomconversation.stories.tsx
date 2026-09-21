// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { useT } from "../i18n";
import { refusalFor } from "./dealroom";
import { DealRoomConversation } from "./dealroomconversation";
import {
  jsonResponse,
  type RouteMap,
  StoryProviders,
  stubWithSession,
} from "./story-utils";

// The seller's side of the document board: the documents in the room with
// their remove verb, the threads under each, and the form that adds another.
//
// Everything here is live — a document added is shared — so the frames are what
// the seller may DO rather than a publish ladder. The refusal sentence comes
// from `refusalFor`, the room page's own helper, so a frame carries the words
// the product would rather than a second spelling of them.

type DealRoom = components["schemas"]["DealRoom"];
type DealRoomDocument = components["schemas"]["DealRoomDocument"];
type DealRoomThread = components["schemas"]["DealRoomThread"];

const PAGE = { next_cursor: null, has_more: false };

const ROOM: DealRoom = {
  id: "11111111-1111-4111-8111-111111111111",
  deal_id: "22222222-2222-4222-8222-222222222222",
  title: "Acme expansion",
  state: "live",
  source: "admin",
  captured_by: "33333333-3333-4333-8333-333333333333",
  created_at: "2026-05-04T09:00:00Z",
  updated_at: "2026-05-04T09:00:00Z",
} as DealRoom;

function roomDocument(over: Partial<DealRoomDocument>): DealRoomDocument {
  return {
    id: "doc-1",
    room_id: ROOM.id,
    attachment_id: "att-1",
    group_key: "commercial",
    title: "Commercial terms v4",
    position: 1,
    filename: "commercial-terms-v4.pdf",
    byte_size: 412_000,
    source: "ui",
    version: 1,
    created_at: "2026-05-04T09:00:00Z",
    updated_at: "2026-05-04T09:00:00Z",
    ...over,
  } as DealRoomDocument;
}

const DOCUMENTS: readonly DealRoomDocument[] = [
  roomDocument({}),
  roomDocument({
    id: "doc-2",
    attachment_id: "att-2",
    group_key: "delivery_operations",
    title: "Implementation plan",
    position: 2,
    filename: "implementation-plan.pdf",
    byte_size: 1_240_000,
  }),
];

const BUYER = { side: "buyer", name: "Dana Buyer" };
const ASKED_AT = "2026-08-24T10:00:00Z";

const THREADS: readonly DealRoomThread[] = [
  {
    id: "thread-1",
    room_id: ROOM.id,
    document_id: "doc-1",
    required_change: true,
    state: "open",
    author: BUYER,
    created_at: ASKED_AT,
    comments: [
      {
        id: "thread-1-c1",
        thread_id: "thread-1",
        body: "Clause 7 conflicts with our standard terms — this one we do need changed before signing.",
        author: BUYER,
        created_at: ASKED_AT,
      },
    ],
  },
];

function routes(
  documents: readonly DealRoomDocument[],
  threads: readonly DealRoomThread[],
): RouteMap {
  return {
    [`GET /deal-rooms/${ROOM.id}/documents`]: () =>
      jsonResponse({ data: documents, page: PAGE }),
    [`GET /deal-rooms/${ROOM.id}/threads`]: () =>
      jsonResponse({ data: threads }),
    // The deal's Files area: what the add form may share from.
    [`GET /deals/${ROOM.deal_id}/documents`]: () =>
      jsonResponse({ data: [], page: PAGE }),
  };
}

function Conversation({
  finished,
  mayWrite,
}: Readonly<{ finished: boolean; mayWrite: boolean }>) {
  const t = useT();
  return (
    <DealRoomConversation
      room={ROOM}
      refusal={refusalFor(finished, mayWrite, t)}
    />
  );
}

function board(
  documents: readonly DealRoomDocument[],
  threads: readonly DealRoomThread[],
  standing: { finished?: boolean; mayWrite?: boolean } = {},
) {
  return () => {
    stubWithSession(routes(documents, threads), { deal: ["read", "update"] });
    return (
      <StoryProviders>
        <Conversation
          finished={standing.finished ?? false}
          mayWrite={standing.mayWrite ?? true}
        />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof DealRoomConversation> = {
  title: "Records/Deal room/Conversation",
  component: DealRoomConversation,
};
export default meta;
type Story = StoryObj<typeof DealRoomConversation>;

/** Two documents, a question the buyer marked as needing a change, and the
 *  form that adds the next file. */
export const DocumentsAndQuestions: Story = {
  render: board(DOCUMENTS, THREADS),
};

/** A room before anything is in it: "no documents" and "nothing said yet" are
 *  different facts, and each panel says its own. */
export const NothingSharedYet: Story = { render: board([], []) };

/** Finished: the conversation stays readable, the verbs go, and the add form
 *  is replaced by the sentence saying why. */
export const RoomFinished: Story = {
  render: board(DOCUMENTS, THREADS, { finished: true }),
};

/** A colleague who may read the deal but not write it — same board, no way to
 *  change it, one sentence saying so. */
export const ReadOnlySeat: Story = {
  render: board(DOCUMENTS, THREADS, { mayWrite: false }),
};

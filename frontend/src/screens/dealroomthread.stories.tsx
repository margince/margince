// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { DealRoomThread, ThreadVerbs } from "./dealroomthread";
import { ThreadComposer, ThreadList } from "./dealroomthread";
import { StoryProviders } from "./story-utils";
// The board's own sheet: `.thread`, `.thread-comment` and their siblings are
// reached BY CLASS from this file, and a story that imports only the thread
// module never reaches dealroomthreads.tsx, which loads it.
import "./dealroomthreads.css";

// One thread of a Deal Room's conversation, and the composer that opens
// another. Both sides render the same thread — the buyer and the seller read
// one conversation — so what differs between the frames is the VERBS: an
// absent `reply` is how a reader is told they may not write, and `refusal` is
// the sentence that says why.
//
// Every frame takes its threads and verbs as props, because that is how the
// board hands them over. There is no fetch here and nothing to stub.

const BUYER = { side: "buyer", name: "Dana Buyer" };
const SELLER = { side: "seller", name: "Lena Fischer" };

function thread(over: Partial<DealRoomThread> = {}): DealRoomThread {
  const id = over.id ?? "thread-1";
  return {
    id,
    room_id: "room-1",
    document_id: "doc-1",
    required_change: false,
    state: "open",
    author: BUYER,
    created_at: "2026-08-24T10:00:00Z",
    comments: [
      {
        id: `${id}-c1`,
        thread_id: id,
        body: "Can we align the payment schedule with our fiscal quarters?",
        author: BUYER,
        created_at: "2026-08-24T10:00:00Z",
      },
      {
        id: `${id}-c2`,
        thread_id: id,
        body: "Yes — v5 moves the milestones to the end of each quarter.",
        author: SELLER,
        created_at: "2026-08-24T16:29:00Z",
      },
    ],
    ...over,
  };
}

// The seller's hand: reply, resolve, open. The buyer's differs in exactly two
// places — no resolve, and `mayRequireChange` — which is why the two are verbs
// rather than a side flag.
const SELLER_VERBS: ThreadVerbs = {
  reply: async () => {},
  resolve: async () => {},
  open: async () => {},
  mayRequireChange: false,
};

const BUYER_VERBS: ThreadVerbs = {
  ...SELLER_VERBS,
  resolve: undefined,
  mayRequireChange: true,
};

function list(threads: readonly DealRoomThread[], verbs = SELLER_VERBS) {
  return () => (
    <StoryProviders>
      <ThreadList threads={threads} verbs={verbs} />
    </StoryProviders>
  );
}

function composer(
  verbs: ThreadVerbs,
  documentId: string | null,
  collapsible = false,
) {
  return () => (
    <StoryProviders>
      <ThreadComposer
        verbs={verbs}
        documentId={documentId}
        label="Ask about this document"
        collapsible={collapsible}
      />
    </StoryProviders>
  );
}

const meta: Meta<typeof ThreadList> = {
  title: "Records/Deal room/Thread",
  component: ThreadList,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof ThreadList>;

/** A question and its answer, each face the author's own and each time the
 *  reader's — a buyer holds no workspace zone, and "when did they write this"
 *  is a question about the reader's clock on either side. */
export const AQuestionAnswered: Story = { render: list([thread()]) };

/** The buyer's mark: a question saying the document needs changing rather than
 *  merely asking about it. Only the buyer may set it, and only the seller may
 *  answer by resolving — so the badge and the Resolve verb never come from the
 *  same side. */
export const RequiresAChange: Story = {
  render: list([thread({ required_change: true })]),
};

/** Answered and closed. A resolved thread stays readable and loses its reply
 *  box: the record of what was asked is the point, not the open count. */
export const Resolved: Story = {
  render: list([
    thread({ state: "resolved", resolved_at: "2026-08-24T16:30:00Z" }),
  ]),
};

/** A reader who may not write. The verbs are ABSENT rather than disabled, so
 *  the thread shows what was said and offers nothing that would be refused. */
export const WriteRefused: Story = {
  render: list([thread()], { mayRequireChange: false }),
};

/** The composer folded to one button on a document card, so a file nobody has
 *  asked about stays a document and not a form. */
export const ComposerFolded: Story = {
  render: composer(BUYER_VERBS, "doc-1", true),
};

/** The same composer open, with the buyer's "needs a change" mark beside the
 *  body — offered only on a document, because a room-wide question is about no
 *  one file. */
export const ComposerOnADocument: Story = {
  render: composer(BUYER_VERBS, "doc-1"),
};

/**
 * The composer a reader may not use. It draws the button in its refused state
 * carrying the reason, rather than nothing at all: a preview showing no reply
 * affordance reads exactly like a commenting seat, which is the opposite of
 * what a preview is for.
 */
export const ComposerRefused: Story = {
  render: composer(
    {
      mayRequireChange: false,
      refusal: "This room is closed. Its conversation stays readable.",
    },
    null,
  ),
};

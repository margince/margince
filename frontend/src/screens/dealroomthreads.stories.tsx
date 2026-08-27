// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Badge } from "../design-system/atoms";
import type {
  BoardDocument,
  BoardGroup,
  DealRoomThread,
  ThreadVerbs,
} from "./dealroomthreads";
import { DocumentBoard } from "./dealroomthreads";
import { StoryProviders } from "./story-utils";

// The room's documents and the conversation about them, drawn as ONE board:
// a thread sits inside the document it is about, so a reader never has to work
// out which document a question belongs to.
//
// The component takes everything as props — both sides of the room render it,
// and what the two sides KNOW differs — so these stories are the two sides and
// the states between them rather than a set of fetches. That is also why the
// verbs arrive as a bag of optional functions: an absent `reply` is how the
// board is told this reader may not write, and `refusal` is the sentence that
// says why. A board given `disabled` controls instead would be refusing without
// explaining.
//
// The second panel is the room-wide one, and its badge counts the threads that
// belong to no document ON SCREEN — including a thread about a document the
// seller has since removed, which still has to be answerable. That last case is
// `OrphanedThread`, and it is the reason the count is computed rather than
// handed in.

const GROUPS: readonly BoardGroup[] = [
  { key: "commercial", label: "Commercial" },
  { key: "technical", label: "Technical" },
];

const DOCUMENTS: readonly BoardDocument[] = [
  {
    id: "doc-1",
    groupKey: "commercial",
    title: "Commercial terms v4",
    meta: "commercial-terms-v4.pdf · Commercial",
    status: <Badge tone="success">Shared</Badge>,
  },
  {
    id: "doc-2",
    groupKey: "technical",
    title: "Implementation plan",
    meta: "implementation-plan.pdf · Technical",
    status: <Badge>Draft</Badge>,
  },
];

function author(side: "seller" | "buyer", name: string) {
  return { side, name };
}

function thread(overrides: Partial<DealRoomThread> = {}): DealRoomThread {
  const id = overrides.id ?? "thread-1";
  return {
    id,
    room_id: "room-1",
    document_id: null,
    required_change: false,
    state: "open",
    author: author("buyer", "Dana Buyer"),
    created_at: "2026-08-24T10:00:00Z",
    comments: [
      {
        id: `${id}-c1`,
        thread_id: id,
        body: "Can we align the payment schedule with our fiscal quarters?",
        author: author("buyer", "Dana Buyer"),
        created_at: "2026-08-24T10:00:00Z",
      },
    ],
    ...overrides,
  };
}

const SELLER_VERBS: ThreadVerbs = {
  reply: async () => {},
  resolve: async () => {},
  open: async () => {},
  mayRequireChange: false,
};

function board(
  threads: readonly DealRoomThread[],
  verbs: ThreadVerbs = SELLER_VERBS,
  documents: readonly BoardDocument[] = DOCUMENTS,
) {
  return () => (
    <StoryProviders>
      <DocumentBoard
        title="Documents"
        sub="What the buyer can read, and what they have asked about it."
        groups={GROUPS}
        documents={documents}
        threads={threads}
        verbs={verbs}
        empty="No document has been shared into this room yet."
      />
    </StoryProviders>
  );
}

const meta: Meta<typeof DocumentBoard> = {
  title: "Records/Deal room/Documents and threads",
  component: DocumentBoard,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof DocumentBoard>;

/** A room with documents and nothing asked yet. Both panels say so in their own
 *  words, because "no documents" and "no questions" are different facts. */
export const NothingAskedYet: Story = { render: board([]) };

/** A room with no documents at all: the empty sentence is the caller's, because
 *  what belongs in a room is the one thing only the caller knows. */
export const NoDocuments: Story = { render: board([], SELLER_VERBS, []) };

/** A question about a document, inside that document's card. */
export const ThreadOnADocument: Story = {
  render: board([thread({ document_id: "doc-1" })]),
};

/** A question about the room rather than any one document, in the room-wide
 *  panel with the count beside its title. */
export const RoomWideThread: Story = { render: board([thread()]) };

/**
 * A thread about a document the list no longer carries. It joins the room-wide
 * panel rather than disappearing: a seller who removed a draft before
 * publishing it must still be able to see and answer the question the buyer
 * asked about it, while the buyer — still on the last release — can.
 */
export const OrphanedThread: Story = {
  render: board([
    thread({ id: "thread-9", document_id: "doc-removed" }),
    thread({ id: "thread-1", document_id: "doc-1" }),
  ]),
};

/**
 * The buyer's mark: a question that says the document needs changing rather
 * than merely asking about it. Only the buyer may set it, which is why
 * `mayRequireChange` is a verb rather than a style.
 */
export const RequiresAChange: Story = {
  render: board(
    [
      thread({
        document_id: "doc-1",
        required_change: true,
        comments: [
          {
            id: "t1-c1",
            thread_id: "thread-1",
            body: "Clause 7 conflicts with our standard terms — this one we do need changed before signing.",
            author: author("buyer", "Dana Buyer"),
            created_at: "2026-08-24T10:00:00Z",
          },
        ],
      }),
    ],
    { ...SELLER_VERBS, resolve: undefined, mayRequireChange: true },
  ),
};

/** Answered and closed. A resolved thread stays readable — the record of what
 *  was asked is the point, not the open count. */
export const Resolved: Story = {
  render: board([
    thread({
      document_id: "doc-1",
      state: "resolved",
      resolved_at: "2026-08-24T16:30:00Z",
      comments: [
        {
          id: "t1-c1",
          thread_id: "thread-1",
          body: "Can we align the payment schedule with our fiscal quarters?",
          author: author("buyer", "Dana Buyer"),
          created_at: "2026-08-24T10:00:00Z",
        },
        {
          id: "t1-c2",
          thread_id: "thread-1",
          body: "Yes — v5 moves the milestones to the end of each quarter.",
          author: author("seller", "Lena Fischer"),
          created_at: "2026-08-24T16:29:00Z",
        },
      ],
    }),
  ]),
};

/**
 * A reader who may not write. The composer's verbs are ABSENT rather than
 * disabled, and `refusal` is the sentence that says why — a refused control
 * with no explanation reaches nobody, least of all a screen reader.
 */
export const WriteRefused: Story = {
  render: board([thread({ document_id: "doc-1" })], {
    mayRequireChange: false,
    refusal: "This room is closed. Its conversation stays readable.",
  }),
};

/**
 * A count wide enough to be written in a notation. The room panel's badge is a
 * magnitude, and four digits is where de-DE first groups — below it the figure
 * reads the same in every language and proves nothing.
 */
export const ManyRoomThreads: Story = {
  render: board(
    Array.from({ length: 1204 }, (_, index) =>
      thread({ id: `thread-${index}` }),
    ),
  ),
};

/** The same board in German: the longer copy, and the badge in the reader's own
 *  notation. */
export const ManyRoomThreadsGerman: Story = {
  render: () => (
    <StoryProviders locale="de">
      <DocumentBoard
        title="Dokumente"
        sub="Was der Käufer lesen kann — und was er dazu gefragt hat."
        groups={GROUPS}
        documents={DOCUMENTS}
        threads={Array.from({ length: 1204 }, (_, index) =>
          thread({ id: `thread-${index}` }),
        )}
        verbs={SELLER_VERBS}
        empty="In diesem Raum liegt noch kein Dokument."
      />
    </StoryProviders>
  ),
};

/** Long content: a comment nobody wrote in one line, which the thread has to
 *  wrap rather than clip. */
export const LongComment: Story = {
  render: board([
    thread({
      document_id: "doc-2",
      comments: [
        {
          id: "t1-c1",
          thread_id: "thread-1",
          body: "Our procurement team has asked whether the implementation plan can be split so the pilot plant runs ahead of the other three, because the retrofit window at Sindelfingen closes in March and we would rather not hold the whole programme to the slowest site. If that works, we would also want the acceptance criteria restated per site rather than per programme.",
          author: author("buyer", "Dana Buyer"),
          created_at: "2026-08-24T10:00:00Z",
        },
      ],
    }),
  ]),
};

/** At 390px the document cards stack and a thread's author, time and body
 *  cannot share a line. */
export const Phone: Story = {
  tags: ["uat-phone"],
  render: board([thread({ document_id: "doc-1" }), thread({ id: "thread-2" })]),
};

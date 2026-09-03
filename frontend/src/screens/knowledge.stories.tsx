// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { KnowledgeCard } from "./knowledge";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

// Settings → Knowledge → Document sets.
//
// Two properties carry this file, and both are about telling a reader the truth
// rather than about the happy path.
//
// The first is WHO IS LOOKING. Read on `knowledge_corpus` is the ASK, held by
// every role that reads records; administering the sets is admin/ops. So there
// are three principals here and each draws a different card — the administrator
// with every verb, the asker with the sets and NO verbs, and the reader who may
// not see the sets at all. The last one is `withheld`, never empty: an empty
// card would state something about the DATA, and the truth is only that these
// sets are not this reader's to see.
//
// The second is that the states a set or a document can be in which are not
// "fine" each say something DIFFERENT — being re-read, a file that could not be
// read at all, a set nothing has been filed in yet. The backend spent a whole
// design keeping those apart, and one badge for all three would undo it here.
//
// The word on screen is "document set", never "corpus": that noun is already
// taken by the VOICE corpus elsewhere in these same settings, and two things
// called a corpus in one settings area is the defect the screen comment refuses.

type Corpus = components["schemas"]["KnowledgeCorpus"];
type CorpusDocument = components["schemas"]["KnowledgeDocument"];

const SET_ID = "00000000-0000-4000-8000-0000000000a1";
const PRICE_BOOK_ID = "00000000-0000-4000-8000-0000000000a2";

// The three principals, spelled as grants rather than as role names: a fixture
// that said "admin" would prove the card asks for SOMETHING, and every wrong
// object looks exactly like the right one under a role that holds them all.
const administers = meRoute({
  knowledge_corpus: ["create", "read", "update", "delete"],
  knowledge_document: ["create", "read", "update", "delete"],
});
const asksOnly = meRoute({
  knowledge_corpus: ["read"],
  knowledge_document: ["read"],
});
// A real denial, which needs both halves said out loud: a read seat on a role
// that was never granted the object. `meRoute({})` alone would be an admin
// holding no grants, and the card would draw the same withheld body for the
// wrong reason.
const seesNothing = meRoute({}, { roles: ["rep"], seat: "read" });

const handbook: Corpus = {
  id: SET_ID,
  name: "Operator handbook",
  topic_statement:
    "How this installation is run, and what it promises the people who use it.",
  min_similarity: 0.35,
  default_ask: true,
  coverage: {
    documents_total: 12,
    chunks_total: 1_180,
    chunks_embedded: 1_180,
  },
  created_at: "2026-08-01T00:00:00Z",
};

// A set whose coverage is short of its own total — the ordinary mid-ingest
// reading, and the one a reader compares against the badges below it.
const priceBook: Corpus = {
  id: PRICE_BOOK_ID,
  name: "Price book",
  topic_statement:
    "What each line costs, and which discounts are ours to give.",
  min_similarity: 0.42,
  default_ask: false,
  coverage: { documents_total: 4, chunks_total: 210, chunks_embedded: 96 },
  created_at: "2026-08-14T00:00:00Z",
};

function filed(
  id: string,
  filename: string,
  status: CorpusDocument["ingest_status"],
): CorpusDocument {
  return {
    id,
    corpus_id: SET_ID,
    filename,
    content_type: "text/markdown",
    byte_size: 4_096,
    ingest_status: status,
    chunk_count: 12,
    created_at: "2026-08-01T00:00:00Z",
  };
}

const searchable = filed(
  "00000000-0000-4000-8000-0000000000b1",
  "operating.md",
  "done",
);

const refused: CorpusDocument = {
  ...filed("00000000-0000-4000-8000-0000000000b4", "prices-2024.pdf", "failed"),
  content_type: "application/pdf",
  // The reason a failed ingest carries. A set quietly short of a file nobody
  // can name answers worse than an empty one, because it still answers.
  ingest_detail: "No text could be extracted: the pages are scanned images.",
};

/** The card, with the set list answered for one principal. */
function sets(
  session: () => Response,
  items: readonly Corpus[],
  extra: RouteMap = {},
): RouteMap {
  return {
    "GET /me": session,
    "GET /knowledge/corpora": () => jsonResponse({ items }),
    ...extra,
  };
}

/** The documents of the handbook set, which is the only set a frame with a
 *  `play()` below carries — one set means one "Show documents" and one
 *  "Archive set", so a canvas lookup names a single control rather than
 *  rejecting on an ambiguous match. */
function documents(items: readonly CorpusDocument[]): RouteMap {
  return {
    [`GET /knowledge/corpora/${SET_ID}/documents`]: () =>
      jsonResponse({ items }),
  };
}

function card(routes: RouteMap) {
  return () => {
    installFetchStub(routes);
    return (
      <StoryProviders>
        <KnowledgeCard />
      </StoryProviders>
    );
  };
}

/** Unfold one set's documents. The button carries the copy the screen renders,
 *  not a fixture's own word. */
const showDocuments = async ({
  canvasElement,
}: {
  canvasElement: HTMLElement;
}) => {
  await userEvent.click(
    await within(canvasElement).findByRole("button", {
      name: "Show documents",
    }),
  );
};

const meta: Meta<typeof KnowledgeCard> = {
  title: "Settings/Admin settings/Knowledge/Document sets",
  component: KnowledgeCard,
};
export default meta;
type Story = StoryObj<typeof KnowledgeCard>;

// The administrator's card: both sets with their coverage line, the archive verb
// on each, and the form for a new one under the panel. The two coverage lines
// differ on purpose — one fully embedded, one still short — because that figure
// is the only place a reader learns a set is not yet answerable in full.
export const Administered: Story = {
  render: card(sets(administers, [handbook, priceBook])),
};

// The seeded posture, and the one this file exists for: a reader who may ASK the
// sets but not administer them sees every set and NOT ONE verb. No archive
// button, no new-set form, no dropzone — a control the server would refuse is a
// worse answer than no control, because pressing it is how the reader finds out.
export const AskerSeesNoVerbs: Story = {
  render: card(sets(asksOnly, [handbook, priceBook])),
};

// Withheld, not empty. The card keeps its title and its subtitle and loses the
// list: an empty card would say this workspace has no document sets, which is a
// statement about the data that this reader is in no position to be given.
export const Withheld: Story = {
  render: card(sets(seesNothing, [])),
};

// A set being re-read. Every ask against it answers "not ready" until the sweep
// finishes, and there is nothing for the reader to do but wait — which is a
// different sentence from "this set is not ready", and the notice says so.
export const BeingReread: Story = {
  render: card(sets(administers, [{ ...handbook, reindexing: true }])),
};

// The four ingest states in one list, which is the whole reason they are four.
// Only `failed` is bad news and it carries the reason it failed; `queued` and
// `running` are the same wait in two words; `done` is deliberately quiet,
// because a set where everything worked should not be a wall of green pills.
export const DocumentsFiled: Story = {
  render: card(
    sets(
      administers,
      [handbook],
      documents([
        searchable,
        filed(
          "00000000-0000-4000-8000-0000000000b2",
          "escalations.md",
          "running",
        ),
        filed(
          "00000000-0000-4000-8000-0000000000b3",
          "onboarding.md",
          "queued",
        ),
        refused,
      ]),
    ),
  ),
  play: showDocuments,
};

// A set created and not yet filled. This is the one absence on the screen that
// IS honest — the set list itself never draws an empty state, because every
// installation is filed with the operator handbook at boot and "no document
// sets" could then only mean a reconciliation that did not run.
export const NothingFiledYet: Story = {
  render: card(
    sets(
      administers,
      [
        {
          ...handbook,
          coverage: { documents_total: 0, chunks_total: 0, chunks_embedded: 0 },
        },
      ],
      documents([]),
    ),
  ),
  play: showDocuments,
};

// Archiving a set is not reversible from this screen, so it asks first. The
// dialog names the set's own act in the confirm button rather than "OK": a
// reader who has stopped reading presses the verb they came for, and it should
// be the one they meant.
export const ArchiveConfirm: Story = {
  render: card(sets(administers, [handbook])),
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", { name: "Archive set" }),
    );
  },
};

// The same question one level down, and the level that matters more: a document
// deleted here is passages the next answer will not have. One document in the
// set, so the row's verb is unambiguous.
export const DeleteDocumentConfirm: Story = {
  render: card(sets(administers, [handbook], documents([searchable]))),
  play: async (context) => {
    await showDocuments(context);
    await userEvent.click(
      await within(context.canvasElement).findByRole("button", {
        name: "Delete",
      }),
    );
  },
};

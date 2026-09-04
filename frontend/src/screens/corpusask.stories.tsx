// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import type { components } from "../api/schema";
import { CorpusAskCard } from "./corpusask";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

// Ask your documents: the one question box in the product whose search is
// BOUNDED, which is what lets it refuse instead of guessing.
//
// So the outcomes get one story each, and they are here rather than folded
// into two because the whole endpoint exists to keep them apart: `not_covered`
// is about the QUESTION and quotes the set's own topic statement back — the
// only thing on screen telling the reader what the set is FOR, read at their
// least patient moment — `not_ready` is about the SET, and
// `retrieval_unavailable` says nothing was searched at all. Drawn as one bare
// "no answer" they would send the reader after the wrong thing, and nothing in
// the render gate would notice.
//
// `CorpusAskCard` renders NOTHING without `knowledge_corpus:read` and nothing
// when the workspace filed no set, so both are routed in every story: either
// omission draws an empty page under a name that claims an answer.

type Answer = components["schemas"]["KnowledgeAnswer"];
type Corpus = components["schemas"]["KnowledgeCorpus"];

const SET_ID = "00000000-0000-4000-8000-0000000000a1";

const SET: Corpus = {
  id: SET_ID,
  name: "How we operate",
  topic_statement: "How this product is operated, day to day.",
  min_similarity: 0.35,
  default_ask: true,
  coverage: {
    documents_total: 12,
    chunks_total: 1_180,
    chunks_embedded: 1_180,
  },
  created_at: "2026-08-01T00:00:00Z",
};

// A second set the workspace filed but did not mark: `preferredSet` still opens
// on the marked one, and the picker only exists at all once there are two.
const OTHER_SET: Corpus = {
  ...SET,
  id: "00000000-0000-4000-8000-0000000000a2",
  name: "Pricing and plans",
  topic_statement: "What each plan costs and what it includes.",
  default_ask: false,
};

const ANSWERED: Answer = {
  outcome: "answered",
  generated_by: "model",
  corpus: {
    id: SET_ID,
    name: SET.name,
    topic_statement: SET.topic_statement,
  },
  coverage: SET.coverage,
  claims: [
    {
      chunk_id: "00000000-0000-4000-8000-0000000000c1",
      document_id: "00000000-0000-4000-8000-0000000000b1",
      document_name: "operating.md",
      line: 14,
      column: 3,
      text: "Captured messages are kept for 400 days.",
      quote: "kept for 400 days from the day they arrive",
    },
  ],
};

// A refusal wrote no prose, so it is `deterministic` — the badge is about who
// produced a sentence, and here nobody did.
const NOT_COVERED: Answer = {
  ...ANSWERED,
  outcome: "not_covered",
  generated_by: "deterministic",
  claims: [],
};

// Mid-ingest, and the counts are the whole point: "try again shortly" is a
// claim the reader can check only if the card says how far the reading got.
const NOT_READY: Answer = {
  ...NOT_COVERED,
  outcome: "not_ready",
  coverage: { documents_total: 12, chunks_total: 1_180, chunks_embedded: 407 },
};

const RETRIEVAL_UNAVAILABLE: Answer = {
  ...NOT_COVERED,
  outcome: "retrieval_unavailable",
};

// The nearest passages, carrying no written sentence, because nothing read
// them. This is the one outcome that is neither an answer nor a refusal.
const UNREVIEWED: Answer = {
  ...ANSWERED,
  outcome: "unreviewed",
  generated_by: "deterministic",
  claims: [
    {
      chunk_id: "00000000-0000-4000-8000-0000000000c1",
      document_id: "00000000-0000-4000-8000-0000000000b1",
      document_name: "operating.md",
      line: 14,
      column: 3,
      quote: "kept for 400 days from the day they arrive",
    },
  ],
};

const ASK_ROUTE = `POST /knowledge/corpora/${SET_ID}/ask`;

function askCard(
  question: string,
  reply: () => Response | Promise<Response>,
  sets: readonly Corpus[] = [SET],
) {
  const routes: RouteMap = {
    "GET /me": meRoute({ knowledge_corpus: ["read"] }),
    "GET /knowledge/corpora": () => jsonResponse({ items: sets }),
    [ASK_ROUTE]: reply,
  };
  return () => {
    installFetchStub(routes);
    return (
      <StoryProviders>
        {/* The card sits in a one-column reading width on its screen; at the
            gallery's full width the passage lines run far longer than any
            reader ever meets. */}
        <div style={{ maxWidth: 720 }}>
          <CorpusAskCard carriedQuestion={question} />
        </div>
      </StoryProviders>
    );
  };
}

// The question is carried in, so pressing Ask is the whole interaction.
//
// Waiting for the button to lose `disabled` is the part that matters: the set
// is chosen in a passive effect once the list arrives, so a press landing
// before it has no corpus to ask — and a click the card refused is
// indistinguishable on screen from one still in flight.
async function pressAsk(canvasElement: HTMLElement): Promise<HTMLElement> {
  const submit = await within(canvasElement).findByRole("button", {
    name: "Ask",
  });
  await waitFor(() => expect(submit).not.toHaveAttribute("disabled"));
  await userEvent.click(submit);
  return submit;
}

// Then wait for the answer ITSELF rather than for the click to return: the
// reply commits a microtask later, so a capture taken when play resolves shows
// the empty form under a story named for what the card said.
function askAndSee(settled: RegExp) {
  return async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await pressAsk(canvasElement);
    await within(canvasElement).findByText(settled);
  };
}

const meta: Meta<typeof CorpusAskCard> = {
  title: "Records/Ask your documents",
  component: CorpusAskCard,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof CorpusAskCard>;

// The answer, whole: the sentence, WHO wrote it, the verbatim quote under it,
// and the file plus the line the quote sits on. Every one of those is what
// makes the sentence checkable, and a card that dropped any of them would
// still look like an answer.
export const Answered: Story = {
  render: askCard("how long are captured messages kept", () =>
    jsonResponse(ANSWERED),
  ),
  play: askAndSee(/Captured messages are kept for 400 days/),
};

// The same answer on the dark ground. The "written from the passages" badge is
// the indigo provenance mark, whose ground and text are both color-mix() of
// tokens that lift with the dark accent — a badge that reads as a claim about a
// model in light can go illegible here and nothing else on the card would.
export const AnsweredDark: Story = {
  ...Answered,
  globals: { theme: "dark" },
};

// The refusal that is about the QUESTION: searched in full, nothing close
// enough — and the set's topic statement quoted back, because a reader who has
// just been refused is the reader who most needs to know what the set covers.
export const NotCovered: Story = {
  render: askCard("what does the Professional plan cost", () =>
    jsonResponse(NOT_COVERED),
  ),
  play: askAndSee(/Not covered by this set/),
};

// The refusal about the SET: it is still being read, and nothing is wrong with
// the question. Drawn as an info callout with the passage counts rather than as
// the not-covered plate, because a reader told "not covered" goes looking for a
// document to file when the truth is that the one they need is already there.
export const NotReady: Story = {
  render: askCard("how long are captured messages kept", () =>
    jsonResponse(NOT_READY),
  ),
  play: askAndSee(/not finished being read/),
};

// The refusal about the INSTALLATION: no search lane is bound, so nothing was
// searched at all. Neither the question nor the set is at fault, and a card
// that said either would send the reader after the wrong thing.
export const RetrievalUnavailable: Story = {
  render: askCard("how long are captured messages kept", () =>
    jsonResponse(RETRIEVAL_UNAVAILABLE),
  ),
  play: askAndSee(/Nothing was searched/),
};

// Neither an answer nor a refusal: the search found these passages and nothing
// judged whether they answer the question. The caveat LEADS the panel, because
// a passage sitting under a heading is read as an answer to it.
export const Unreviewed: Story = {
  render: askCard("what is the boiling point of nitrogen", () =>
    jsonResponse(UNREVIEWED),
  ),
  play: askAndSee(/Nothing has read them/),
};

// Mid-ask. The pressed button keeps its label and its focus and says it is busy
// beside it; swapping the word or disabling the control would move the reader
// off the one thing about to tell them something.
export const Asking: Story = {
  render: askCard(
    "how long are captured messages kept",
    () => new Promise<Response>(() => {}),
  ),
  play: async ({ canvasElement }) => {
    const submit = await pressAsk(canvasElement);
    await waitFor(() => expect(submit).toHaveAttribute("aria-busy", "true"));
  },
};

// Two sets, which is the only condition under which the picker exists — a
// workspace with one is never asked which. It matters that the picker is drawn
// ABOVE the question and pre-chosen: the box is never offered with no set
// selected, so a reader who arrived carrying a question can just ask it.
export const WhichSet: Story = {
  render: askCard(
    "how long are captured messages kept",
    () => jsonResponse(ANSWERED),
    [SET, OTHER_SET],
  ),
};

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { within } from "storybook/test";
import type { components } from "../api/schema";
import { ASK_QUESTION_PARAM } from "../app/palette";
import { AskAiScreen } from "./ai";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// AskAiScreen (B-EP09.12c, 03b): the BYO-agent surface. It states the two-tier
// contract and connects to no chat backend, and the screen never pretends
// otherwise — the one thing on it a reader can actually ask is the bounded
// question box over a document set.
//
// That box is what the session below is about. `CorpusAskCard` reads
// `knowledge_corpus:read` off /me and renders NOTHING without it, so a story
// routed with any other grant would draw the tier contract alone under a name
// that claims the ask surface. The document set is routed for the same reason:
// the card stands down when the workspace has none, and "From the palette"
// would then be a story about a question with nowhere to land.
//
// The screen prints no title of its own: the app shell mints the one h1 for this
// route ("Ask Margince") and now carries the subtitle under it too. A story
// renders the screen without that shell, so the heading is absent here on
// purpose rather than missing.

type Corpus = components["schemas"]["KnowledgeCorpus"];

const SET: Corpus = {
  id: "00000000-0000-4000-8000-0000000000a1",
  name: "Everything",
  topic_statement:
    "What this company sells, who it sells to, and what it has already said.",
  min_similarity: 0.35,
  default_ask: true,
  coverage: {
    documents_total: 42,
    chunks_total: 1_180,
    chunks_embedded: 1_180,
  },
  created_at: "2026-08-01T00:00:00Z",
};

// What the carried question lands on, so the story shows the ANSWER rather than
// a box with a sentence in it: arriving with a question is the whole
// interaction now, and a story that stopped at the filled box would be a story
// about the shape this surface used to have.
const ANSWER = {
  outcome: "answered" as const,
  generated_by: "model" as const,
  corpus: { id: SET.id, name: SET.name, topic_statement: SET.topic_statement },
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

function askSurface(carried: string | null) {
  return () => {
    // The palette hands its question over in the ADDRESS, and the screen asks
    // it and empties the dial. Written on every story rather than only when
    // there is one, so a story never inherits the address the story before it
    // left behind.
    globalThis.location.hash = carried
      ? `#/ai?${ASK_QUESTION_PARAM}=${encodeURIComponent(carried)}`
      : "#/ai";
    installFetchStub({
      "GET /me": meRoute({ knowledge_corpus: ["read"] }),
      "GET /knowledge/corpora": () => jsonResponse({ items: [SET] }),
      [`POST /knowledge/corpora/${SET.id}/ask`]: () => jsonResponse(ANSWER),
    });
    return (
      <StoryProviders>
        <AskAiScreen />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof AskAiScreen> = {
  title: "Records/Ask Margince",
  component: AskAiScreen,
};
export default meta;
type Story = StoryObj<typeof AskAiScreen>;

// Opened from the rail: nothing was typed on the way in, so the box is empty
// and the tier contract under it is the rest of the surface.
export const Cold: Story = { render: askSurface(null) };

// Opened from the command palette, which hands the typed question over in the
// address. It is ASKED on arrival rather than reprinted above a box the reader
// must press — they asked it when they typed it.
export const FromThePalette: Story = {
  render: askSurface("which accounts went quiet since the trade fair?"),
  play: async ({ canvasElement }) => {
    await within(canvasElement).findByText(
      /Captured messages are kept for 400 days/,
    );
  },
};

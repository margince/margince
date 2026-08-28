// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { ASK_QUERY_KEY } from "../app/palette";
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

function askSurface(carried: string | null) {
  return () => {
    // The palette hands its question over in session storage, and the screen
    // reads it ONCE and clears it. Written or removed explicitly, so a story
    // never inherits what the story before it left behind.
    if (carried === null) {
      sessionStorage.removeItem(ASK_QUERY_KEY);
    } else {
      sessionStorage.setItem(ASK_QUERY_KEY, carried);
    }
    installFetchStub({
      "GET /me": meRoute({ knowledge_corpus: ["read"] }),
      "GET /knowledge/corpora": () => jsonResponse({ items: [SET] }),
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

// Opened from the command palette, which hands the typed question over in
// session storage. It goes straight into the ask box rather than being reprinted
// above one — a reader who typed a question ASKED it.
export const FromThePalette: Story = {
  render: askSurface("which accounts went quiet since the trade fair?"),
};

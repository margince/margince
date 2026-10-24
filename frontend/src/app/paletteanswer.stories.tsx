// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  type HandbookAnswer,
  NoDocumentSetsError,
} from "../features/voice-knowledge/corpusaskqueries";
import { StoryProviders, stubWithSession } from "../testing/story-utils";
import { PaletteAnswer } from "./paletteanswer";

// The answer a question in the search box gets, in each shape a reader meets.
// It is written from the handbook, or from the default set when the handbook
// has nothing. It can also be covered by neither, still pending, or failed.
const meta: Meta<typeof PaletteAnswer> = {
  title: "Shell/Command palette answer",
  component: PaletteAnswer,
};
export default meta;
type Story = StoryObj<typeof PaletteAnswer>;

const coverage = {
  documents_total: 34,
  chunks_total: 410,
  chunks_embedded: 410,
  embed_state: "complete" as const,
};

const handbook = {
  id: "00000000-0000-4000-8000-0000000000a1",
  name: "Margince handbook",
  topic_statement: "How Margince works.",
  shipped: true,
};

const playbook = {
  id: "00000000-0000-4000-8000-0000000000a2",
  name: "Sales playbook",
  topic_statement: "How our team sells.",
  shipped: false,
};

const timeZoneClaim = {
  chunk_id: "00000000-0000-4000-8000-0000000000c1",
  document_id: "00000000-0000-4000-8000-0000000000b1",
  document_name: "your-own-settings.md",
  line: 156,
  column: 1,
  text: "You change your own time zone on the Meetings page.",
  quote: "open Settings → **Meetings**",
  section: { title: "How do I change my time zone?", line: 154 },
  opens: { settings_page: "meetings" },
};

const answered: HandbookAnswer = {
  question: "How do I change my time zone?",
  asked: [handbook.name],
  answer: {
    outcome: "answered",
    generated_by: "model",
    corpus: handbook,
    coverage,
    summary:
      "Open Settings → Meetings and pick a place in the Timezone list. [1]",
    claims: [timeZoneClaim],
  },
};

function frame(props: Parameters<typeof PaletteAnswer>[0]) {
  stubWithSession({}, { knowledge_corpus: ["read"] });
  return (
    <StoryProviders>
      <div className="palette" style={{ position: "static" }}>
        <PaletteAnswer {...props} />
      </div>
    </StoryProviders>
  );
}

const base = { pending: false, error: null, onLeave: () => undefined };

export const FromTheHandbook: Story = {
  render: () => frame({ ...base, result: answered }),
};

export const FromTheDefaultSet: Story = {
  render: () =>
    frame({
      ...base,
      result: {
        question: "What discount may I give?",
        asked: [handbook.name, playbook.name],
        answer: {
          ...answered.answer,
          corpus: playbook,
          summary: "Up to 10% without approval. [1]",
          claims: [
            {
              ...timeZoneClaim,
              document_name: "discounts.md",
              text: "Up to 10% needs no approval.",
              quote: "up to 10%",
              section: undefined,
              opens: undefined,
            },
          ],
        },
      },
    }),
};

export const NotCovered: Story = {
  render: () =>
    frame({
      ...base,
      result: {
        question: "Can Margince book my flights?",
        asked: [handbook.name, playbook.name],
        answer: {
          ...answered.answer,
          outcome: "not_covered",
          corpus: playbook,
          summary:
            "Your documents do not cover travel booking. They cover meetings and booking links.",
          claims: [],
        },
      },
    }),
};

export const Asking: Story = {
  render: () => frame({ ...base, pending: true, result: undefined }),
};

export const Failed: Story = {
  render: () =>
    frame({
      ...base,
      error: new Error("The service is not reachable."),
      result: undefined,
    }),
};

export const NoDocumentSets: Story = {
  render: () =>
    frame({ ...base, error: new NoDocumentSetsError(), result: undefined }),
};

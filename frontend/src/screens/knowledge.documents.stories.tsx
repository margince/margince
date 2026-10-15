// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { Panel } from "../design-system/panel";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { DocumentList } from "./knowledge.documents";

// One set's documents as its row opens them, A to Z. A document still waiting
// or one that failed carries a badge. So does each one not yet searchable,
// unless the set states that for every row.

type CorpusDocument = components["schemas"]["KnowledgeDocument"];
type EmbedState = components["schemas"]["KnowledgeEmbedState"];

const SET_ID = "00000000-0000-4000-8000-0000000000a1";

function filed(
  id: string,
  filename: string,
  status: CorpusDocument["ingest_status"],
  chunks: number,
): CorpusDocument {
  return {
    id,
    corpus_id: SET_ID,
    filename,
    title: null,
    shipped: false,
    content_type: "text/markdown",
    byte_size: chunks * 512,
    ingest_status: status,
    chunk_count: chunks,
    chunks_embedded: chunks,
    embed_state: status === "done" ? "complete" : "indexing",
    created_at: "2026-08-01T00:00:00Z",
  };
}

const DOCUMENTS: CorpusDocument[] = [
  filed("00000000-0000-4000-8000-0000000000b1", "operating.md", "done", 43),
  // A handbook page: named by its heading, its file under it, and no menu.
  {
    ...filed(
      "00000000-0000-4000-8000-0000000000b5",
      "approvals.md",
      "done",
      18,
    ),
    title: "Approving agent proposals",
    shipped: true,
  },
  {
    ...filed("00000000-0000-4000-8000-0000000000b6", "pricing.md", "done", 9),
    chunks_embedded: 4,
    embed_state: "indexing",
  },
  filed("00000000-0000-4000-8000-0000000000b2", "escalations.md", "running", 0),
  filed("00000000-0000-4000-8000-0000000000b3", "onboarding.md", "queued", 0),
  {
    ...filed(
      "00000000-0000-4000-8000-0000000000b4",
      "prices-2024.csv",
      "failed",
      0,
    ),
    ingest_detail: "No text could be extracted: the file is empty.",
  },
];

function story(
  items: CorpusDocument[],
  canManage: boolean,
  setState: EmbedState = "indexing",
) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({
        knowledge_corpus: ["create", "read", "update", "delete"],
        knowledge_document: ["create", "read", "update", "delete"],
      }),
      [`GET /knowledge/corpora/${SET_ID}/documents`]: () =>
        jsonResponse({ items }),
    });
    return (
      <StoryProviders>
        <Panel title="Document sets">
          <DocumentList
            corpusId={SET_ID}
            setName="Operator handbook"
            setState={setState}
            shipped={false}
            canManage={canManage}
            afterDelete={() => null}
          />
        </Panel>
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof DocumentList> = {
  title: "Settings/Data/Knowledge/Documents in a set",
  component: DocumentList,
};
export default meta;
type Story = StoryObj<typeof DocumentList>;

export const Filed: Story = { render: story(DOCUMENTS, true) };

export const FiledAtPhoneWidth: Story = {
  ...Filed,
  tags: ["uat-phone"],
};

export const FiledDark: Story = {
  ...Filed,
  globals: { theme: "dark" },
};

// A seat that may ask but not administer: the rows stay, the menu goes.
export const ReadOnly: Story = { render: story(DOCUMENTS, false) };

export const NothingFiled: Story = { render: story([], true) };

// No embedding model bound: the set says so for every row, so no row repeats it.
export const NotSearchable: Story = {
  render: story(
    DOCUMENTS.map((doc) => ({
      ...doc,
      chunks_embedded: 0,
      embed_state: "no_embedding_model",
    })),
    true,
    "no_embedding_model",
  ),
};

export const DeleteConfirm: Story = {
  render: story(DOCUMENTS, true),
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", {
        name: "Actions for operating.md",
      }),
    );
    const page = within(canvasElement.ownerDocument.body);
    await userEvent.click(page.getByRole("button", { name: "Delete" }));
    await page.findByRole("dialog");
  },
};

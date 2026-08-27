// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { AddDocument } from "./dealroomdocuments";
import {
  installFetchStub,
  jsonResponse,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

type DealRoom = components["schemas"]["DealRoom"];
type DealDocument = components["schemas"]["DealDocument"];

// The room the form attaches to. Only the two fields AddDocument reads are
// meaningful — the id it posts to and the deal whose Files area it offers.
const ROOM: DealRoom = {
  id: "11111111-1111-4111-8111-111111111111",
  deal_id: "22222222-2222-4222-8222-222222222222",
  title: "Acme expansion",
  state: "live",
  source: "admin",
  captured_by: "33333333-3333-4333-8333-333333333333",
  created_at: "2026-05-04T09:00:00Z",
  updated_at: "2026-05-04T09:00:00Z",
};

function file(id: string, filename: string, title?: string): DealDocument {
  return {
    attachment: {
      id,
      entity_type: "deal",
      entity_id: ROOM.deal_id,
      filename,
      title,
      source: "upload",
      captured_by: "33333333-3333-4333-8333-333333333333",
      created_at: "2026-05-04T09:00:00Z",
    },
    hidden: false,
  };
}

// What the deal's Files area answers with: uploads and the files its emails
// carried. The server is what excludes hidden ones — this list is already the
// shareable set.
const FILES: readonly DealDocument[] = [
  file("a-1", "acme-msa-v4.pdf", "Commercial terms v4"),
  file("a-2", "security-questionnaire.xlsx"),
  file("a-3", "implementation-plan.pdf", "Implementation plan"),
];

const DEAL_DOCUMENTS = `GET /deals/${ROOM.deal_id}/documents`;

function addDocument(routes: RouteMap, refusal?: string) {
  return () => {
    installFetchStub(routes);
    return (
      <StoryProviders>
        <AddDocument room={ROOM} refusal={refusal} />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof AddDocument> = {
  title: "Records/Deal room/Add a document",
  component: AddDocument,
};
export default meta;
type Story = StoryObj<typeof AddDocument>;

/**
 * The ordinary case: pick one of the deal's files, say which group it belongs
 * in, add it. The add verb stays disabled until a file is picked, because a
 * group on its own attaches nothing.
 */
export const PickAFile: Story = {
  render: addDocument({
    [DEAL_DOCUMENTS]: () =>
      jsonResponse({
        data: FILES,
        page: { next_cursor: null, has_more: false },
      }),
  }),
};

/**
 * The deal carries no shareable file yet. The picker says so in its own face
 * and disables itself — an empty list a reader can open reads as a loading
 * state that never finished.
 */
export const NoFilesToShare: Story = {
  render: addDocument({
    [DEAL_DOCUMENTS]: () =>
      jsonResponse({ data: [], page: { next_cursor: null, has_more: false } }),
  }),
};

/**
 * Refused before the form exists. A refusal is a SENTENCE rather than a
 * disabled form: a reader who may not add documents needs to know why, and a
 * greyed-out picker says only that something is broken.
 */
export const Refused: Story = {
  render: addDocument(
    {},
    "This room is closed, so its documents can no longer change.",
  ),
};

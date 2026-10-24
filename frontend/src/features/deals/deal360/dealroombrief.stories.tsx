// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../../testing/story-utils";
import { RoomBrief } from "./dealroombrief";

// The room brief in each of its states: written by the model, composed from
// the records, and still being read. Each sentence names what it came from.

const meta: Meta = {
  title: "Records/Deal 360/Room brief",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const AT = "2026-08-24T14:02:00Z";

function stub(generatedBy: "model" | "deterministic", pending = false) {
  installFetchStub({
    "GET /me": meRoute({ deal_room: ["read"] }),
    "GET /deal-rooms/room-1/participants": () =>
      jsonResponse({
        data: [
          {
            id: "part-1",
            room_id: "room-1",
            full_name: "Dana Buyer",
            email: "dana.buyer@nordwind.example",
            capability: "comment",
            delivery_state: "delivered",
            has_signed_in: true,
            source: "manual",
            created_at: AT,
            updated_at: AT,
          },
        ],
        page: { next_cursor: null },
      }),
    "GET /deal-rooms/room-1/documents": () =>
      jsonResponse({
        data: [
          {
            id: "doc-1",
            room_id: "room-1",
            attachment_id: "att-1",
            group_key: "legal",
            title: "MSA redline",
            filename: "msa-redline.docx",
            position: 0,
            source: "manual",
            created_at: AT,
            updated_at: AT,
          },
        ],
        page: { next_cursor: null },
      }),
    "GET /deal-rooms/room-1/threads": () =>
      jsonResponse({
        data: [
          {
            id: "thread-1",
            room_id: "room-1",
            document_id: "doc-1",
            state: "open",
            required_change: false,
            author: { name: "Dana Buyer", side: "buyer" },
            created_at: AT,
            comments: [],
          },
        ],
        page: { next_cursor: null },
      }),
    "GET /deal-rooms/room-1/brief": () =>
      pending
        ? new Promise<Response>(() => undefined)
        : jsonResponse({
            room_id: "room-1",
            generated_at: AT,
            generated_by: generatedBy,
            sentences: [
              {
                text: "Dana Buyer is waiting on your answer about the liability cap.",
                nature: "fact",
                evidence: [
                  { entity_type: "thread", entity_id: "thread-1" },
                  { entity_type: "participant", entity_id: "part-1" },
                ],
              },
              {
                text: "Answer it before the redline goes to legal.",
                nature: "recommendation",
                evidence: [{ entity_type: "thread", entity_id: "thread-1" }],
              },
            ],
          }),
  });
}

function brief(generatedBy: "model" | "deterministic", pending = false) {
  stub(generatedBy, pending);
  return (
    <StoryProviders>
      <RoomBrief roomId="room-1" />
    </StoryProviders>
  );
}

/** Written by the model: the AI panel with its sources under each sentence. */
export const WrittenByMargince: Story = { render: () => brief("model") };

/** Composed from the records when no model answered, and labelled so. */
export const FromTheRecords: Story = { render: () => brief("deterministic") };

/** While the brief is being written. */
export const Pending: Story = { render: () => brief("model", true) };

export const WrittenByMarginceDark: Story = {
  globals: { theme: "dark" },
  render: () => brief("model"),
};

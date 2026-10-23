// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent, within } from "storybook/test";
import type { components } from "../../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../../testing/story-utils";
import { DealRoomTab, RoomText, ViewAsBuyerButton } from "./dealroomtab";

// The deal's own door into its room: the band, what the room still lacks,
// the brief, the documents beside the conversation, and who may enter. The
// whole-tab stories walk the room's states; the text panel below is the room
// page's, which the tab shares the rest with.

type DealRoom = components["schemas"]["DealRoom"];

const ROOM: DealRoom = {
  id: "room-1",
  deal_id: "deal-1",
  title: "Nordwind — Fleet telematics",
  welcome_message:
    "Everything we agreed is in here. Add a comment on anything you want changed.",
  state: "live",
  source: "manual",
  captured_by: "u-me",
  version: 1,
  created_at: "2026-08-22T09:00:00Z",
  updated_at: "2026-08-24T14:02:00Z",
};

const meta: Meta = {
  title: "Records/Deal 360/Deal room tab",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

function room(over: Partial<DealRoom> = {}): DealRoom {
  return { ...ROOM, ...over };
}

const PARTICIPANT = {
  id: "part-1",
  room_id: ROOM.id,
  full_name: "Dana Buyer",
  email: "dana.buyer@nordwind.example",
  capability: "read",
  delivery_state: "delivered",
  has_signed_in: true,
  last_seen_at: "2026-08-24T14:02:00Z",
  source: "manual",
  captured_by: "u-me",
  created_at: "2026-08-22T09:10:00Z",
  updated_at: "2026-08-24T14:02:00Z",
};

function stub() {
  installFetchStub({
    "GET /me": meRoute({ deal_room: ["read", "update"] }),
    "GET /deal-rooms/room-1/participants": () =>
      jsonResponse({
        data: [
          {
            id: "part-1",
            room_id: ROOM.id,
            full_name: "Dana Buyer",
            email: "dana.buyer@nordwind.example",
            capability: "read",
            delivery_state: "delivered",
            has_signed_in: true,
            last_seen_at: "2026-08-24T14:02:00Z",
            source: "manual",
            captured_by: "u-me",
            created_at: "2026-08-22T09:10:00Z",
            updated_at: "2026-08-24T14:02:00Z",
          },
        ],
        page: { next_cursor: null },
      }),
  });
}

const DOC_ID = "doc-1";
const THREAD_ID = "thread-1";

// The room as a rep finds it: one buyer in, one document shared, and that
// buyer waiting on an answer about it.
function fullStub(state: DealRoom["state"], filled: boolean) {
  stub();
  const at = "2026-08-24T14:02:00Z";
  installFetchStub({
    "GET /me": meRoute({ deal_room: ["read", "update"] }),
    "GET /users": () => jsonResponse({ data: [], page: { next_cursor: null } }),
    "GET /deal-rooms": () =>
      jsonResponse({
        data: [
          room({
            state,
            welcome_message: filled ? ROOM.welcome_message : null,
          }),
        ],
        page: { next_cursor: null },
      }),
    "GET /deals/deal-1/documents": () =>
      jsonResponse({ data: [], page: { next_cursor: null } }),
    "GET /deal-rooms/room-1/participants": () =>
      jsonResponse({
        data: filled ? [PARTICIPANT] : [],
        page: { next_cursor: null },
      }),
    "GET /deal-rooms/room-1/documents": () =>
      jsonResponse({
        data: filled
          ? [
              {
                id: DOC_ID,
                room_id: ROOM.id,
                attachment_id: "att-1",
                group_key: "legal",
                title: "MSA redline",
                filename: "msa-redline.docx",
                byte_size: 212_000,
                position: 0,
                source: "manual",
                version: 1,
                created_at: at,
                updated_at: at,
              },
            ]
          : [],
        page: { next_cursor: null },
      }),
    "GET /deal-rooms/room-1/threads": () =>
      jsonResponse({
        data: filled
          ? [
              {
                id: THREAD_ID,
                room_id: ROOM.id,
                document_id: DOC_ID,
                state: "open",
                required_change: false,
                author: { name: "Dana Buyer", side: "buyer" },
                created_at: at,
                comments: [
                  {
                    id: "c-1",
                    thread_id: THREAD_ID,
                    author: { name: "Dana Buyer", side: "buyer" },
                    body: "Clause 11.2 caps liability at fees paid. We need twelve months.",
                    created_at: at,
                  },
                ],
              },
            ]
          : [],
        page: { next_cursor: null },
      }),
    "GET /deal-rooms/room-1/brief": () =>
      jsonResponse({
        room_id: ROOM.id,
        generated_at: at,
        generated_by: "model",
        sentences: filled
          ? [
              {
                text: "Dana Buyer is waiting on your answer about the liability cap in the MSA redline.",
                nature: "fact",
                evidence: [
                  { entity_type: "thread", entity_id: THREAD_ID },
                  { entity_type: "document", entity_id: DOC_ID },
                ],
              },
              {
                text: "Answer it before the redline goes to legal.",
                nature: "recommendation",
                evidence: [{ entity_type: "thread", entity_id: THREAD_ID }],
              },
            ]
          : [],
      }),
  });
}

function wholeTab(state: DealRoom["state"], filled: boolean) {
  fullStub(state, filled);
  return (
    <StoryProviders>
      <DealRoomTab dealId="deal-1" dealName="Fleet telematics" />
    </StoryProviders>
  );
}

/** Just opened: the band, the four steps still to do, and no brief. */
export const Empty: Story = { render: () => wholeTab("live", false) };

/** In use: a buyer waiting on an answer, said first by the brief. */
export const Active: Story = { render: () => wholeTab("live", true) };

/** Paused: the notice says what the buyer sees, and Resume is beside it. */
export const Paused: Story = { render: () => wholeTab("paused", true) };

/** Closed: read-only, with the way to the room's settings in the notice. */
export const Closed: Story = { render: () => wholeTab("closed", true) };

export const ActiveDark: Story = {
  globals: { theme: "dark" },
  render: () => wholeTab("live", true),
};

/** Manage room: who may enter, until when, the contact, and the text. */
export const ManageRoom: Story = {
  render: () => wholeTab("live", true),
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", { name: "Manage room" }),
    );
    await userEvent.click(await screen.findByRole("radio", { name: /Closed/ }));
  },
};

/** The buyer-facing text, editable. The head is a title; the rest is body. */
export const Text: Story = {
  render: () => {
    stub();
    return (
      <StoryProviders>
        <RoomText room={room()} refusal={undefined} />
      </StoryProviders>
    );
  },
};

/**
 * A finished room. The panel keeps its fields and loses the Save verb — one
 * sentence stands where the verb was, which is what a refusal owes a reader.
 */
export const TextRefused: Story = {
  render: () => {
    stub();
    return (
      <StoryProviders>
        <RoomText
          room={room({ state: "closed" })}
          refusal="This room has ended. Its shared content is kept as a record."
        />
      </StoryProviders>
    );
  },
};

/** An archived room: the preview is refused with the reason beside it. */
export const PreviewRefused: Story = {
  render: () => {
    stub();
    return (
      <StoryProviders>
        <ViewAsBuyerButton room={room({ state: "archived" })} />
      </StoryProviders>
    );
  },
};

export const TextDark: Story = {
  globals: { theme: "dark" },
  render: () => {
    stub();
    return (
      <StoryProviders>
        <RoomText room={room()} refusal={undefined} />
      </StoryProviders>
    );
  },
};

/** A deal with no room: the verb opens the form that names the room. */
export const OpeningARoom: Story = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute({ deal_room: ["read", "create", "update"] }),
      "GET /deal-rooms": () =>
        jsonResponse({ data: [], page: { next_cursor: null } }),
    });
    return (
      <StoryProviders>
        <DealRoomTab dealId="deal-1" dealName="Fleet telematics" />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", {
        name: "Open a Deal Room",
      }),
    );
    await screen.findByRole("dialog");
  },
};

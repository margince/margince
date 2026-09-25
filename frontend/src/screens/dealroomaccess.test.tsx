// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { DealRoomAccess } from "./dealroomaccess";
import { installFetchStub, jsonResponse } from "./story-utils";

type DealRoom = components["schemas"]["DealRoom"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const ROOM: DealRoom = {
  id: "room-1",
  deal_id: "deal-1",
  title: "Acme Expansion — Deal Room",
  state: "live",
  source: "manual",
  captured_by: "u-me",
  version: 1,
  created_at: "2026-08-22T09:00:00Z",
  updated_at: "2026-08-22T09:00:00Z",
};

function drawWithDownloads(downloads: number) {
  installFetchStub({
    "GET /deal-rooms/room-1/participants": () =>
      jsonResponse({
        data: [
          {
            id: "part-1",
            room_id: ROOM.id,
            full_name: "Dana Buyer",
            email: "dana.buyer@brandt-automotive.example",
            capability: "read",
            delivery_state: "delivered",
            has_signed_in: true,
            source: "manual",
            captured_by: "u-me",
            created_at: "2026-08-22T09:10:00Z",
            updated_at: "2026-08-24T14:02:00Z",
            download_count: downloads,
          },
        ],
        page: { next_cursor: null },
      }),
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <DealRoomAccess room={ROOM} mayManage={false} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("what a seat has taken out of the room", () => {
  it.each([
    [1, "1 document downloaded"],
    [3, "3 documents downloaded"],
  ])("counts %i download(s) in the reader's grammar", async (count, said) => {
    drawWithDownloads(count);
    expect(await screen.findByText(said)).toBeTruthy();
  });
});

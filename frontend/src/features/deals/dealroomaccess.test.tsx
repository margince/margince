// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { type WriteTo, WriteToProvider } from "../../lib/writeto";
import { installFetchStub, jsonResponse } from "../../testing/story-utils";
import { DealRoomAccess } from "./dealroomaccess";

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

type Participant = components["schemas"]["DealRoomParticipant"];

function drawWithDownloads(
  downloads: number,
  seat: Partial<Participant> = {},
  writeTo: WriteTo | null = null,
) {
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
            ...seat,
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
        <WriteToProvider writeTo={writeTo}>
          <DealRoomAccess room={ROOM} mayManage={false} />
        </WriteToProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("what a seat has taken out of the room", () => {
  it.each([
    [1, "1 document downloaded"],
    [3, "3 documents downloaded"],
  ])(
    "counts the downloads in the reader's grammar (%i)",
    async (count, said) => {
      drawWithDownloads(count);
      expect(await screen.findByText(new RegExp(said))).toBeTruthy();
    },
  );

  it("names the documents taken on the seat's own line", async () => {
    drawWithDownloads(2, { documents_downloaded: ["Offer.pdf", "MSA.pdf"] });
    expect(
      await screen.findByText(/2 documents downloaded: Offer\.pdf, MSA\.pdf/),
    ).toBeTruthy();
  });
});

describe("where a seat stands", () => {
  it.each<[string, Partial<Participant>, string]>([
    ["signed in", {}, "Signed in"],
    ["not in yet", { has_signed_in: false }, "Invited"],
    [
      "asking for a new link",
      { link_requested_at: "2026-08-25T08:00:00Z" },
      "Link requested",
    ],
    ["revoked", { revoked_at: "2026-08-26T08:00:00Z" }, "Revoked"],
  ])("badges a seat that is %s", async (_case, seat, badge) => {
    drawWithDownloads(0, seat);
    expect(await screen.findByText(badge)).toBeTruthy();
  });
});

describe("a seat's address", () => {
  it("opens the composer filed under the room's deal", async () => {
    const writeTo = vi.fn();
    drawWithDownloads(0, {}, writeTo);
    fireEvent.click(
      await screen.findByRole("button", {
        name: "dana.buyer@brandt-automotive.example",
      }),
    );
    expect(writeTo).toHaveBeenCalledWith({
      entityType: "deal",
      entityId: "deal-1",
      address: "dana.buyer@brandt-automotive.example",
    });
  });
});

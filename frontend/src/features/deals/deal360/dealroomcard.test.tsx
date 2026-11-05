/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider, translate } from "../../../i18n";
import { DealRoomCard } from "./dealroomcard";

// The card says only what the room and seat reads say. It names who was invited
// and who of them signed in, never how often. It says "No room yet" only once
// the room read has answered.

const DEAL = "01a02e25-a5ac-7099-8099-581cbf001a99";
const ROOM = "01a02e25-a5ac-7099-8099-581cbf00f001";

function json(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "content-type": "application/json" },
  });
}

const room = {
  id: ROOM,
  deal_id: DEAL,
  title: "Facade retrofit",
  state: "live",
  source: "user",
  created_at: "2026-08-01T09:00:00Z",
  updated_at: "2026-08-01T09:00:00Z",
};

function seat(id: string, lastSeen: string | null) {
  return {
    id,
    room_id: ROOM,
    full_name: id,
    email: `${id}@buyer.example`,
    capability: "view",
    delivery_state: "delivered",
    has_signed_in: lastSeen !== null,
    last_seen_at: lastSeen,
    source: "user",
    created_at: "2026-08-01T09:00:00Z",
    updated_at: "2026-08-01T09:00:00Z",
  };
}

function stubFetch(rooms: unknown[], seats: unknown[]) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const page = { next_cursor: null };
      if (request.url.includes("/participants")) {
        return json({ data: seats, page });
      }
      if (request.url.includes("/deal-rooms")) {
        return json({ data: rooms, page });
      }
      return json({ data: [], page });
    }),
  );
}

function draw() {
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <DealRoomCard dealId={DEAL} onOpen={() => undefined} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the deal's room card", () => {
  it("counts who signed in of those invited", async () => {
    stubFetch([room], [seat("a", "2026-08-02T10:00:00Z"), seat("b", null)]);
    draw();
    expect(
      await screen.findByText(
        translate("en", "deal.room.invited", { invited: "2" }),
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        new RegExp(translate("en", "deal.room.signedIn", { active: "1" })),
      ),
    ).toBeInTheDocument();
  });

  it("says nobody is invited when the room has no seats", async () => {
    stubFetch([room], []);
    draw();
    expect(
      await screen.findByText(translate("en", "deal.room.nobodyInvited")),
    ).toBeInTheDocument();
    expect(screen.getByText(room.title)).toBeInTheDocument();
  });

  it("offers to set a room up when the deal has none", async () => {
    stubFetch([], []);
    draw();
    expect(
      await screen.findByText(translate("en", "deal.room.none")),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: translate("en", "deal.room.setUp") }),
    ).toBeInTheDocument();
  });
});

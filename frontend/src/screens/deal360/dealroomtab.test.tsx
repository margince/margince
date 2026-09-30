// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { jsonResponse, stubWithSession } from "../story-utils";
import { DealRoomTab } from "./dealroomtab";

// The deal record's own door into the Deal Room: the tab draws the room's
// reading and editable text, and the rail draws who may enter it — the same
// sections the room's own page draws (dealroompage.test.tsx), without that
// page's identity band or verbs.

type DealRoom = components["schemas"]["DealRoom"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function withProviders(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{node}</LocaleProvider>
    </QueryClientProvider>,
  );
}

function room(over: Partial<DealRoom> = {}): DealRoom {
  return {
    id: "room-1",
    deal_id: "deal-1",
    title: "Acme Expansion — Deal Room",
    state: "live",
    source: "manual",
    version: 1,
    created_at: "2026-08-22T09:00:00Z",
    updated_at: "2026-08-22T09:00:00Z",
    ...over,
  } as DealRoom;
}

describe("DealRoomTab", () => {
  it("draws the room's editable text when the deal already has one", async () => {
    stubWithSession(
      {
        "GET /deal-rooms": () =>
          jsonResponse({ data: [room()], page: { next_cursor: null } }),
        "GET /deal-rooms/room-1/participants": () =>
          jsonResponse({ data: [], page: {} }),
        "GET /deal-rooms/room-1/threads": () =>
          jsonResponse({ data: [], page: { next_cursor: null } }),
      },
      { deal_room: ["read", "update"] },
    );
    withProviders(<DealRoomTab dealId="deal-1" dealName="Acme Expansion" />);

    expect(
      await screen.findByDisplayValue("Acme Expansion — Deal Room"),
    ).toBeInTheDocument();
  });

  it("offers to open a room when the deal has none", async () => {
    stubWithSession(
      { "GET /deal-rooms": () => jsonResponse({ data: [], page: {} }) },
      { deal_room: ["read", "create"] },
    );
    withProviders(<DealRoomTab dealId="deal-1" dealName="Acme Expansion" />);

    expect(
      await screen.findByRole("button", { name: "Open a Deal Room" }),
    ).toBeInTheDocument();
  });

  it("a reader who may not create is offered no way to open a room", async () => {
    stubWithSession(
      { "GET /deal-rooms": () => jsonResponse({ data: [], page: {} }) },
      { deal_room: ["read"] },
    );
    const { container } = withProviders(
      <DealRoomTab dealId="deal-1" dealName="Acme Expansion" />,
    );

    await waitFor(() => expect(container).toBeEmptyDOMElement());
  });
});

describe("the tab carries the room's own access list", () => {
  // On the tab, not in the record's details pane: the pane is shut when a
  // reader arrives, so a room they had just opened offered them no way to let
  // anybody into it.
  it("names who may enter once a room exists", async () => {
    stubWithSession(
      {
        "GET /deal-rooms": () =>
          jsonResponse({ data: [room()], page: { next_cursor: null } }),
        "GET /deal-rooms/room-1/participants": () =>
          jsonResponse({ data: [], page: {} }),
      },
      { deal_room: ["read", "update"] },
    );
    withProviders(<DealRoomTab dealId="deal-1" dealName="BaymeOps" />);

    expect(await screen.findByText("Access")).toBeInTheDocument();
  });
});

// Pause, resume, close and the end date are the room's own verbs and live on
// its own page; the tab is where a rep arrives, so the tab is the way there.
describe("the tab leads to the room's own page", () => {
  function roomRoutes(state: DealRoom["state"] = "live") {
    return {
      "GET /deal-rooms": () =>
        jsonResponse({ data: [room({ state })], page: { next_cursor: null } }),
      "GET /deal-rooms/room-1/participants": () =>
        jsonResponse({ data: [], page: {} }),
    };
  }

  it("takes a reader who may manage the room to its page", async () => {
    stubWithSession(roomRoutes(), { deal_room: ["read", "update"] });
    const user = userEvent.setup();
    withProviders(<DealRoomTab dealId="deal-1" dealName="Acme Expansion" />);

    await user.click(
      await screen.findByRole("button", { name: "Manage room" }),
    );

    await waitFor(() =>
      expect(window.location.hash).toBe("#/deals/deal-1/room"),
    );
  });

  // Once closed, the tab stops offering revoke and new links; the page still
  // does, so the way there has to outlive the room's last live day.
  it("keeps the way there once the room is closed", async () => {
    stubWithSession(roomRoutes("closed"), { deal_room: ["read", "update"] });
    withProviders(<DealRoomTab dealId="deal-1" dealName="Acme Expansion" />);

    expect(
      await screen.findByRole("button", { name: "Manage room" }),
    ).toBeInTheDocument();
  });

  it("offers a reader who may not manage the room no way to its verbs", async () => {
    stubWithSession(roomRoutes(), { deal_room: ["read"] });
    withProviders(<DealRoomTab dealId="deal-1" dealName="Acme Expansion" />);

    expect(
      await screen.findByDisplayValue("Acme Expansion — Deal Room"),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Manage room" })).toBeNull();
  });
});

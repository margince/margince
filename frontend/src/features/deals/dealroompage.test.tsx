/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "@testing-library/jest-dom/vitest";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { DealRoomPage } from "./dealroompage";

// The head of the seller's Deal Room: what the room is, what it is called, how
// it stands, and who is in it. Everything here is a claim a rep acts on — a
// room that reads live is one they believe a buyer can walk into — so each is
// pinned rather than left to the eye.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

type DealRoom = components["schemas"]["DealRoom"];
type Participant = components["schemas"]["DealRoomParticipant"];

const ROOM = {
  id: "room-1",
  deal_id: "deal-1",
  title: "Brandt Automotive — retrofit programme",
  state: "live",
  source: "manual",
  version: 3,
  created_at: "2026-08-22T09:00:00Z",
  updated_at: "2026-08-24T14:02:00Z",
} as DealRoom;

// One buyer who has been through the door, and one whose seat was taken back.
// The revoked row is what separates "invited" from "rows in the list": counting
// the list would say two contacts may enter a room only one may.
const SIGNED_IN = {
  id: "part-1",
  room_id: ROOM.id,
  full_name: "Dana Buyer",
  email: "dana@brandt.example",
  capability: "read",
  delivery_state: "delivered",
  has_signed_in: true,
  last_seen_at: "2026-08-24T14:02:00Z",
  source: "manual",
  created_at: "2026-08-22T09:10:00Z",
  updated_at: "2026-08-24T14:02:00Z",
} as Participant;

const REVOKED = {
  ...SIGNED_IN,
  id: "part-2",
  full_name: "Gone Buyer",
  email: "gone@brandt.example",
  has_signed_in: false,
  last_seen_at: null,
  revoked_at: "2026-08-23T09:00:00Z",
} as Participant;

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

// `participants` undefined leaves that read in flight for ever, which is the
// state the attendance line refuses to speak in.
function stubApi(
  room: DealRoom,
  participants: readonly Participant[] | undefined,
) {
  vi.stubGlobal("fetch", (input: Request) => {
    const path = new URL(input.url).pathname;
    if (path.endsWith("/me")) {
      return Promise.resolve(
        jsonResponse(meFixture({ allow: { deal_room: ["read", "update"] } })),
      );
    }
    if (path.endsWith("/participants")) {
      return participants
        ? Promise.resolve(jsonResponse({ data: participants, page: {} }))
        : new Promise<Response>(() => {});
    }
    if (path.endsWith("/deal-rooms")) {
      return Promise.resolve(jsonResponse({ data: [room], page: {} }));
    }
    return Promise.resolve(jsonResponse({ data: [], page: {} }));
  });
}

const render = (ui: ReactNode) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
};

it("names the room, marks it live, and counts who is in it", async () => {
  stubApi(ROOM, [SIGNED_IN, REVOKED]);
  render(<DealRoomPage dealId="deal-1" />);

  expect(
    await screen.findByRole("heading", { name: ROOM.title }),
  ).toBeInTheDocument();
  // The standing reads beside the name, in the success tone with the dot that
  // says the state is true right now rather than remembered.
  const badge = screen.getByText("Live").closest(".badge");
  expect(badge).toHaveClass("badge-success");
  expect(badge?.querySelector(".badge-live-dot")).toBeInTheDocument();
  // One invited, not two: the revoked seat may not enter.
  expect(
    await screen.findByText("1 invited · 1 signed in"),
  ).toBeInTheDocument();
  expect(
    screen.getByText("Last seen by a buyer: 24 Aug 2026"),
  ).toBeInTheDocument();
  expect(
    screen.getByRole("button", { name: "← Back to deal" }),
  ).toBeInTheDocument();
});

// A "0 invited" drawn while the read is still out is a wrong statement rather
// than a loading one, and a rep who read it would conclude the invitation they
// sent never landed.
it("says nothing about attendance until the count is a fact", async () => {
  stubApi(ROOM, undefined);
  render(<DealRoomPage dealId="deal-1" />);

  expect(
    await screen.findByRole("heading", { name: ROOM.title }),
  ).toBeInTheDocument();
  expect(screen.queryByText(/invited/)).toBeNull();
});

// A room a rep has stopped is not marked as current: the tone changes and the
// dot goes, because "paused" is something the room recorded rather than
// something happening as the page is read.
it("marks a paused room as stopped rather than current", async () => {
  stubApi({ ...ROOM, state: "paused" }, []);
  render(<DealRoomPage dealId="deal-1" />);

  const badge = (await screen.findByText("Paused")).closest(".badge");
  expect(badge).toHaveClass("badge-warning");
  expect(badge?.querySelector(".badge-live-dot")).not.toBeInTheDocument();
});

it("ends access on the day picked from the calendar", async () => {
  const expiring = { ...ROOM, expires_at: "2026-09-30T23:59:59Z" };
  const sent: unknown[] = [];
  vi.stubGlobal("fetch", async (input: Request) => {
    const path = new URL(input.url).pathname;
    if (path.endsWith("/expiry")) {
      sent.push(await input.json());
      return jsonResponse(expiring);
    }
    if (path.endsWith("/me")) {
      return jsonResponse(
        meFixture({ allow: { deal_room: ["read", "update"] } }),
      );
    }
    if (path.endsWith("/deal-rooms")) {
      return jsonResponse({ data: [expiring], page: {} });
    }
    return jsonResponse({ data: [], page: {} });
  });
  const user = userEvent.setup();
  render(<DealRoomPage dealId="deal-1" />);

  await user.click(await screen.findByRole("button", { name: "Room access" }));
  await user.click(
    await screen.findByRole("button", { name: /^Set end date/ }),
  );
  const box = await screen.findByRole("textbox", { name: "Access ends on" });
  expect(box).toHaveValue("30/09/2026");
  await user.click(screen.getByRole("button", { name: /^Choose date/ }));
  await user.click(
    await screen.findByRole("button", { name: "Monday, 28 September 2026" }),
  );
  const dialog = screen.getByRole("dialog", { name: "Set end date" });
  await user.click(within(dialog).getByRole("button", { name: "Save" }));

  await waitFor(() =>
    expect(sent).toEqual([{ expires_at: "2026-09-28T23:59:59.000Z" }]),
  );
});

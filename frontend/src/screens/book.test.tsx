/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { formatDateTime } from "../format/format";
import { viewerZone } from "../format/timezone";
import { LocaleProvider } from "../i18n";
import { BookingScreen, PUBLIC_BOOKING_CONSENT } from "./book";

// Every free slot the server offers is a button, and pressing one books THAT
// slot — on both variants of the page. A slot drawn but not bookable, or booked
// with another slot's times, is a meeting at an hour nobody chose.

const SLOTS = [
  { start: "2026-07-20T09:00:00Z", end: "2026-07-20T09:30:00Z" },
  { start: "2026-07-21T13:30:00Z", end: "2026-07-21T14:00:00Z" },
];

const CONSENT_WORDING =
  "I agree that my name and email are stored to arrange and follow up on this meeting.";

// The button's accessible name IS the formatted slot, in the zone the screen
// renders through, so the lookup cannot depend on the machine's own zone.
function slotName(slot: { start: string }): string {
  return formatDateTime(slot.start, "en", viewerZone()).replace(/\s+/g, " ");
}

type Call = { method: string; path: string; body?: unknown };

// Answers availability with SLOTS and records every call, so a spec can assert
// the path and body the booking was sent with.
function serveSlots(bookingReply: unknown) {
  const calls: Call[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const path = new URL(request.url).pathname;
      const body =
        request.method === "POST" ? await request.clone().json() : undefined;
      calls.push({ method: request.method, path, body });
      const reply = path.endsWith("/availability")
        ? { slots: SLOTS, truncated: false }
        : bookingReply;
      return new Response(JSON.stringify(reply), {
        status: request.method === "POST" ? 201 : 200,
        headers: { "content-type": "application/json" },
      });
    }),
  );
  return calls;
}

function mount(hostSlug?: string) {
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <BookingScreen hostSlug={hostSlug} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
  cleanup();
});

describe("the booking page", () => {
  it("offers each free slot to a signed-in booker and books the one picked", async () => {
    const user = userEvent.setup();
    const calls = serveSlots({ id: "b-1", occurred_at: SLOTS[1].start });
    mount();

    const picked = await screen.findByRole("button", {
      name: slotName(SLOTS[1]),
    });
    expect(
      screen.getByRole("button", { name: slotName(SLOTS[0]) }),
    ).toBeTruthy();

    await user.click(picked);

    await screen.findByText("Booked.");
    const bookings = calls.filter((call) => call.method === "POST");
    expect(bookings).toEqual([
      {
        method: "POST",
        path: "/v1/bookings",
        body: {
          start: SLOTS[1].start,
          end: SLOTS[1].end,
          subject: "Meeting via Margince",
          attendee_emails: [],
          links: [],
        },
      },
    ]);
  });

  it("offers each free slot to a visitor, bookable only once name, email and consent are given", async () => {
    const user = userEvent.setup();
    const calls = serveSlots(SLOTS[0]);
    mount("ada-lovelace");

    const first = await screen.findByRole("button", {
      name: slotName(SLOTS[0]),
    });
    const second = screen.getByRole("button", { name: slotName(SLOTS[1]) });
    expect(first).toHaveProperty("disabled", true);
    expect(second).toHaveProperty("disabled", true);

    await user.type(screen.getByLabelText("Your name"), "Nina Weber");
    await user.type(screen.getByLabelText("Your email"), "nina@brandt.example");
    await user.click(screen.getByRole("checkbox"));
    await waitFor(() => expect(first).toHaveProperty("disabled", false));

    await user.click(first);

    await screen.findByText("Booked.");
    const bookings = calls.filter((call) => call.method === "POST");
    expect(bookings).toEqual([
      {
        method: "POST",
        path: "/v1/public/booking/ada-lovelace",
        body: {
          start: SLOTS[0].start,
          end: SLOTS[0].end,
          subject: "Meeting via Margince",
          booker: { name: "Nina Weber", email: "nina@brandt.example" },
          consent: { ...PUBLIC_BOOKING_CONSENT, wording: CONSENT_WORDING },
        },
      },
    ]);
  });
});

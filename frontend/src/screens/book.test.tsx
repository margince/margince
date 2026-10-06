/** @vitest-environment happy-dom */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { formatTimeOfDay } from "../format/format";
import { viewerZone } from "../format/timezone";
import { LocaleProvider } from "../i18n";
import { BookingScreen, PUBLIC_BOOKING_CONSENT } from "./book";
import {
  bookingConnection,
  bookingInvitation,
  bookingProfile,
  bookingSlots,
} from "./book.testkit";

type Call = { method: string; path: string; body: unknown; key: string | null };
function serve(fail = false) {
  const calls: Call[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const path = new URL(request.url).pathname;
      const body =
        request.method === "POST" ? await request.clone().json() : null;
      calls.push({
        method: request.method,
        path,
        body,
        key: request.headers.get("Idempotency-Key"),
      });
      const post = request.method === "POST";
      const reply = post
        ? fail
          ? {
              status: 409,
              title: "Slot no longer available",
              detail: "Choose another time.",
            }
          : {
              invitation: {
                ...bookingInvitation,
                management_token: "private-link",
              },
            }
        : path.endsWith("/connectors")
          ? { data: [bookingConnection] }
          : path.endsWith("/calendars")
            ? []
            : path.endsWith("/availability")
              ? { slots: bookingSlots, truncated: false }
              : path.includes("/meeting/")
                ? bookingInvitation
                : bookingProfile;
      return new Response(JSON.stringify(reply), {
        status: post ? (fail ? 409 : 201) : 200,
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
// The guest page lists the chosen day's times by their time alone; the day is
// the calendar's, and the first day with a free time is chosen for the guest.
const slotName = (index: number, zone = viewerZone()) =>
  formatTimeOfDay(bookingSlots[index].start, "en", zone);
const CONFIRM = /^Confirm \d/;
// The month on show is the one the guest opens the page in, so the clock is
// pinned to the month the fixture's free times fall in: midday UTC on the 1st
// is October in every zone.
function inBookingMonth() {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-10-01T12:00:00Z"));
}
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
  window.location.hash = "";
});

it("offers one reusable copy action without requiring a contact", async () => {
  const calls = serve();
  mount();
  expect(
    await screen.findByDisplayValue(bookingProfile.public_url ?? ""),
  ).toBeTruthy();
  expect(screen.getByRole("button", { name: "Copy link" })).toBeTruthy();
  expect(
    screen.queryByRole("button", { name: "Copy signature link" }),
  ).toBeNull();
  expect(calls.some((call) => call.path.includes("/contacts"))).toBe(false);
});
it("submits a real invitation only after the visitor confirms their details and consent", async () => {
  inBookingMonth();
  const user = userEvent.setup();
  const calls = serve();
  mount("ada-lovelace");
  await user.click(await screen.findByRole("button", { name: slotName(0) }));
  expect(calls.filter((call) => call.method === "POST")).toHaveLength(0);
  expect(screen.getByRole("button", { name: CONFIRM })).toHaveProperty(
    "disabled",
    true,
  );
  await user.type(screen.getByLabelText("Your name"), "Nina Weber");
  await user.type(screen.getByLabelText("Your email"), "nina@brandt.example");
  await user.click(screen.getByRole("checkbox"));
  await user.click(screen.getByRole("button", { name: CONFIRM }));
  await waitFor(() =>
    expect(window.location.hash).toContain("manage-private-link"),
  );
  const posted = calls.find((call) => call.method === "POST");
  expect(posted?.body).toMatchObject({
    ...bookingSlots[0],
    delivery: "calendar",
    booker: { name: "Nina Weber", email: "nina@brandt.example" },
    consent: PUBLIC_BOOKING_CONSENT,
  });
  expect(posted?.key).toBeTruthy();
});
it("retains the visitor's details and retry identity when a slot is refused", async () => {
  inBookingMonth();
  const user = userEvent.setup();
  const calls = serve(true);
  mount("ada-lovelace");
  await user.click(await screen.findByRole("button", { name: slotName(1) }));
  await user.type(screen.getByLabelText("Your name"), "Nina Weber");
  await user.type(screen.getByLabelText("Your email"), "nina@brandt.example");
  await user.click(screen.getByRole("checkbox"));
  await user.click(screen.getByRole("button", { name: CONFIRM }));
  await screen.findByText("Choose another time.");
  expect(screen.getByLabelText("Your email")).toHaveProperty(
    "value",
    "nina@brandt.example",
  );
  await user.click(screen.getByRole("button", { name: CONFIRM }));
  await waitFor(() =>
    expect(calls.filter((call) => call.method === "POST")).toHaveLength(2),
  );
  const posts = calls.filter((call) => call.method === "POST");
  expect(posts[0].key).toBe(posts[1].key);
  expect(window.location.hash).not.toContain("manage-");
});
it("keeps a pending provider operation distinct from a confirmed invitation", async () => {
  serve();
  mount("manage-private-link");
  // The status is a badge over the meeting's own name, not the page's title.
  expect(
    await screen.findByRole("heading", { level: 1, name: "Project discovery" }),
  ).toBeTruthy();
  expect(screen.getByRole("status").textContent).toBe(
    "Creating your invitation…",
  );
  expect(screen.queryByText("Calendar invitation created")).toBeNull();
  expect(
    screen
      .getByRole("button", { name: "Cancel meeting" })
      .hasAttribute("disabled"),
  ).toBe(false);
});

it("recovers the existing meeting from a used personal proposal", async () => {
  const user = userEvent.setup();
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            profile: { ...bookingProfile, enabled: false },
            used: true,
            options: [],
            description: "Project discovery",
            expires_at: "2026-10-06T09:00:00Z",
            meeting: {
              ...bookingInvitation,
              management_token: "recovered-link",
            },
          }),
          { headers: { "Content-Type": "application/json" } },
        ),
    ),
  );
  mount("proposal-personal-link");
  await user.click(
    await screen.findByRole("button", { name: "View your meeting" }),
  );
  expect(window.location.hash).toContain("manage-recovered-link");
});

it("shows guest slots in the city selected from the timezone dropdown", async () => {
  inBookingMonth();
  const user = userEvent.setup();
  serve();
  mount("ada-lovelace");
  await user.click(await screen.findByRole("combobox", { name: "Time zone" }));
  await user.keyboard("Bangkok{Enter}");
  expect(
    await screen.findByRole("button", {
      name: slotName(0, "Asia/Bangkok"),
    }),
  ).toBeTruthy();
});

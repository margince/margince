/** @vitest-environment happy-dom */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { formatDateTime } from "../format/format";
import { viewerZone } from "../format/timezone";
import { LocaleProvider } from "../i18n";
import {
  bookingConnection,
  bookingContact,
  bookingHours,
  bookingInvitation,
  bookingProfile,
  bookingSlots,
} from "./book.testkit";
import { BookingInviteScreen } from "./booking-invite";

vi.mock("./compose", () => ({
  ComposeModal: ({
    initialMessage,
  }: Readonly<{ initialMessage?: { subject: string; body: string } }>) => (
    <div role="dialog" aria-label="Compose">
      {initialMessage?.body}
    </div>
  ),
}));

function mount(
  configured = true,
  horizon = 30,
  availability: (
    window: URLSearchParams,
  ) => typeof bookingSlots | components["schemas"]["Problem"] = () =>
    bookingSlots,
  hoursAnswer: unknown = bookingHours,
) {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-09-27T06:00:00Z"));
  const proposals: unknown[] = [];
  const invitations: unknown[] = [];
  const paths: string[] = [];
  const windows: URLSearchParams[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const path = new URL(request.url).pathname;
      paths.push(path);
      let body: unknown;
      if (request.method === "POST" && path.endsWith("/invitations")) {
        invitations.push(await request.json());
        body = { ...bookingInvitation, id: "meeting-1" };
      } else if (request.method === "POST") {
        proposals.push(await request.json());
        body = {
          id: "proposal-1",
          url: "https://crm.example.test/#/book/proposal-private",
          expires_at: "2026-10-06T09:00:00Z",
        };
      } else if (path.endsWith("/connectors"))
        body = {
          data: [
            {
              ...bookingConnection,
              scopes: configured
                ? bookingConnection.scopes
                : ["https://www.googleapis.com/auth/calendar.readonly"],
            },
          ],
        };
      else if (path.includes("/contacts/"))
        body = {
          ...bookingContact,
          primary_email: null,
          emails: [{ email: "nina@brandt.example", is_primary: false }],
        };
      else if (path.endsWith("/availability")) {
        windows.push(new URL(request.url).searchParams);
        const answer = availability(new URL(request.url).searchParams);
        if (!Array.isArray(answer))
          return new Response(JSON.stringify(answer), {
            status: 422,
            headers: { "content-type": "application/problem+json" },
          });
        body = { slots: answer, truncated: false };
      } else if (path.endsWith("/me/working-hours")) body = hoursAnswer;
      else
        body = {
          ...bookingProfile,
          horizon_days: horizon,
          provider: configured ? "gcal" : "",
        };
      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { "content-type": "application/json" },
      });
    }),
  );
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <BookingInviteScreen contactId={bookingContact.id} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { proposals, invitations, paths, windows };
}

afterEach(() => {
  cleanup();
  window.location.hash = "";
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

const slotName = (start: string) =>
  formatDateTime(start, "en", viewerZone()).replace(/\s+/g, " ");

it("prefills the contact and saved details, then offers two picked times in one step", async () => {
  const user = userEvent.setup();
  const { proposals } = mount();
  expect(await screen.findByDisplayValue("nina@brandt.example")).toBeTruthy();
  expect(
    screen.getByRole("heading", { name: "Book a meeting with Nina Weber" }),
  ).toBeTruthy();
  expect(
    screen.getByRole("button", { name: "Pick at least 2 times" }),
  ).toHaveProperty("disabled", true);
  for (const slot of bookingSlots)
    await user.click(
      await screen.findByRole("button", { name: slotName(slot.start) }),
    );
  expect(screen.getByText("2 of 3")).toBeTruthy();
  await user.click(
    screen.getByRole("button", { name: "Review email · 2 times" }),
  );
  await waitFor(() => expect(proposals).toHaveLength(1));
  expect(proposals[0]).toMatchObject({
    contact_id: bookingContact.id,
    attendee_email: "nina@brandt.example",
    subject: bookingProfile.title,
    location: "",
    video_call: true,
    options: bookingSlots,
  });
  const email = await screen.findByRole("dialog", { name: "Compose" });
  expect(email.textContent).toContain(
    "https://crm.example.test/#/book/proposal-private",
  );
  expect(screen.getByText(/Link created/)).toBeTruthy();

  // The link carries what was proposed, so a change after it means another one.
  await user.type(screen.getByLabelText("Meeting title"), "!");
  expect(screen.queryByText(/Link created/)).toBeNull();
  expect(screen.getByText(/so it gets a new link/)).toBeTruthy();
});

it("removes an offered time from the review list", async () => {
  const user = userEvent.setup();
  mount();
  await user.click(
    await screen.findByRole("button", {
      name: slotName(bookingSlots[0].start),
    }),
  );
  expect(screen.getByText("1 of 3")).toBeTruthy();
  await user.click(
    screen.getByRole("button", {
      name: `Remove ${formatDateTime(bookingSlots[0].start, "en", viewerZone())}`,
    }),
  );
  expect(screen.getByText("0 of 3")).toBeTruthy();
  expect(screen.getByText("Pick 2 or 3 times in the calendar.")).toBeTruthy();
});

it("sends an agreed time as an invitation without a video link when switched off", async () => {
  const user = userEvent.setup();
  const { invitations } = mount();
  await user.click(
    await screen.findByRole("radio", { name: /Send an invite/ }),
  );
  await user.click(
    screen.getByRole("switch", { name: "Add Google Meet link" }),
  );
  await user.click(screen.getByText(/^Details/));
  const location = screen.getByLabelText("Location or meeting link");
  await user.clear(location);
  await user.type(location, "Office, Room 2");
  await user.click(
    await screen.findByRole("button", {
      name: slotName(bookingSlots[1].start),
    }),
  );
  await user.click(
    screen.getByRole("button", {
      name: `Send invite · ${formatDateTime(bookingSlots[1].start, "en", viewerZone())}`,
    }),
  );
  await waitFor(() => expect(invitations).toHaveLength(1));
  expect(invitations[0]).toMatchObject({
    ...bookingSlots[1],
    location: "Office, Room 2",
    video_call: false,
  });
  await waitFor(() =>
    expect(window.location.hash).toBe("#/book/meeting-meeting-1"),
  );
});

it("shows the calendar setup step instead of times when no calendar can send invites", async () => {
  const { paths } = mount(false);
  expect(
    await screen.findByRole("link", { name: "Open meeting settings" }),
  ).toBeTruthy();
  expect(
    screen.getByText(/Set up calendars and availability in Settings/),
  ).toBeTruthy();
  expect(
    screen.getByRole("button", { name: "Connect a calendar first" }),
  ).toHaveProperty("disabled", true);
  expect(paths.some((path) => path.endsWith("/availability"))).toBe(false);
});

it("lets the guest pick for a personal link, showing the open times without asking for any", async () => {
  const user = userEvent.setup();
  const { proposals } = mount();
  const slot = slotName(bookingSlots[0].start);
  await user.click(await screen.findByRole("button", { name: slot }));
  await user.click(
    screen.getByRole("radio", { name: /Share a personal link/ }),
  );
  // The same week stays on screen as the times the guest will choose from,
  // and none of them is a time the host can pick.
  expect(screen.getByRole("heading", { name: "Your open times" })).toBeTruthy();
  expect(screen.getByText(slot)).toBeTruthy();
  expect(screen.queryByRole("button", { name: slot })).toBeNull();
  expect(screen.getByText("Nina Weber picks the time")).toBeTruthy();
  // The link books a meeting of the length chosen here.
  await user.click(screen.getByRole("button", { name: "45 min" }));
  await user.click(
    screen.getByRole("button", { name: "Create link and review email" }),
  );
  await waitFor(() => expect(proposals).toHaveLength(1));
  expect(proposals[0]).toMatchObject({ options: [], duration_minutes: 45 });
});

it("stops at the booking horizon when paging forward by week", async () => {
  const user = userEvent.setup();
  mount(true, 10);
  const next = await screen.findByRole("button", { name: "Next week" });
  expect(next).toHaveProperty("disabled", false);
  await user.click(next);
  expect(
    await screen.findByRole("button", { name: "Next week" }),
  ).toHaveProperty("disabled", true);
  expect(screen.getByRole("button", { name: "Previous week" })).toHaveProperty(
    "disabled",
    false,
  );
});
it("searches beyond a busy month in bounded requests and finds November within a longer horizon", async () => {
  const user = userEvent.setup();
  let calls = 0;
  const novemberSlots = bookingSlots.map((slot) => ({
    start: slot.start.replace("2026-10-05", "2026-11-02"),
    end: slot.end.replace("2026-10-05", "2026-11-02"),
  }));
  const { windows } = mount(true, 90, () => (++calls < 3 ? [] : novemberSlots));
  await user.click(
    await screen.findByRole("button", { name: "Find next available times" }),
  );
  await waitFor(() => expect(windows).toHaveLength(3));
  expect(
    await screen.findByRole("button", {
      name: slotName(novemberSlots[0].start),
    }),
  ).toBeTruthy();
  for (const window of windows) {
    const length =
      Date.parse(window.get("to") ?? "") - Date.parse(window.get("from") ?? "");
    expect(length).toBeGreaterThan(0);
    expect(length).toBeLessThanOrEqual(31 * 86400000);
  }
  expect(Date.parse(windows[2].get("from") ?? "")).toBeGreaterThan(
    Date.parse("2026-10-27"),
  );
});

it("finds a meeting crossing a search chunk boundary without losing it", async () => {
  const user = userEvent.setup();
  const start = "2026-10-28T05:45:00Z";
  const end = "2026-10-28T06:15:00Z";
  const { windows } = mount(true, 90, (window) =>
    Date.parse(window.get("from") ?? "") <= Date.parse(start) &&
    Date.parse(window.get("to") ?? "") >= Date.parse(end)
      ? [{ start, end }]
      : [],
  );
  await user.click(
    await screen.findByRole("button", { name: "Find next available times" }),
  );
  expect(
    await screen.findByRole("button", { name: slotName(start) }),
  ).toBeTruthy();
  expect(windows).toHaveLength(3);
  expect(Date.parse(windows[2].get("from") ?? "")).toBe(
    Date.parse(windows[1].get("to") ?? "") - 30 * 60000,
  );
});
it("ends a forward search cleanly when the server's horizon ends before the browser's", async () => {
  const user = userEvent.setup();
  let count = 0;
  mount(true, 90, () =>
    ++count < 3
      ? []
      : {
          type: "about:blank",
          status: 422,
          code: "validation_error",
          details: {
            errors: [
              {
                field: "from",
                code: "booking_horizon",
                message: "Past horizon",
              },
            ],
          },
        },
  );
  await user.click(
    await screen.findByRole("button", { name: "Find next available times" }),
  );
  expect(
    await screen.findByText(/No available times within the booking horizon/),
  ).toBeTruthy();
  expect(screen.queryByRole("alert")).toBeNull();
});

it("keeps the contact draft usable if the hours response is missing its required window", async () => {
  const user = userEvent.setup();
  mount(true, 30, () => bookingSlots, { chosen: false });
  const message = await screen.findByLabelText("Message for your guest");
  await user.type(message, "Keep this draft");
  expect(screen.getByDisplayValue("Keep this draft")).toBeTruthy();
  expect(await screen.findByDisplayValue("nina@brandt.example")).toBeTruthy();
});

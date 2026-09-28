/** @vitest-environment happy-dom */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { formatDateTime } from "../format/format";
import { dayInZone, viewerZone } from "../format/timezone";
import { LocaleProvider } from "../i18n";
import {
  bookingConnection,
  bookingContact,
  bookingHours,
  bookingProfile,
  bookingSlots,
} from "./book.testkit";
import { BookingInviteScreen } from "./booking-invite";

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
  const paths: string[] = [];
  const windows: URLSearchParams[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const path = new URL(request.url).pathname;
      paths.push(path);
      let body: unknown;
      if (request.method === "POST") {
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
  return { proposals, paths, windows };
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

it("prefills a singleton contact email and saved meeting details, then binds the selected proposal", async () => {
  const user = userEvent.setup();
  const { proposals } = mount();
  expect(await screen.findByDisplayValue("nina@brandt.example")).toBeTruthy();
  expect(await screen.findByDisplayValue(bookingProfile.title)).toBeTruthy();
  expect(screen.getByDisplayValue(bookingProfile.location)).toBeTruthy();
  expect(screen.getByText(/0 selected/)).toBeTruthy();
  for (const slot of bookingSlots)
    await user.click(
      await screen.findByRole("button", {
        name: formatDateTime(slot.start, "en", viewerZone()).replace(
          /\s+/g,
          " ",
        ),
      }),
    );
  expect(screen.getByText(/2 selected/)).toBeTruthy();
  await user.click(screen.getByRole("button", { name: "Create proposal" }));
  await waitFor(() => expect(proposals).toHaveLength(1));
  expect(proposals[0]).toMatchObject({
    contact_id: bookingContact.id,
    attendee_email: "nina@brandt.example",
    subject: bookingProfile.title,
    location: bookingProfile.location,
    options: bookingSlots,
  });
  expect(
    await screen.findByRole("link", { name: "Open personal invitation" }),
  ).toBeTruthy();
  await user.type(screen.getByLabelText("Meeting title"), " updated");
  expect(
    screen.getByRole("link", { name: "Open personal invitation" }),
  ).toBeTruthy();
  expect(
    screen.getByRole("button", { name: "Create updated proposal" }),
  ).toBeTruthy();
  expect(proposals).toHaveLength(1);
});

it("explains calendar setup without requesting unavailable times", async () => {
  const { paths } = mount(false);
  expect(
    await screen.findByRole("link", {
      name: "Open meeting settings",
    }),
  ).toBeTruthy();
  expect(
    screen.getByText(/Set up calendars and availability in Settings/),
  ).toBeTruthy();
  expect(paths.some((path) => path.endsWith("/availability"))).toBe(false);
});

it("explains a November date outside the saved horizon without claiming the calendar is busy", async () => {
  const { windows } = mount();
  await screen.findByText(/Bookings available through/);
  const date = screen.getByLabelText("Starting date");
  expect(date.getAttribute("max")).toBe(
    dayInZone(Date.parse("2026-10-27T06:00:00Z"), viewerZone()),
  );
  await waitFor(() => expect(windows).toHaveLength(1));
  fireEvent.change(date, { target: { value: "2026-11-02" } });
  expect(await screen.findByText(/outside the booking horizon/)).toBeTruthy();
  expect(screen.queryByText(/No available times/)).toBeNull();
  expect(windows).toHaveLength(1);
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
      name: formatDateTime(novemberSlots[0].start, "en", viewerZone()).replace(
        /\s+/g,
        " ",
      ),
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
    await screen.findByRole("button", {
      name: formatDateTime(start, "en", viewerZone()).replace(/\s+/g, " "),
    }),
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

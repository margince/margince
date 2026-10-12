/** @vitest-environment happy-dom */
import { QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { createQueryClient } from "../app/queryclient";
import { formatTimeOfDay, fullDayName } from "../format/format";
import { formatDayFull, formatTimeRange } from "../format/meetingtime";
import { dayInZone, viewerZone } from "../format/timezone";
import { BookingScreen } from "./book";
import {
  bookingConnection,
  bookingHours,
  bookingProfile,
  bookingSlots,
} from "./book.testkit";
import { MeetingSettings } from "./meeting-settings";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
// The page opens on the month the guest is in, so every guest-page test pins
// the clock to the month the fixture's free times fall in.
function inBookingMonth() {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-10-01T12:00:00Z"));
}
const slotName = (index: number) =>
  formatTimeOfDay(bookingSlots[index].start, "en", viewerZone());
const CONFIRM = /^Confirm \d/;

it.each([false, true])(
  "previews the saved page with enabled=%s without public reads or writes",
  async (enabled) => {
    inBookingMonth();
    const user = userEvent.setup();
    const publicRead = vi.fn(() => jsonResponse(bookingProfile));
    const availability = vi.fn(() =>
      jsonResponse({ slots: bookingSlots, truncated: false }),
    );
    installFetchStub({
      "GET /scheduling/profile": () =>
        jsonResponse({ ...bookingProfile, enabled }),
      "GET /public/booking/ada-lovelace/profile": publicRead,
      "GET /public/booking/ada-lovelace/availability": publicRead,
      "GET /availability": availability,
    });
    const requests = vi.spyOn(globalThis, "fetch");
    render(
      <StoryProviders>
        <BookingScreen hostSlug="preview" />
      </StoryProviders>,
    );
    expect(
      await screen.findByRole("heading", { name: bookingProfile.title }),
    ).toBeTruthy();
    expect(
      screen.getByText(
        enabled ? /booking page is live/ : /Public booking is paused/,
      ),
    ).toBeTruthy();
    // The host's calendar decides the video app a guest is told about.
    expect(screen.getByText("Google Meet · link in your invite")).toBeTruthy();
    await user.click(await screen.findByRole("button", { name: slotName(0) }));
    // The chosen time moves to the summary, and the times give way to the form.
    expect(
      screen.getByText(
        `${formatDayFull(bookingSlots[0].start, "en", viewerZone())} · ${formatTimeRange(bookingSlots[0].start, bookingSlots[0].end, "en", viewerZone())}`,
      ),
    ).toBeTruthy();
    expect(screen.queryByRole("button", { name: slotName(1) })).toBeNull();
    await user.type(screen.getByLabelText("Your name"), "Demo guest");
    await user.type(screen.getByLabelText("Your email"), "guest@example.test");
    await user.click(screen.getByRole("checkbox"));
    const button = screen.getByRole("button", { name: CONFIRM });
    expect(button).toHaveProperty("disabled", true);
    await user.click(button);
    expect(publicRead).not.toHaveBeenCalled();
    const form = button.closest("form");
    if (!form) throw new Error("Confirmation must belong to the guest form");
    await act(async () => {
      fireEvent.submit(form);
    });
    expect(availability).toHaveBeenCalled();
    const reads = requests.mock.calls.map(([input, init]) =>
      input instanceof Request ? input : new Request(input, init),
    );
    expect(reads.every((request) => request.method === "GET")).toBe(true);
    const query = reads.find((request) =>
      new URL(request.url).pathname.endsWith("/availability"),
    );
    expect(query).toBeDefined();
    expect(new URL(query?.url ?? "").searchParams.get("reliable")).toBe("true");
  },
);
it("keeps a paused public page unavailable to visitors", async () => {
  installFetchStub({
    "GET /public/booking/ada-lovelace/profile": () =>
      jsonResponse({ ...bookingProfile, enabled: false }),
  });
  render(
    <StoryProviders>
      <BookingScreen hostSlug="ada-lovelace" />
    </StoryProviders>,
  );
  expect(await screen.findByText(/unavailable/i)).toBeTruthy();
  expect(screen.queryByRole("button", { name: CONFIRM })).toBeNull();
});

// A day's own read asks for at most one day; a month's asks for far more.
function isDayRead(request: Request) {
  const query = new URL(request.url).searchParams;
  const span =
    Date.parse(query.get("to") ?? "") - Date.parse(query.get("from") ?? "");
  return span <= 86_400_000;
}

function availabilityReads(calls: readonly Parameters<typeof fetch>[]) {
  return calls
    .map(([input, init]) =>
      input instanceof Request ? input : new Request(input, init),
    )
    .filter((request) =>
      new URL(request.url).pathname.endsWith("/availability"),
    );
}

it("reads the whole month on show, following a truncated answer from its last time", async () => {
  inBookingMonth();
  installFetchStub({
    "GET /scheduling/profile": () =>
      jsonResponse({ ...bookingProfile, enabled: false }),
    "GET /availability": () =>
      jsonResponse({ slots: bookingSlots, truncated: true }),
  });
  const requests = vi.spyOn(globalThis, "fetch");
  render(
    <StoryProviders>
      <BookingScreen hostSlug="preview" />
    </StoryProviders>,
  );
  await screen.findByRole("button", { name: slotName(0) });
  const reads = availabilityReads(requests.mock.calls);
  const first = new URL(reads[0].url).searchParams;
  // From now to the start of November: one month, never an open-ended window.
  expect(first.get("from")).toBe("2026-10-01T12:00:00.000Z");
  expect(Date.parse(first.get("to") ?? "")).toBeLessThanOrEqual(
    Date.parse("2026-11-01T12:00:00Z"),
  );
  expect(new URL(reads[1].url).searchParams.get("from")).toBe(
    "2026-10-05T11:15:00.000Z",
  );
  // A bounded number of pages, however often the server says there is more.
  expect(reads.filter((read) => !isDayRead(read)).length).toBeLessThanOrEqual(
    4,
  );
});

it("reads the day a truncated month stopped in, and the days after it, on their own", async () => {
  inBookingMonth();
  const user = userEvent.setup();
  const rest = { start: "2026-10-05T15:00:00Z", end: "2026-10-05T15:30:00Z" };
  const late = { start: "2026-10-14T10:00:00Z", end: "2026-10-14T10:30:00Z" };
  // The month's read is always told there is more, so it stops at its page
  // bound partway through 5 October; each day's own read answers in full.
  const answer = (request: Request) => {
    if (!isDayRead(request))
      return jsonResponse({ slots: bookingSlots, truncated: true });
    const from = new URL(request.url).searchParams.get("from") ?? "";
    const day = dayInZone(Date.parse(from), viewerZone());
    return jsonResponse({
      slots: day === "2026-10-05" ? [...bookingSlots, rest] : [late],
      truncated: false,
    });
  };
  installFetchStub({
    "GET /scheduling/profile": () =>
      jsonResponse({ ...bookingProfile, enabled: false }),
    "GET /availability": () => {
      const [input, init] = requests.mock.lastCall ?? [""];
      return answer(
        input instanceof Request ? input : new Request(input, init),
      );
    },
  });
  const requests = vi.spyOn(globalThis, "fetch");
  render(
    <StoryProviders>
      <BookingScreen hostSlug="preview" />
    </StoryProviders>,
  );
  const timeOf = (slot: { start: string }) =>
    formatTimeOfDay(slot.start, "en", viewerZone());
  expect(
    await screen.findByRole("button", { name: timeOf(rest) }),
  ).toBeTruthy();
  const [year, month, date] = dayInZone(Date.parse(late.start), viewerZone())
    .split("-")
    .map(Number);
  await user.click(
    screen.getByRole("button", {
      name: fullDayName(new Date(year, month - 1, date), "en"),
    }),
  );
  expect(
    await screen.findByRole("button", { name: timeOf(late) }),
  ).toBeTruthy();
});

it("refuses days with nothing free and clears the time when another day is chosen", async () => {
  inBookingMonth();
  const user = userEvent.setup();
  installFetchStub({
    "GET /scheduling/profile": () =>
      jsonResponse({ ...bookingProfile, enabled: false }),
    "GET /availability": () =>
      jsonResponse({
        slots: [
          ...bookingSlots,
          { start: "2026-10-07T09:00:00Z", end: "2026-10-07T09:30:00Z" },
        ],
        truncated: false,
      }),
  });
  render(
    <StoryProviders>
      <BookingScreen hostSlug="preview" />
    </StoryProviders>,
  );
  // The first day with a free time is chosen for the guest.
  await user.click(await screen.findByRole("button", { name: slotName(0) }));
  expect(
    screen.getByRole("button", {
      name: "Tuesday, 6 October 2026, nothing free",
    }),
  ).toHaveProperty("disabled", true);
  expect(
    screen.getByRole("button", { name: "Monday, 5 October 2026" }),
  ).toHaveProperty("disabled", false);
  await user.click(
    screen.getByRole("button", { name: "Wednesday, 7 October 2026" }),
  );
  expect(screen.queryByRole("button", { name: CONFIRM })).toBeNull();
  expect(
    await screen.findByRole("heading", { name: "Wednesday 7 October" }),
  ).toBeTruthy();
  expect(
    screen
      .getByRole("button", { name: slotName(0) })
      .getAttribute("aria-pressed"),
  ).toBe("false");
});

it("says when a month has no free time rather than hiding availability", async () => {
  inBookingMonth();
  installFetchStub({
    "GET /scheduling/profile": () =>
      jsonResponse({ ...bookingProfile, enabled: false }),
    "GET /availability": () => jsonResponse({ slots: [], truncated: false }),
  });
  render(
    <StoryProviders>
      <BookingScreen hostSlug="preview" />
    </StoryProviders>,
  );
  expect(
    await screen.findByText("No free times this month. Try the next month."),
  ).toBeTruthy();
  expect(
    screen.getByRole("button", {
      name: "Monday, 5 October 2026, nothing free",
    }),
  ).toHaveProperty("disabled", true);
  expect(screen.queryByRole("button", { name: CONFIRM })).toBeNull();
});

it("shows calendar errors and retries the authenticated availability read", async () => {
  inBookingMonth();
  const user = userEvent.setup();
  const availability = vi
    .fn()
    .mockImplementationOnce(() =>
      jsonResponse(
        {
          title: "Unavailable",
          status: 503,
          detail: "Calendar temporarily unavailable",
        },
        503,
      ),
    )
    .mockImplementation(() =>
      jsonResponse({ slots: bookingSlots, truncated: false }),
    );
  installFetchStub({
    "GET /scheduling/profile": () =>
      jsonResponse({ ...bookingProfile, enabled: false }),
    "GET /availability": availability,
  });
  render(
    <StoryProviders>
      <BookingScreen hostSlug="preview" />
    </StoryProviders>,
  );
  expect(
    await screen.findByText("Calendar temporarily unavailable"),
  ).toBeTruthy();
  expect(screen.queryByRole("button", { name: slotName(0) })).toBeNull();
  await user.click(screen.getByRole("button", { name: "Retry" }));
  expect(await screen.findByRole("button", { name: slotName(0) })).toBeTruthy();
  expect(screen.getByRole("button", { name: slotName(1) })).toBeTruthy();
});

function Journey() {
  const [preview, setPreview] = useState(true);
  return (
    <>
      <button type="button" onClick={() => setPreview(!preview)}>
        Switch view
      </button>
      {preview ? <BookingScreen hostSlug="preview" /> : <MeetingSettings />}
    </>
  );
}

it("returns from settings to a preview with the newly saved values in a fresh cache", async () => {
  const user = userEvent.setup();
  let latest: unknown = { ...bookingProfile, enabled: false };
  installFetchStub({
    "GET /me": meRoute({ activity: ["create"] }),
    "GET /me/working-hours": () => jsonResponse(bookingHours),
    "GET /connectors": () => jsonResponse({ data: [bookingConnection] }),
    "GET /scheduling/calendars": () =>
      jsonResponse([
        { id: "primary", name: "Work", primary: true, writable: true },
      ]),
    "GET /scheduling/profile": () => jsonResponse(latest),
    "GET /availability": () =>
      jsonResponse({ slots: bookingSlots, truncated: false }),
    "PUT /scheduling/profile": (body) => {
      latest = body;
      return jsonResponse(body);
    },
  });
  const client = createQueryClient();
  client.setDefaultOptions({ queries: { staleTime: Infinity, retry: false } });
  render(
    <StoryProviders>
      <QueryClientProvider client={client}>
        <Journey />
      </QueryClientProvider>
    </StoryProviders>,
  );
  await screen.findByRole("heading", { name: bookingProfile.title });
  await user.click(screen.getByRole("button", { name: "Switch view" }));
  const title = await screen.findByLabelText("Meeting title");
  await user.clear(title);
  await user.type(title, "Newly saved title");
  await user.click(screen.getByRole("button", { name: "Save settings" }));
  await waitFor(() =>
    expect(latest).toMatchObject({ title: "Newly saved title" }),
  );
  await user.click(screen.getByRole("button", { name: "Switch view" }));
  expect(
    await screen.findByRole("heading", { name: "Newly saved title" }),
  ).toBeTruthy();
  expect(
    screen.queryByRole("heading", { name: bookingProfile.title }),
  ).toBeNull();
});

it("refreshes cached preview times after saving working hours", async () => {
  inBookingMonth();
  const user = userEvent.setup();
  let saved = false;
  const client = createQueryClient();
  client.setDefaultOptions({ queries: { staleTime: Infinity, retry: false } });
  installFetchStub({
    "GET /me": meRoute({ activity: ["create"] }),
    "GET /me/working-hours": () => jsonResponse(bookingHours),
    "PUT /me/working-hours": () => {
      saved = true;
      return jsonResponse({
        ...bookingHours,
        working_hours: { ...bookingHours.working_hours, start_time: "10:00" },
      });
    },
    "GET /connectors": () => jsonResponse({ data: [bookingConnection] }),
    "GET /scheduling/calendars": () =>
      jsonResponse([
        { id: "primary", name: "Work", primary: true, writable: true },
      ]),
    "GET /scheduling/profile": () =>
      jsonResponse({ ...bookingProfile, enabled: false }),
    "GET /availability": () =>
      jsonResponse({
        slots: saved ? bookingSlots.slice(1) : bookingSlots,
        truncated: false,
      }),
  });
  render(
    <StoryProviders>
      <QueryClientProvider client={client}>
        <Journey />
      </QueryClientProvider>
    </StoryProviders>,
  );
  expect(await screen.findByRole("button", { name: slotName(0) })).toBeTruthy();
  await user.click(screen.getByRole("button", { name: "Switch view" }));
  fireEvent.change(await screen.findByLabelText("Day starts"), {
    target: { value: "10:00" },
  });
  await user.click(screen.getByRole("button", { name: "Save settings" }));
  await waitFor(() =>
    expect(client.getQueryData(["working-hours"])).toMatchObject({
      working_hours: { start_time: "10:00" },
    }),
  );
  await user.click(screen.getByRole("button", { name: "Switch view" }));
  expect(await screen.findByRole("button", { name: slotName(1) })).toBeTruthy();
  expect(screen.queryByRole("button", { name: slotName(0) })).toBeNull();
});

it("explains calendar setup without requesting availability when no provider is selected", async () => {
  const availability = vi.fn(() =>
    jsonResponse({ slots: bookingSlots, truncated: false }),
  );
  installFetchStub({
    "GET /scheduling/profile": () =>
      jsonResponse({ ...bookingProfile, enabled: false, provider: "" }),
    "GET /availability": availability,
  });
  render(
    <StoryProviders>
      <BookingScreen hostSlug="preview" />
    </StoryProviders>,
  );
  expect(await screen.findByText(/Choose a calendar in Settings/)).toBeTruthy();
  expect(
    screen.getByRole("link", { name: "Open meeting settings" }),
  ).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
  expect(availability).not.toHaveBeenCalled();
});

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
it.each([false, true])(
  "previews the saved page with enabled=%s without public reads or writes",
  async (enabled) => {
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
    const [slot] = await screen.findAllByRole("button", { pressed: false });
    await user.click(slot);
    expect(slot.getAttribute("aria-pressed")).toBe("true");
    await user.type(screen.getByLabelText("Your name"), "Demo guest");
    await user.type(screen.getByLabelText("Your email"), "guest@example.test");
    await user.click(screen.getByRole("checkbox"));
    const button = screen.getByRole("button", { name: "Confirm meeting" });
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
  expect(screen.queryByRole("button", { name: "Confirm meeting" })).toBeNull();
});

it("browses later times and clears the selection when the date changes", async () => {
  const user = userEvent.setup();
  const availability = vi.fn(() =>
    jsonResponse({ slots: bookingSlots, truncated: true }),
  );
  installFetchStub({
    "GET /scheduling/profile": () =>
      jsonResponse({ ...bookingProfile, enabled: false }),
    "GET /availability": availability,
  });
  const requests = vi.spyOn(globalThis, "fetch");
  render(
    <StoryProviders>
      <BookingScreen hostSlug="preview" />
    </StoryProviders>,
  );
  const [slot] = await screen.findAllByRole("button", { pressed: false });
  await user.click(slot);
  await user.click(screen.getByRole("button", { name: "More times" }));
  await waitFor(() => expect(availability).toHaveBeenCalledTimes(2));
  await screen.findAllByRole("button", { pressed: false });
  expect(screen.queryByRole("button", { pressed: true })).toBeNull();
  const later = requests.mock.calls
    .map(([input, init]) =>
      input instanceof Request ? input : new Request(input, init),
    )
    .filter((request) =>
      new URL(request.url).pathname.endsWith("/availability"),
    );
  expect(new URL(later[1].url).searchParams.get("from")).toBe(
    "2026-10-05T11:15:00.000Z",
  );
  await user.click(
    (await screen.findAllByRole("button", { pressed: false }))[0],
  );
  fireEvent.change(screen.getByLabelText("Starting date"), {
    target: { value: "2026-10-12" },
  });
  await waitFor(() => expect(availability).toHaveBeenCalledTimes(3));
  await screen.findAllByRole("button", { pressed: false });
  expect(screen.queryByRole("button", { pressed: true })).toBeNull();
});

it("shows a genuine empty window rather than hiding availability", async () => {
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
    await screen.findByText(
      "No available times in this window. Try another date.",
    ),
  ).toBeTruthy();
  expect(
    screen.getByRole("button", { name: "Confirm meeting" }),
  ).toHaveProperty("disabled", true);
});

it("shows calendar errors and retries the authenticated availability read", async () => {
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
  expect(screen.queryByRole("button", { pressed: false })).toBeNull();
  await user.click(screen.getByRole("button", { name: "Retry" }));
  expect(await screen.findAllByRole("button", { pressed: false })).toHaveLength(
    2,
  );
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
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-09-27T06:00:00Z"));
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
  expect(await screen.findAllByRole("button", { pressed: false })).toHaveLength(
    2,
  );
  await user.click(screen.getByRole("button", { name: "Switch view" }));
  fireEvent.change(await screen.findByLabelText("Day starts"), {
    target: { value: "10:00" },
  });
  await user.click(screen.getByRole("button", { name: "Save working hours" }));
  await waitFor(() =>
    expect(client.getQueryData(["working-hours"])).toMatchObject({
      working_hours: { start_time: "10:00" },
    }),
  );
  await user.click(screen.getByRole("button", { name: "Switch view" }));
  await waitFor(() =>
    expect(screen.getAllByRole("button", { pressed: false })).toHaveLength(1),
  );
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

/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { SavePageProvider } from "../../design-system/savepage";
import { ToastProvider } from "../../design-system/toast";
import {
  bookingConnection,
  bookingHours,
  bookingProfile,
} from "../../testing/book.testkit";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { BookingScreen } from "./book";
import { MeetingSettings } from "./meeting-settings";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

const holidays = "en.german#holiday@group.v.calendar.google.com";
const calendars = [
  { id: "work", name: "Work calendar", primary: true, writable: true },
  {
    id: holidays,
    name: "Holidays in Germany",
    primary: false,
    writable: false,
  },
];
const profile = { ...bookingProfile, blocking_calendars: [holidays] };
const NAMED = /Your calendar “Holidays in Germany” could not be read/;

// What the server answers the host reading their own availability.
const unreadable = (
  details: Record<string, unknown> = { calendar: holidays },
) =>
  jsonResponse(
    {
      status: 502,
      code: "calendar_unavailable",
      detail: "The host's selected calendar could not be read.",
      details,
    },
    502,
  );

function routes(extra = {}) {
  return {
    "GET /me": meRoute({ activity: ["create"] }),
    "GET /me/working-hours": () => jsonResponse(bookingHours),
    "GET /connectors": () => jsonResponse({ data: [bookingConnection] }),
    "GET /scheduling/profile": () => jsonResponse(profile),
    "GET /scheduling/calendars": () => jsonResponse(calendars),
    "GET /availability": () => unreadable(),
    ...extra,
  };
}

it("names the unreadable calendar under the row that holds it on the settings page", async () => {
  installFetchStub(routes());
  render(
    <StoryProviders>
      <ToastProvider>
        <SavePageProvider>
          <MeetingSettings />
        </SavePageProvider>
      </ToastProvider>
    </StoryProviders>,
  );
  expect(await screen.findByText(NAMED)).toBeTruthy();
});

it("names the unreadable calendar in the host's preview", async () => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-10-01T12:00:00Z"));
  installFetchStub(routes());
  render(
    <StoryProviders>
      <BookingScreen hostSlug="preview" />
    </StoryProviders>,
  );
  expect(await screen.findByText(NAMED)).toBeTruthy();
});

it("names no calendar on the public page, whatever the answer carries", async () => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-10-01T12:00:00Z"));
  installFetchStub(
    routes({
      "GET /public/booking/ada-lovelace/profile": () =>
        jsonResponse(bookingProfile),
      "GET /public/booking/ada-lovelace/availability": () => unreadable(),
    }),
  );
  render(
    <StoryProviders>
      <BookingScreen hostSlug="ada-lovelace" />
    </StoryProviders>,
  );
  expect(await screen.findByText(/calendar could not be read/)).toBeTruthy();
  expect(screen.queryByText(NAMED)).toBeNull();
});

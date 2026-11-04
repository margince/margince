/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { SavePageProvider } from "../../design-system/savepage";
import { ToastProvider } from "../../design-system/toast";
import { en } from "../../i18n/en";
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
import { MeetingSettings } from "./meeting-settings";

// The server refuses every calendar connect for a member whose role or seat
// cannot capture, so Meetings shows why in place of each connect button.

type Profile = components["schemas"]["SchedulingProfile"];
type SeatCapture = components["schemas"]["SeatCapture"];

const CONTACT_MISSING: SeatCapture = {
  allowed: false,
  reason: "role_cannot_capture",
  missing: "contact",
};
const SEAT_MISSING: SeatCapture = {
  allowed: false,
  reason: "role_cannot_capture",
  missing: "seat",
};
const ALLOWED: SeatCapture = { allowed: true };

function mount(
  profile: Partial<Profile>,
  connections: unknown[],
  seatCapture: SeatCapture,
) {
  installFetchStub({
    "GET /me": meRoute({ activity: ["create"] }),
    "GET /me/working-hours": () => jsonResponse(bookingHours),
    "GET /connectors": () =>
      jsonResponse({ data: connections, seat_capture: seatCapture }),
    "GET /scheduling/profile": () =>
      jsonResponse({ ...bookingProfile, enabled: false, ...profile }),
    "GET /scheduling/calendars": () => jsonResponse([]),
  });
  render(
    <StoryProviders>
      <ToastProvider>
        <SavePageProvider>
          <MeetingSettings />
        </SavePageProvider>
      </ToastProvider>
    </StoryProviders>,
  );
}

const expired = [{ ...bookingConnection, status: "reauth_required" }];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

describe("Meetings for a member who cannot capture", () => {
  it.each([
    [CONTACT_MISSING, en["connectors.roleCannotCapture.contact"]],
    [SEAT_MISSING, en["connectors.roleCannotCapture.seat"]],
  ])(
    "offers no first calendar connect and names the missing grant",
    async (seat, says) => {
      mount({ provider: "" }, [], seat);
      expect(await screen.findByText(says)).toBeTruthy();
      expect(screen.getByText(en["scheduling.setup.title"])).toBeTruthy();
      expect(
        screen.queryByRole("button", {
          name: en["scheduling.connectCalendar"],
        }),
      ).toBeNull();
    },
  );

  it("keeps the first calendar connect for a member who may capture", async () => {
    mount({ provider: "" }, [], ALLOWED);
    expect(
      await screen.findByRole("button", {
        name: en["scheduling.connectCalendar"],
      }),
    ).toBeTruthy();
    expect(screen.queryByText(en["connectors.roleCannotCapture"])).toBeNull();
  });

  it.each([
    [CONTACT_MISSING, en["connectors.roleCannotCapture.contact"]],
    [SEAT_MISSING, en["connectors.roleCannotCapture.seat"]],
  ])(
    "offers no reconnect for an expired calendar and names the missing grant",
    async (seat, says) => {
      mount({ provider: "gcal", enabled: true }, expired, seat);
      // Once on the calendar row, once in the live-bookings warning.
      expect(await screen.findAllByText(says)).toHaveLength(2);
      expect(
        screen.queryByRole("button", { name: en["scheduling.reconnect"] }),
      ).toBeNull();
      expect(
        screen.queryByRole("button", {
          name: en["scheduling.reconnectCalendar"],
        }),
      ).toBeNull();
    },
  );

  it("keeps both reconnects for a member who may capture", async () => {
    mount({ provider: "gcal", enabled: true }, expired, ALLOWED);
    expect(
      await screen.findByRole("button", { name: en["scheduling.reconnect"] }),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: en["scheduling.reconnectCalendar"] }),
    ).toBeTruthy();
  });

  it.each([
    [CONTACT_MISSING, en["connectors.roleCannotCapture.contact"]],
    [SEAT_MISSING, en["connectors.roleCannotCapture.seat"]],
  ])(
    "says why a refused calendar connect landed back with nothing",
    async (seat, says) => {
      window.location.hash = "#/settings/meetings/role_cannot_capture/gcal";
      mount({ provider: "gcal" }, [bookingConnection], seat);
      expect(
        await screen.findByText(en["connectors.oauthNotConnected"]),
      ).toBeTruthy();
      expect(screen.getByText(says)).toBeTruthy();
    },
  );
});

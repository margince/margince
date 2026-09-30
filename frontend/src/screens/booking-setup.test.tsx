/** @vitest-environment happy-dom */
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import {
  bookingConnection,
  bookingContact,
  bookingHours,
  bookingInvitation,
  bookingProfile,
  bookingSlots,
} from "./book.testkit";
import { BookingGuestScreen } from "./booking-guest";
import { BookingInviteScreen } from "./booking-invite";
import { BookingMeetingScreen } from "./booking-meeting";
import { BookingProfileScreen } from "./booking-profile";
import { mount, view } from "./contactpage.testkit";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});
it("books from an empty Meetings tab with this contact selected", async () => {
  const user = userEvent.setup();
  mount("meetings", {
    ...view,
    activities: { data: [], page: { has_more: false } },
  });
  await user.click(
    await screen.findByRole("button", { name: "Book a meeting" }),
  );
  expect(window.location.hash).toBe("#/book/contact-p-1");
});
it("opens meeting settings in another tab without discarding the invitation draft", async () => {
  const user = userEvent.setup();
  installFetchStub({
    "GET /connectors": () => jsonResponse({ data: [bookingConnection] }),
    "GET /scheduling/profile": () =>
      jsonResponse({ ...bookingProfile, provider: "", enabled: false }),
    [`GET /contacts/${bookingContact.id}`]: () => jsonResponse(bookingContact),
    "GET /me/working-hours": () => jsonResponse(bookingHours),
  });
  render(
    <StoryProviders>
      <BookingInviteScreen contactId={bookingContact.id} />
    </StoryProviders>,
  );
  const settings = await screen.findByRole("link", {
    name: "Open meeting settings",
  });
  expect(settings.getAttribute("href")).toBe("#/settings/meetings");
  expect(settings.getAttribute("target")).toBe("_blank");
  await user.type(
    screen.getByLabelText("Message for your guest"),
    "Keep my draft",
  );
  expect(screen.queryByLabelText("Calendar provider")).toBeNull();
  expect(screen.queryByLabelText("Invitation calendar")).toBeNull();
  expect(screen.getByLabelText("Message for your guest")).toHaveProperty(
    "value",
    "Keep my draft",
  );
});
for (const status of [
  "pending",
  "rescheduling",
  "confirmed",
  "needs_attention",
]) {
  it(`opens and submits cancellation while ${status}`, async () => {
    const user = userEvent.setup();
    const changes: unknown[] = [];
    installFetchStub({
      "GET /scheduling/invitations/meeting-1": () =>
        jsonResponse({ ...bookingInvitation, status }),
      "PATCH /scheduling/invitations/meeting-1": async (req) => {
        changes.push(req);
        return jsonResponse({ ...bookingInvitation, status: "canceling" });
      },
    });
    render(
      <StoryProviders>
        <BookingMeetingScreen id="meeting-1" />
      </StoryProviders>,
    );
    await user.click(
      await screen.findByRole("button", { name: "Cancel meeting" }),
    );
    const dialog = await screen.findByRole("dialog");
    await user.click(
      within(dialog).getByRole("button", { name: "Cancel meeting" }),
    );
    await waitFor(() =>
      expect(changes).toEqual([{ action: "cancel", version: 1 }]),
    );
    expect(
      await screen.findByRole("heading", { name: "Canceling your meeting…" }),
    ).toBeTruthy();
  });
}
it("shows the personal proposal's title, duration and location", async () => {
  installFetchStub({
    "GET /public/proposal/personal": () =>
      jsonResponse({
        profile: {
          ...bookingProfile,
          title: "Architecture review",
          location: "Room 4",
          duration_minutes: 60,
        },
        options: [],
        used: false,
      }),
    "GET /public/proposal/personal/availability": () =>
      jsonResponse({ slots: bookingSlots, truncated: false }),
  });
  render(
    <StoryProviders>
      <BookingGuestScreen hostSlug="" proposalToken="personal" />
    </StoryProviders>,
  );
  expect(
    await screen.findByRole("heading", { name: "Architecture review" }),
  ).toBeTruthy();
  expect(screen.getByText("Room 4")).toBeTruthy();
  expect(screen.getByText("60 min")).toBeTruthy();
});

it("refreshes the worker's version after a cancellation conflict", async () => {
  const user = userEvent.setup();
  let version = 1;
  const changes: unknown[] = [];
  installFetchStub({
    "GET /scheduling/invitations/meeting-1": () =>
      jsonResponse({ ...bookingInvitation, version }),
    "PATCH /scheduling/invitations/meeting-1": (body) => {
      changes.push(body);
      if (changes.length === 1) {
        version = 2;
        return jsonResponse(
          { code: "conflict", status: 409, title: "Conflict" },
          409,
        );
      }
      return jsonResponse({
        ...bookingInvitation,
        version: 3,
        status: "canceling",
      });
    },
  });
  render(
    <StoryProviders>
      <BookingMeetingScreen id="meeting-1" />
    </StoryProviders>,
  );
  await user.click(
    await screen.findByRole("button", { name: "Cancel meeting" }),
  );
  const dialog = await screen.findByRole("dialog");
  await user.click(
    within(dialog).getByRole("button", { name: "Cancel meeting" }),
  );
  expect(
    await within(dialog).findByText(
      "The meeting changed. Review the updated details and try again.",
    ),
  ).toBeTruthy();
  await waitFor(() =>
    expect(
      within(dialog).getByRole("button", { name: "Cancel meeting" }),
    ).toHaveProperty("disabled", false),
  );
  await user.click(
    within(dialog).getByRole("button", { name: "Cancel meeting" }),
  );
  expect(
    await screen.findByRole("heading", { name: "Canceling your meeting…" }),
  ).toBeTruthy();
  expect(changes).toEqual([
    { action: "cancel", version: 1 },
    { action: "cancel", version: 2 },
  ]);
});

it("warns when an active public page cannot send calendar invitations", async () => {
  installFetchStub({
    "GET /connectors": () =>
      jsonResponse({ data: [{ ...bookingConnection, scopes: [] }] }),
    "GET /scheduling/profile": () => jsonResponse(bookingProfile),
  });
  render(
    <StoryProviders>
      <BookingProfileScreen />
    </StoryProviders>,
  );
  expect(
    await screen.findByText(
      "Your booking page is active, but calendar invitations are unavailable. Open meeting settings to check the connection or pause the page.",
    ),
  ).toBeTruthy();
  expect(screen.getByRole("button", { name: "Pause bookings" })).toBeTruthy();
});

it("pauses a public link without overwriting meeting settings saved in another tab", async () => {
  const user = userEvent.setup();
  let latest = { ...bookingProfile, enabled: true };
  const writes: unknown[] = [];
  installFetchStub({
    "GET /connectors": () => jsonResponse({ data: [bookingConnection] }),
    "GET /scheduling/profile": () => jsonResponse(latest),
    "PUT /scheduling/profile": (body) => {
      writes.push(body);
      return jsonResponse({ ...latest, enabled: false });
    },
  });
  render(
    <StoryProviders>
      <BookingProfileScreen />
    </StoryProviders>,
  );
  const pause = await screen.findByRole("button", { name: "Pause bookings" });
  latest = { ...latest, horizon_days: 90, calendar_id: "another-calendar" };
  await user.click(pause);
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).toMatchObject({
    enabled: false,
    horizon_days: 90,
    calendar_id: "another-calendar",
  });
});

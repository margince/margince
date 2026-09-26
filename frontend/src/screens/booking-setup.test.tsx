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

const readyCalendar = {
  id: "ada@example.test",
  name: "Work calendar",
  writable: true,
  primary: true,
};
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
it("selects an already-connected calendar without leaving or losing the invitation", async () => {
  const user = userEvent.setup();
  const saved: unknown[] = [];
  installFetchStub({
    "GET /connectors": () => jsonResponse({ data: [bookingConnection] }),
    "GET /scheduling/profile": () =>
      jsonResponse({ ...bookingProfile, provider: "", enabled: false }),
    "GET /scheduling/calendars": () => jsonResponse([readyCalendar]),
    [`GET /contacts/${bookingContact.id}`]: () => jsonResponse(bookingContact),
    "GET /availability": () =>
      jsonResponse({ slots: bookingSlots, truncated: false }),
    "PUT /scheduling/profile": async (req) => {
      const body = req;
      saved.push(body);
      return jsonResponse({
        ...bookingProfile,
        enabled: false,
        calendar_id: readyCalendar.id,
      });
    },
  });
  render(
    <StoryProviders>
      <BookingInviteScreen contactId={bookingContact.id} />
    </StoryProviders>,
  );
  expect(await screen.findByText(/Invitation access granted/)).toBeTruthy();
  await user.type(
    screen.getByLabelText("Message for your guest"),
    "Discuss our project",
  );
  await user.click(screen.getByRole("button", { name: "Use this calendar" }));
  await waitFor(() => expect(saved).toHaveLength(1));
  expect(saved[0]).toMatchObject({ provider: "gcal", enabled: false });
  expect(screen.getByLabelText("Message for your guest")).toHaveProperty(
    "value",
    "Discuss our project",
  );
  expect(screen.getByDisplayValue(bookingContact.primary_email)).toBeTruthy();
  await waitFor(() =>
    expect(
      screen.queryByRole("button", { name: "Use this calendar" }),
    ).toBeNull(),
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

it("keeps settings changed in another tab when saving the calendar", async () => {
  const user = userEvent.setup();
  let latest = { ...bookingProfile, provider: "", enabled: true };
  const saved: unknown[] = [];
  installFetchStub({
    "GET /connectors": () => jsonResponse({ data: [bookingConnection] }),
    "GET /scheduling/profile": () => jsonResponse(latest),
    "GET /scheduling/calendars": () => jsonResponse([readyCalendar]),
    [`GET /contacts/${bookingContact.id}`]: () => jsonResponse(bookingContact),
    "GET /availability": () => jsonResponse({ slots: [], truncated: false }),
    "PUT /scheduling/profile": (body) => {
      saved.push(body);
      return jsonResponse({ ...latest, provider: "gcal" });
    },
  });
  render(
    <StoryProviders>
      <BookingInviteScreen contactId={bookingContact.id} />
    </StoryProviders>,
  );
  await screen.findByText(/Invitation access granted/);
  latest = {
    ...latest,
    enabled: false,
    title: "Changed in another tab",
    buffer_minutes: 25,
  };
  await user.click(screen.getByRole("button", { name: "Use this calendar" }));
  await waitFor(() => expect(saved).toHaveLength(1));
  expect(saved[0]).toMatchObject({
    enabled: false,
    title: "Changed in another tab",
    buffer_minutes: 25,
    provider: "gcal",
  });
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

it("keeps calendar-error recovery in another tab", async () => {
  installFetchStub({
    "GET /connectors": () => jsonResponse({ data: [bookingConnection] }),
    "GET /scheduling/profile": () =>
      jsonResponse({ ...bookingProfile, provider: "" }),
    "GET /scheduling/calendars": () =>
      jsonResponse(
        { code: "unavailable", title: "Calendar unavailable", status: 503 },
        503,
      ),
    [`GET /contacts/${bookingContact.id}`]: () => jsonResponse(bookingContact),
  });
  render(
    <StoryProviders>
      <BookingInviteScreen contactId={bookingContact.id} />
    </StoryProviders>,
  );
  const recovery = await screen.findByRole("link", {
    name: "Open calendar connections",
  });
  expect(recovery.getAttribute("target")).toBe("_blank");
  expect(recovery.getAttribute("href")).toBe("#/settings/connections");
});

it("explains that an expired calendar needs reconnection", async () => {
  installFetchStub({
    "GET /connectors": () =>
      jsonResponse({
        data: [{ ...bookingConnection, status: "reauth_required" }],
      }),
    "GET /scheduling/profile": () => jsonResponse(bookingProfile),
    [`GET /contacts/${bookingContact.id}`]: () => jsonResponse(bookingContact),
  });
  render(
    <StoryProviders>
      <BookingInviteScreen contactId={bookingContact.id} />
    </StoryProviders>,
  );
  expect(
    await screen.findByText(
      "Your calendar connection has expired. Reconnect it to send invitations.",
    ),
  ).toBeTruthy();
  expect(
    screen.getByRole("button", { name: "Use this calendar" }),
  ).toHaveProperty("disabled", true);
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
      "Your booking page is active, but calendar invitations are unavailable. Check the calendar connection below or pause the page.",
    ),
  ).toBeTruthy();
  expect(screen.getByRole("button", { name: "Pause bookings" })).toBeTruthy();
});

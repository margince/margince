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
import type { components } from "../../api/schema";
import { stubClipboard } from "../../design-system/clipboard-testing";
import { en as englishCopy } from "../../i18n/en";
import {
  bookingConnection,
  bookingContact,
  bookingHours,
  bookingInvitation,
  bookingProfile,
  bookingSlots,
} from "../../testing/book.testkit";
import { mount, view } from "../../testing/contactpage.testkit";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { BookingGuestScreen } from "./booking-guest";
import { BookingInviteScreen } from "./booking-invite";
import { BookingMeetingScreen } from "./booking-meeting";
import { BookingProfileScreen } from "./booking-profile";

type SchedulingProfile = components["schemas"]["SchedulingProfile"];

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
    await screen.findByRole("button", { name: englishCopy["scheduling.new"] }),
  );
  expect(window.location.hash).toBe("#/book/contact-p-1");
});
it("opens meeting settings in another tab without discarding the invitation draft", async () => {
  const user = userEvent.setup();
  installFetchStub({
    "GET /me": meRoute({ activity: ["read", "create"] }),
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
    name: englishCopy["scheduling.openSettings"],
  });
  expect(settings.getAttribute("href")).toBe("/#/settings/meetings");
  expect(settings.getAttribute("target")).toBe("_blank");
  await user.type(
    screen.getByLabelText(englishCopy["scheduling.agenda"]),
    "Keep my draft",
  );
  expect(
    screen.queryByLabelText(englishCopy["scheduling.provider"]),
  ).toBeNull();
  expect(screen.queryByLabelText("Invitation calendar")).toBeNull();
  expect(
    screen.getByLabelText(englishCopy["scheduling.agenda"]),
  ).toHaveProperty("value", "Keep my draft");
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
      await screen.findByRole("button", {
        name: englishCopy["scheduling.cancel"],
      }),
    );
    const dialog = await screen.findByRole("dialog");
    await user.click(
      within(dialog).getByRole("button", {
        name: englishCopy["scheduling.cancel"],
      }),
    );
    await waitFor(() =>
      expect(changes).toEqual([{ action: "cancel", version: 1 }]),
    );
    await waitFor(() =>
      expect(screen.getByRole("status").textContent).toBe(
        englishCopy["scheduling.canceling"],
      ),
    );
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
    await screen.findByRole("button", {
      name: englishCopy["scheduling.cancel"],
    }),
  );
  const dialog = await screen.findByRole("dialog");
  await user.click(
    within(dialog).getByRole("button", {
      name: englishCopy["scheduling.cancel"],
    }),
  );
  expect(
    await within(dialog).findByText(englishCopy["book.meetingChanged"]),
  ).toBeTruthy();
  await waitFor(() =>
    expect(
      within(dialog).getByRole("button", {
        name: englishCopy["scheduling.cancel"],
      }),
    ).toHaveProperty("disabled", false),
  );
  await user.click(
    within(dialog).getByRole("button", {
      name: englishCopy["scheduling.cancel"],
    }),
  );
  await waitFor(() =>
    expect(screen.getByRole("status").textContent).toBe(
      englishCopy["scheduling.canceling"],
    ),
  );
  expect(changes).toEqual([
    { action: "cancel", version: 1 },
    { action: "cancel", version: 2 },
  ]);
});

it("warns when an active public page cannot send calendar invitations", async () => {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read", "create"] }),
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
      englishCopy["scheduling.publicCalendarUnavailable"],
    ),
  ).toBeTruthy();
  expect(
    screen
      .getByRole("switch", { name: englishCopy["scheduling.acceptBookings"] })
      .getAttribute("aria-checked"),
  ).toBe("true");
});

it("pauses a public link without overwriting meeting settings saved in another tab", async () => {
  const user = userEvent.setup();
  let latest = { ...bookingProfile, enabled: true };
  const writes: unknown[] = [];
  installFetchStub({
    "GET /me": meRoute({ activity: ["read", "create"] }),
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
  const pause = await screen.findByRole("switch", {
    name: englishCopy["scheduling.acceptBookings"],
  });
  latest = { ...latest, horizon_days: 90, calendar_id: "another-calendar" };
  await user.click(pause);
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).toMatchObject({
    enabled: false,
    horizon_days: 90,
    calendar_id: "another-calendar",
  });
});

it("keeps focus on Accept bookings after its answer comes back", async () => {
  const user = userEvent.setup();
  let latest = { ...bookingProfile, enabled: true };
  installFetchStub({
    "GET /me": meRoute({ activity: ["read", "create"] }),
    "GET /connectors": () => jsonResponse({ data: [bookingConnection] }),
    "GET /scheduling/profile": () => jsonResponse(latest),
    "PUT /scheduling/profile": () => {
      latest = { ...latest, enabled: false };
      return jsonResponse(latest);
    },
  });
  render(
    <StoryProviders>
      <BookingProfileScreen />
    </StoryProviders>,
  );
  const accept = await screen.findByRole("switch", {
    name: englishCopy["scheduling.acceptBookings"],
  });
  await user.click(accept);
  await waitFor(() =>
    expect(accept.getAttribute("aria-checked")).toBe("false"),
  );
  expect(document.activeElement).toBe(accept);
});

it("names the link once on its own page, in the heading and not again on the card", async () => {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read", "create"] }),
    "GET /connectors": () => jsonResponse({ data: [bookingConnection] }),
    "GET /scheduling/profile": () => jsonResponse(bookingProfile),
  });
  render(
    <StoryProviders>
      <BookingProfileScreen />
    </StoryProviders>,
  );
  await screen.findByRole("switch", {
    name: englishCopy["scheduling.acceptBookings"],
  });
  expect(
    screen.getAllByRole("heading", { name: englishCopy["scheduling.myLink"] }),
  ).toHaveLength(1);
  expect(screen.getAllByText(/One link for your email signature/)).toHaveLength(
    1,
  );
});

it("keeps focus on Accept bookings when the answer brings the first link", async () => {
  const user = userEvent.setup();
  let latest: SchedulingProfile = {
    ...bookingProfile,
    enabled: false,
    public_url: undefined,
  };
  installFetchStub({
    "GET /me": meRoute({ activity: ["read", "create"] }),
    "GET /connectors": () => jsonResponse({ data: [bookingConnection] }),
    "GET /scheduling/profile": () => jsonResponse(latest),
    "PUT /scheduling/profile": () => {
      latest = { ...bookingProfile, enabled: true };
      return jsonResponse(latest);
    },
  });
  render(
    <StoryProviders>
      <BookingProfileScreen />
    </StoryProviders>,
  );
  const accept = await screen.findByRole("switch", {
    name: englishCopy["scheduling.acceptBookings"],
  });
  await user.click(accept);
  await screen.findByRole("textbox", {
    name: englishCopy["scheduling.myLink"],
  });
  expect(accept.getAttribute("aria-checked")).toBe("true");
  expect(document.activeElement).toBe(accept);
});

it("hands focus back to Replace link once the new link lands", async () => {
  const user = userEvent.setup();
  let latest = { ...bookingProfile };
  installFetchStub({
    "GET /me": meRoute({ activity: ["read", "create"] }),
    "GET /connectors": () => jsonResponse({ data: [bookingConnection] }),
    "GET /scheduling/profile": () => jsonResponse(latest),
    "PUT /scheduling/profile": () => {
      latest = { ...latest, public_url: "https://book.example/new-link" };
      return jsonResponse(latest);
    },
  });
  render(
    <StoryProviders>
      <BookingProfileScreen />
    </StoryProviders>,
  );
  const replace = await screen.findByRole("button", {
    name: englishCopy["scheduling.replaceAction"],
  });
  await user.click(replace);
  const dialog = await screen.findByRole("dialog");
  await user.click(
    within(dialog).getByRole("button", {
      name: englishCopy["scheduling.replace"],
    }),
  );
  await waitFor(() =>
    expect(
      screen.getByRole("textbox", { name: englishCopy["scheduling.myLink"] }),
    ).toHaveProperty("value", "https://book.example/new-link"),
  );
  await waitFor(() => expect(document.activeElement).toBe(replace));
});

it("keeps a failed copy's remedy over a replaced link until the next copy", async () => {
  const user = userEvent.setup();
  let latest = { ...bookingProfile };
  installFetchStub({
    "GET /me": meRoute({ activity: ["read", "create"] }),
    "GET /connectors": () => jsonResponse({ data: [bookingConnection] }),
    "GET /scheduling/profile": () => jsonResponse(latest),
    "PUT /scheduling/profile": () => {
      latest = { ...latest, public_url: "https://book.example/new-link" };
      return jsonResponse(latest);
    },
  });
  render(
    <StoryProviders>
      <BookingProfileScreen />
    </StoryProviders>,
  );
  stubClipboard("refuses");
  await user.click(await screen.findByRole("button", { name: "Copy link" }));
  await screen.findByText(/clipboard access denied/i);
  await user.click(
    screen.getByRole("button", {
      name: englishCopy["scheduling.replaceAction"],
    }),
  );
  await user.click(
    within(await screen.findByRole("dialog")).getByRole("button", {
      name: englishCopy["scheduling.replace"],
    }),
  );
  await waitFor(() =>
    expect(
      screen.getByRole("textbox", { name: englishCopy["scheduling.myLink"] }),
    ).toHaveProperty("value", "https://book.example/new-link"),
  );
  expect(screen.getByText(/clipboard access denied/i)).toBeTruthy();
  const accepted = stubClipboard("accepts");
  await user.click(screen.getByRole("button", { name: "Copy link" }));
  expect(await screen.findByRole("button", { name: "Copied" })).toBeTruthy();
  expect(screen.queryByText(/clipboard access denied/i)).toBeNull();
  expect(accepted.written).toEqual(["https://book.example/new-link"]);
});

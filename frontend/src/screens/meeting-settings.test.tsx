/** @vitest-environment happy-dom */
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { stubClipboard } from "../design-system/clipboard-testing";
import {
  bookingConnection,
  bookingHours,
  bookingProfile,
} from "./book.testkit";
import { MeetingSettings } from "./meeting-settings";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

const calendars = [
  { id: "work", name: "Work calendar", primary: true, writable: true },
  {
    id: "personal",
    name: "Personal calendar",
    primary: false,
    writable: false,
  },
];
function mount(overrides: RouteMap = {}) {
  installFetchStub({
    "GET /me": meRoute({ activity: ["create"] }),
    "GET /me/working-hours": () => jsonResponse(bookingHours),
    "GET /connectors": () => jsonResponse({ data: [bookingConnection] }),
    "GET /scheduling/profile": () =>
      jsonResponse({ ...bookingProfile, provider: "", enabled: false }),
    "GET /scheduling/calendars": () => jsonResponse(calendars),
    ...overrides,
  });
  render(
    <StoryProviders>
      <MeetingSettings />
    </StoryProviders>,
  );
}
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});
it("uses the only connected provider and keeps hours and calendar choices together", async () => {
  const user = userEvent.setup();
  const writes: unknown[] = [];
  mount({
    "PUT /scheduling/profile": (body) => {
      writes.push(body);
      return jsonResponse({
        ...bookingProfile,
        enabled: false,
        calendar_id: "work",
      });
    },
  });
  const save = await screen.findByRole("button", { name: "Save settings" });
  await waitFor(() => expect(save).toHaveProperty("disabled", false));
  expect(screen.queryByLabelText("Calendar provider")).toBeNull();
  expect(screen.queryByText("Microsoft Outlook")).toBeNull();
  expect(screen.getByRole("heading", { name: "Bookable hours" })).toBeTruthy();
  expect(screen.getByLabelText("Timezone")).toBeTruthy();
  expect(
    screen.queryByRole("checkbox", { name: "Ignore all-day events" }),
  ).toBeNull();
  await user.click(
    screen.getByRole("combobox", { name: "Other calendars that block time" }),
  );
  expect(screen.queryByRole("option", { name: "Work calendar" })).toBeNull();
  await user.click(screen.getByRole("option", { name: "Personal calendar" }));
  await user.keyboard("{Escape}");
  await user.click(save);
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).toMatchObject({
    provider: "gcal",
    calendar_id: "work",
    blocking_calendars: ["personal"],
    enabled: false,
  });
});
it("offers a provider choice only when both calendars are connected", async () => {
  const user = userEvent.setup();
  mount({
    "GET /connectors": () =>
      jsonResponse({
        data: [
          bookingConnection,
          {
            ...bookingConnection,
            id: "outlook",
            provider: "graphcal",
            scopes: ["Calendars.ReadWrite"],
          },
          { ...bookingConnection, id: "mail", provider: "gmail" },
        ],
      }),
  });
  const choice = await screen.findByRole("combobox", {
    name: "Calendar provider",
  });
  await user.click(choice);
  expect(screen.getByRole("option", { name: "Google Calendar" })).toBeTruthy();
  expect(
    screen.getByRole("option", { name: "Microsoft Outlook" }),
  ).toBeTruthy();
  expect(screen.queryByRole("option", { name: "Gmail" })).toBeNull();
  await user.click(screen.getByRole("option", { name: "Microsoft Outlook" }));
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: "Save settings" }),
    ).toHaveProperty("disabled", false),
  );
});
it("explains read-only access and keeps reconnection separate from the settings draft", async () => {
  mount({
    "GET /connectors": () =>
      jsonResponse({
        data: [
          {
            ...bookingConnection,
            scopes: ["https://www.googleapis.com/auth/calendar.readonly"],
          },
        ],
      }),
  });
  expect(await screen.findByText(/connected for reading events/)).toBeTruthy();
  const link = screen.getByRole("link", { name: "Open calendar connections" });
  expect(link.getAttribute("target")).toBe("_blank");
  expect(screen.getByRole("button", { name: "Save settings" })).toHaveProperty(
    "disabled",
    false,
  );
});
it("preserves a public page paused in another tab when saving meeting preferences", async () => {
  const user = userEvent.setup();
  let latest = { ...bookingProfile, enabled: true };
  const writes: unknown[] = [];
  mount({
    "GET /scheduling/profile": () => jsonResponse(latest),
    "PUT /scheduling/profile": (body) => {
      writes.push(body);
      return jsonResponse(latest);
    },
  });
  const save = await screen.findByRole("button", { name: "Save settings" });
  await waitFor(() => expect(save).toHaveProperty("disabled", false));
  latest = { ...latest, enabled: false };
  await user.click(save);
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).toMatchObject({ enabled: false });
});
it("does not offer disconnected providers or retain their old calendar ids", async () => {
  const user = userEvent.setup();
  const writes: unknown[] = [];
  mount({
    "GET /scheduling/profile": () =>
      jsonResponse({
        ...bookingProfile,
        provider: "graphcal",
        calendar_id: "old-outlook",
        blocking_calendars: ["old-private"],
      }),
    "GET /connectors": () =>
      jsonResponse({
        data: [
          bookingConnection,
          {
            ...bookingConnection,
            id: "old",
            provider: "graphcal",
            status: "disconnected",
          },
        ],
      }),
    "PUT /scheduling/profile": (body) => {
      writes.push(body);
      return jsonResponse(bookingProfile);
    },
  });
  const save = await screen.findByRole("button", { name: "Save settings" });
  await waitFor(() => expect(save).toHaveProperty("disabled", false));
  await user.click(save);
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).toMatchObject({
    provider: "gcal",
    calendar_id: "work",
    blocking_calendars: [],
  });
});

it("edits the host name while company branding comes from the anchor", async () => {
  const user = userEvent.setup();
  const writes: unknown[] = [];
  mount({
    "PUT /scheduling/profile": (body) => {
      writes.push(body);
      return jsonResponse(bookingProfile);
    },
  });
  const name = await screen.findByLabelText("Your public name");
  await user.clear(name);
  await user.type(name, "Ada Example");
  expect(screen.queryByLabelText("Company name")).toBeNull();
  expect(screen.queryByLabelText("Public company logo URL")).toBeNull();
  expect(
    screen.getByText(
      /Company name and logo are taken from your company profile/,
    ),
  ).toBeTruthy();
  expect(
    screen.getByPlaceholderText("https://meet.google.com/abc-defg-hij"),
  ).toBeTruthy();
  await user.click(screen.getByRole("button", { name: "Save settings" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).toMatchObject({
    host_name: "Ada Example",
  });
});

it("preserves another tab's horizon when only the title was edited", async () => {
  const user = userEvent.setup();
  let latest = { ...bookingProfile, calendar_id: "work" };
  const writes: unknown[] = [];
  mount({
    "GET /scheduling/profile": () => jsonResponse(latest),
    "PUT /scheduling/profile": (body) => {
      writes.push(body);
      return jsonResponse(latest);
    },
  });
  const title = await screen.findByLabelText("Meeting title");
  await user.clear(title);
  await user.type(title, "Updated subject");
  latest = { ...latest, horizon_days: 90 };
  await user.click(screen.getByRole("button", { name: "Save settings" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).toMatchObject({
    title: "Updated subject",
    horizon_days: 90,
  });
});
it("allows editing a paused page's policy without a calendar", async () => {
  const user = userEvent.setup();
  const writes: unknown[] = [];
  mount({
    "GET /connectors": () => jsonResponse({ data: [] }),
    "PUT /scheduling/profile": (body) => {
      writes.push(body);
      return jsonResponse({ ...bookingProfile, provider: "", enabled: false });
    },
  });
  const title = await screen.findByLabelText("Meeting title");
  await user.clear(title);
  await user.type(title, "Prepare before connecting");
  await user.click(screen.getByRole("button", { name: "Save settings" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).toMatchObject({
    provider: "",
    title: "Prepare before connecting",
    enabled: false,
  });
});
it("explains a missing saved calendar and requires a replacement for active bookings", async () => {
  const user = userEvent.setup();
  mount({
    "GET /scheduling/profile": () =>
      jsonResponse({
        ...bookingProfile,
        calendar_id: "deleted",
        enabled: true,
      }),
  });
  expect(
    await screen.findByText(/saved event calendar is unavailable/),
  ).toBeTruthy();
  expect(screen.getByRole("button", { name: "Save settings" })).toHaveProperty(
    "disabled",
    true,
  );
  await user.click(screen.getByRole("combobox", { name: "Event calendar" }));
  await user.click(screen.getByRole("option", { name: "Work calendar" }));
  expect(screen.getByRole("button", { name: "Save settings" })).toHaveProperty(
    "disabled",
    false,
  );
});
it("explains expired authorization without hiding the settings draft", async () => {
  mount({
    "GET /connectors": () =>
      jsonResponse({
        data: [{ ...bookingConnection, status: "reauth_required" }],
      }),
  });
  expect(await screen.findByText(/expired/)).toBeTruthy();
  expect(
    screen
      .getByRole("link", { name: "Open calendar connections" })
      .getAttribute("target"),
  ).toBe("_blank");
  expect(screen.getByLabelText("Meeting title")).toBeTruthy();
});

it("sets a public name during setup and then enables the reusable booking link", async () => {
  const user = userEvent.setup();
  let latest: unknown = {
    ...bookingProfile,
    host_name: null,
    provider: "",
    enabled: false,
  };
  const writes: unknown[] = [];
  installFetchStub({
    "GET /me": meRoute({ activity: ["create"] }),
    "GET /me/working-hours": () => jsonResponse(bookingHours),
    "GET /connectors": () => jsonResponse({ data: [bookingConnection] }),
    "GET /scheduling/calendars": () => jsonResponse(calendars),
    "GET /scheduling/profile": () => jsonResponse(latest),
    "PUT /scheduling/profile": (body) => {
      latest = body;
      writes.push(body);
      return jsonResponse(body);
    },
  });
  render(
    <StoryProviders>
      <MeetingSettings />
    </StoryProviders>,
  );
  await user.type(
    await screen.findByLabelText("Your public name"),
    "Ada Example",
  );
  const save = screen.getByRole("button", { name: "Save settings" });
  await waitFor(() => expect(save).toHaveProperty("disabled", false));
  await user.click(save);
  await waitFor(() => expect(writes).toHaveLength(1));
  await user.click(screen.getByRole("button", { name: "Enable bookings" }));
  await waitFor(() => expect(writes).toHaveLength(2));
  expect(writes[1]).toMatchObject({
    host_name: "Ada Example",
    provider: "gcal",
    calendar_id: "work",
    enabled: true,
  });
});

it.each([
  ["24", 1440],
  ["1.5", 90],
  ["0", 0],
  ["168", 10080],
])("saves %s hours as %i minutes", async (hours, minutes) => {
  const user = userEvent.setup();
  const writes: unknown[] = [];
  mount({
    "PUT /scheduling/profile": (body) => {
      writes.push(body);
      return jsonResponse({ ...bookingProfile, notice_minutes: minutes });
    },
  });
  const notice = await screen.findByLabelText("Minimum notice in hours");
  expect(notice).toHaveProperty("value", "2");
  await user.clear(notice);
  await user.type(notice, hours);
  await user.click(screen.getByRole("button", { name: "Save settings" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).toMatchObject({ notice_minutes: minutes });
});
it("puts the reusable link before working hours and copies it", async () => {
  const user = userEvent.setup();
  const copy = stubClipboard("accepts");
  mount();
  const link = await screen.findByRole("textbox", { name: "My booking link" });
  expect(link).toHaveProperty("value", bookingProfile.public_url);
  const hours = screen.getByRole("heading", { name: "Bookable hours" });
  expect(
    link.compareDocumentPosition(hours) & Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBeTruthy();
  await user.click(screen.getByRole("button", { name: "Copy link" }));
  expect(copy.written).toEqual([bookingProfile.public_url]);
  await user.click(screen.getByRole("button", { name: "Preview public page" }));
  expect(window.location.hash).toBe("#/book/preview");
});

it("uses the link's current enabled state without losing an edited title", async () => {
  const user = userEvent.setup();
  let latest: unknown = { ...bookingProfile, enabled: false };
  mount({
    "GET /scheduling/profile": () => jsonResponse(latest),
    "PUT /scheduling/profile": (body) => {
      latest = body;
      return jsonResponse(body);
    },
  });
  const title = await screen.findByLabelText("Meeting title");
  await user.clear(title);
  await user.type(title, "Keep this draft");
  const name = screen.getByLabelText("Your public name");
  expect(name).toHaveProperty("required", false);
  await user.click(screen.getByRole("button", { name: "Enable bookings" }));
  await waitFor(() => expect(name).toHaveProperty("required", true));
  expect(title).toHaveProperty("value", "Keep this draft");
  await user.click(screen.getByRole("button", { name: "Pause bookings" }));
  await waitFor(() => expect(name).toHaveProperty("required", false));
  expect(title).toHaveProperty("value", "Keep this draft");
});
it("displays an old minute value concisely without changing it on unrelated saves", async () => {
  const user = userEvent.setup();
  const writes: unknown[] = [];
  mount({
    "GET /scheduling/profile": () =>
      jsonResponse({ ...bookingProfile, notice_minutes: 50 }),
    "PUT /scheduling/profile": (body) => {
      writes.push(body);
      return jsonResponse(body);
    },
  });
  expect(
    await screen.findByLabelText("Minimum notice in hours"),
  ).toHaveProperty("value", "0.83");
  const title = screen.getByLabelText("Meeting title");
  await user.clear(title);
  await user.type(title, "Updated title");
  await user.click(screen.getByRole("button", { name: "Save settings" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).toMatchObject({ notice_minutes: 50 });
});

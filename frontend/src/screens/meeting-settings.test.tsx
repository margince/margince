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
  expect(screen.getByRole("heading", { name: "Availability" })).toBeTruthy();
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
  await user.type(await screen.findByLabelText("Meeting title"), "!");
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

it("uses the Account name while company branding comes from the anchor", async () => {
  const user = userEvent.setup();
  const writes: unknown[] = [];
  mount({
    "PUT /scheduling/profile": (body) => {
      writes.push(body);
      return jsonResponse(bookingProfile);
    },
  });
  expect(await screen.findByText("Your public name")).toBeTruthy();
  expect(screen.getByText(bookingProfile.host_name ?? "")).toBeTruthy();
  expect(screen.queryByLabelText("Your public name")).toBeNull();
  expect(
    screen.getByRole("link", { name: "Account" }).getAttribute("href"),
  ).toBe("#/settings/account");
  expect(screen.queryByLabelText("Company name")).toBeNull();
  expect(screen.queryByLabelText("Public company logo URL")).toBeNull();
  expect(screen.getByText("Company name and logo")).toBeTruthy();
  expect(
    screen.getByPlaceholderText("https://meet.google.com/abc-defg-hij"),
  ).toBeTruthy();
  await user.type(screen.getByLabelText("Meeting title"), "!");
  await user.click(screen.getByRole("button", { name: "Save settings" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).toMatchObject({
    host_name: bookingProfile.host_name,
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
  await user.type(screen.getByLabelText("Meeting title"), "!");
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

it("saves calendar setup and enables the reusable link using the Account name", async () => {
  const user = userEvent.setup();
  let latest: unknown = {
    ...bookingProfile,
    host_name: bookingProfile.host_name,
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
  expect(await screen.findByText("Your public name")).toBeTruthy();
  expect(
    screen.getByRole("button", { name: "Enable bookings" }),
  ).toHaveProperty("disabled", true);
  expect(
    screen.getByText(
      "Choose and save a calendar in meeting settings before enabling bookings.",
    ),
  ).toBeDefined();
  const save = await screen.findByRole("button", { name: "Save settings" });
  await waitFor(() => expect(save).toHaveProperty("disabled", false));
  await user.click(save);
  await waitFor(() => expect(writes).toHaveLength(1));
  await waitFor(() =>
    expect(screen.queryByRole("button", { name: "Save settings" })).toBeNull(),
  );
  await user.click(screen.getByRole("button", { name: "Enable bookings" }));
  await waitFor(() => expect(writes).toHaveLength(2));
  expect(writes[1]).toMatchObject({
    host_name: bookingProfile.host_name,
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
it("orders the page by dependency, with the booking link after its calendar and hours", async () => {
  const user = userEvent.setup();
  const copy = stubClipboard("accepts");
  mount();
  const link = await screen.findByRole("textbox", { name: "My booking link" });
  expect(link).toHaveProperty("value", bookingProfile.public_url);
  const headings = screen
    .getAllByRole("heading")
    .map((heading) => heading.textContent);
  expect(headings.indexOf("Calendar")).toBeLessThan(
    headings.indexOf("Availability"),
  );
  expect(headings.indexOf("Availability")).toBeLessThan(
    headings.indexOf("Meeting defaults"),
  );
  expect(headings.indexOf("Meeting defaults")).toBeLessThan(
    headings.indexOf("My booking link"),
  );
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
  await user.click(screen.getByRole("button", { name: "Enable bookings" }));
  await screen.findByRole("button", { name: "Pause bookings" });
  expect(title).toHaveProperty("value", "Keep this draft");
  await user.click(screen.getByRole("button", { name: "Pause bookings" }));
  await screen.findByRole("button", { name: "Enable bookings" });
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

it.each([false, true])(
  "distinguishes an uncreated link from a missing public address (created=%s)",
  async (created) => {
    mount({
      "GET /scheduling/profile": () =>
        jsonResponse({
          ...bookingProfile,
          enabled: false,
          provider: "",
          slug: created ? "ada" : undefined,
          public_url: undefined,
        }),
    });
    expect(
      await screen.findByText(
        created ? /Set a public address/ : /Save meeting settings to create/,
      ),
    ).toBeTruthy();
    expect(
      screen.queryByRole("textbox", { name: "My booking link" }),
    ).toBeNull();
    expect(screen.getByRole("button", { name: "Copy link" })).toHaveProperty(
      "disabled",
      true,
    );
    expect(
      screen.getByRole("button", { name: "Enable bookings" }),
    ).toHaveProperty("disabled", true);
  },
);

it("adds a video link to new meetings by default and saves switching it off", async () => {
  const user = userEvent.setup();
  const writes: unknown[] = [];
  mount({
    "GET /scheduling/profile": () =>
      jsonResponse({ ...bookingProfile, calendar_id: "work" }),
    "PUT /scheduling/profile": (body) => {
      writes.push(body);
      return jsonResponse(body);
    },
  });
  const video = await screen.findByRole("switch", {
    name: "Add a Google Meet link to new meetings",
  });
  expect(video.getAttribute("aria-checked")).toBe("true");
  expect(screen.queryByRole("button", { name: "Save settings" })).toBeNull();
  await user.click(video);
  await user.click(screen.getByRole("button", { name: "Save settings" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).toMatchObject({ video_call: false });
});

it("names Microsoft Teams for an Outlook calendar", async () => {
  mount({
    "GET /connectors": () =>
      jsonResponse({
        data: [
          {
            ...bookingConnection,
            provider: "graphcal",
            scopes: ["Calendars.ReadWrite"],
          },
        ],
      }),
    "GET /scheduling/profile": () =>
      jsonResponse({ ...bookingProfile, provider: "graphcal" }),
  });
  expect(
    await screen.findByRole("switch", {
      name: "Add a Microsoft Teams link to new meetings",
    }),
  ).toBeTruthy();
  expect(screen.getByText(/work or school Microsoft 365/)).toBeTruthy();
});

it("saves hours and meeting preferences together from one save bar, and discards both", async () => {
  const user = userEvent.setup();
  const profileWrites: unknown[] = [];
  const hourWrites: unknown[] = [];
  mount({
    "GET /scheduling/profile": () =>
      jsonResponse({ ...bookingProfile, calendar_id: "work" }),
    "PUT /scheduling/profile": (body) => {
      profileWrites.push(body);
      return jsonResponse(body);
    },
    "PUT /me/working-hours": (body) => {
      hourWrites.push(body);
      return jsonResponse({ chosen: true, working_hours: body });
    },
  });
  const title = await screen.findByLabelText("Meeting title");
  const saturday = screen.getByRole("checkbox", { name: "Saturday" });
  await user.click(saturday);
  await user.type(title, "?");
  await user.click(screen.getByRole("button", { name: "Discard" }));
  expect(saturday).toHaveProperty("checked", false);
  expect(title).toHaveProperty("value", bookingProfile.title);
  expect(screen.queryByRole("button", { name: "Save settings" })).toBeNull();
  // Monday re-ticked lands after Friday in the draft; the save is still whole.
  const monday = screen.getByRole("checkbox", { name: "Monday" });
  await user.click(monday);
  await user.click(monday);
  await user.click(saturday);
  await user.type(title, "!");
  await user.click(screen.getByRole("button", { name: "Save settings" }));
  await waitFor(() => expect(profileWrites).toHaveLength(1));
  expect(hourWrites).toEqual([
    expect.objectContaining({ days: [1, 2, 3, 4, 5, 6] }),
  ]);
  expect(profileWrites[0]).toMatchObject({ title: `${bookingProfile.title}!` });
  await waitFor(() =>
    expect(screen.queryByRole("button", { name: "Save settings" })).toBeNull(),
  );
});

it("lists what is left to set up until the booking link is on", async () => {
  mount();
  await waitFor(() =>
    expect(screen.getByText(/Finish setting up booking/).textContent).toContain(
      "2 of 3 done",
    ),
  );
  expect(screen.getByText("Your booking link turned on")).toBeTruthy();
});

it("offers the booking link for a quick copy at the top of the page", async () => {
  const user = userEvent.setup();
  const copy = stubClipboard("accepts");
  mount();
  const quick = await screen.findByRole("button", {
    name: "Copy booking link",
  });
  const calendar = screen.getByRole("heading", { name: "Calendar" });
  expect(
    quick.compareDocumentPosition(calendar) & Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBeTruthy();
  expect(screen.getByText(bookingProfile.public_url ?? "")).toBeTruthy();
  await user.click(quick);
  expect(copy.written).toEqual([bookingProfile.public_url]);
});

it("keeps hours the server saved when the profile write after them fails", async () => {
  const user = userEvent.setup();
  mount({
    "GET /scheduling/profile": () =>
      jsonResponse({ ...bookingProfile, calendar_id: "work" }),
    "PUT /scheduling/profile": () =>
      jsonResponse({ title: "Unavailable", status: 503 }, 503),
    "PUT /me/working-hours": (body) =>
      jsonResponse({ chosen: true, working_hours: body }),
  });
  await screen.findByLabelText("Meeting title");
  const saturday = screen.getByRole("checkbox", { name: "Saturday" });
  await user.click(saturday);
  await user.type(screen.getByLabelText("Meeting title"), "!");
  await user.click(screen.getByRole("button", { name: "Save settings" }));
  expect(await screen.findByRole("alert")).toBeTruthy();
  await user.click(screen.getByRole("button", { name: "Discard" }));
  expect(saturday).toHaveProperty("checked", true);
  expect(screen.queryByRole("button", { name: "Save settings" })).toBeNull();
});

it("says the booking page has no company to show rather than leaving the fact blank", async () => {
  mount({
    "GET /scheduling/profile": () =>
      jsonResponse({ ...bookingProfile, company_name: "", logo_url: "" }),
  });
  const term = await screen.findByText("Company name and logo");
  expect(term.nextElementSibling?.textContent).toContain("Not set");
});

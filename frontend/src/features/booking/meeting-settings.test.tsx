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
import { PageSaveBar, SavePageProvider } from "../../design-system/savepage";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import {
  bookingConnection,
  bookingHours,
  bookingProfile,
} from "../../testing/book.testkit";
import { locationDouble } from "../../testing/locationdouble";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "../../testing/story-utils";
import { MeetingSettings } from "./meeting-settings";

type Profile = components["schemas"]["SchedulingProfile"];
type User = ReturnType<typeof userEvent.setup>;

const calendars = [
  { id: "work", name: "Work calendar", primary: true, writable: true },
  {
    id: "personal",
    name: "Personal calendar",
    primary: false,
    writable: false,
  },
];

// A server that keeps what it is sent, so every card's GET-latest-then-PUT reads
// what the card before it wrote. A test can change it underneath.
function mount(profile: Partial<Profile> = {}, overrides: RouteMap = {}) {
  const start: Profile = {
    ...bookingProfile,
    provider: "",
    enabled: false,
    ...profile,
  };
  const server = {
    profile: start,
    profileWrites: [] as Profile[],
    hourWrites: [] as unknown[],
  };
  installFetchStub({
    "GET /me": meRoute({ activity: ["create"] }),
    "GET /me/working-hours": () => jsonResponse(bookingHours),
    "GET /connectors": () => jsonResponse({ data: [bookingConnection] }),
    "GET /scheduling/profile": () => jsonResponse(server.profile),
    "GET /scheduling/calendars": () => jsonResponse(calendars),
    "PUT /scheduling/profile": (body) => {
      server.profile = body as Profile;
      server.profileWrites.push(server.profile);
      return jsonResponse(body);
    },
    "PUT /me/working-hours": (body) => {
      server.hourWrites.push(body);
      return jsonResponse({ chosen: true, working_hours: body });
    },
    ...overrides,
  });
  render(
    <StoryProviders>
      <ToastProvider>
        <SavePageProvider>
          <MeetingSettings />
          <PageSaveBar />
        </SavePageProvider>
        <ToastRegion />
      </ToastProvider>
    </StoryProviders>,
  );
  return server;
}

async function pick(user: User, combobox: string, option: string) {
  await user.click(screen.getByRole("combobox", { name: combobox }));
  await user.click(screen.getByRole("option", { name: option }));
}

// The rule rows render once the hours answer; resolving the name first keeps
// each pick from racing that answer.
async function ruleName(name: string) {
  await screen.findByRole("combobox", { name });
  return name;
}

// The page's one save bar, drawn only while some card holds a draft.
function saveBar() {
  return screen.queryByRole("region", { name: "Unsaved changes" });
}

function barSave() {
  const bar = saveBar();
  if (bar === null) throw new Error("the page has no save bar");
  return within(bar).getByRole("button", { name: "Save" });
}

async function enabledSave() {
  // Re-read each time: the held Save and the live one are not the same node.
  return waitFor(() => {
    const button = barSave();
    expect(button).toHaveProperty("disabled", false);
    return button;
  });
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

it("uses the only connected calendar, saves it with the calendars that block time, then takes bookings", async () => {
  const user = userEvent.setup();
  const server = mount();
  const accept = await screen.findByRole("switch", { name: "Accept bookings" });
  expect(accept).toHaveProperty("disabled", true);
  expect(
    screen.getByText(
      "Available once a calendar for bookings is saved under Calendar.",
    ),
  ).toBeTruthy();
  const save = await enabledSave();
  expect(screen.queryByRole("combobox", { name: "Calendar provider" })).toBe(
    null,
  );
  await user.click(
    screen.getByRole("combobox", { name: "Also count as busy" }),
  );
  expect(screen.queryByRole("option", { name: "Work calendar" })).toBeNull();
  await user.click(screen.getByRole("option", { name: "Personal calendar" }));
  await user.keyboard("{Escape}");
  await user.click(save);
  await waitFor(() => expect(server.profileWrites).toHaveLength(1));
  expect(server.profileWrites[0]).toMatchObject({
    provider: "gcal",
    calendar_id: "work",
    blocking_calendars: ["personal"],
    enabled: false,
  });
  await waitFor(() => {
    expect(saveBar()).toBeNull();
    expect(screen.getByText("Saved")).toBeTruthy();
  });
  expect(document.activeElement?.textContent).toBe("Calendar");
  await waitFor(() => expect(accept).toHaveProperty("disabled", false));
  await user.click(accept);
  await waitFor(() => expect(server.profileWrites).toHaveLength(2));
  expect(server.profileWrites[1]).toMatchObject({
    host_name: bookingProfile.host_name,
    provider: "gcal",
    calendar_id: "work",
    enabled: true,
  });
});

it("offers a provider choice only when both calendars are connected", async () => {
  const user = userEvent.setup();
  mount(
    { provider: "gcal" },
    {
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
    },
  );
  await user.click(
    await screen.findByRole("combobox", { name: "Calendar provider" }),
  );
  expect(screen.getByRole("option", { name: "Google Calendar" })).toBeTruthy();
  expect(screen.getByRole("option", { name: "Outlook Calendar" })).toBeTruthy();
  expect(screen.queryByRole("option", { name: "Gmail" })).toBeNull();
  expect(saveBar()).toBeNull();
  await user.click(screen.getByRole("option", { name: "Outlook Calendar" }));
  await enabledSave();
});

it("does not offer a disconnected provider or keep its old calendar ids", async () => {
  const user = userEvent.setup();
  const server = mount(
    {
      provider: "graphcal",
      calendar_id: "old-outlook",
      blocking_calendars: ["old-private"],
    },
    {
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
    },
  );
  await user.click(await enabledSave());
  await waitFor(() => expect(server.profileWrites).toHaveLength(1));
  expect(server.profileWrites[0]).toMatchObject({
    provider: "gcal",
    calendar_id: "work",
    blocking_calendars: [],
  });
  expect(screen.queryByRole("combobox", { name: "Calendar provider" })).toBe(
    null,
  );
});

it("states a read-only calendar's saved choices and reconnects it back to Meetings", async () => {
  const user = userEvent.setup();
  const connects: unknown[] = [];
  mount(
    { provider: "gcal" },
    {
      "GET /connectors": () =>
        jsonResponse({
          data: [
            {
              ...bookingConnection,
              scopes: ["https://www.googleapis.com/auth/calendar.readonly"],
            },
          ],
        }),
      "POST /connectors/gcal/connect": (body) => {
        connects.push(body);
        return jsonResponse({ authorize_url: "https://accounts.google/x" });
      },
    },
  );
  expect(
    (await screen.findAllByText("Needs attention")).length,
  ).toBeGreaterThan(0);
  expect(screen.getByText(/connected for reading events/)).toBeTruthy();
  expect(screen.getAllByText(/Kept as saved/)).toHaveLength(2);
  expect(
    screen.queryByRole("combobox", { name: "Add booked meetings to" }),
  ).toBe(null);
  expect(
    screen.getByText(/Available once your calendar is reconnected/),
  ).toBeTruthy();
  const assign = vi.fn();
  vi.stubGlobal("location", locationDouble({ assign }));
  await user.click(screen.getByRole("button", { name: "Reconnect" }));
  await waitFor(() => expect(assign).toHaveBeenCalled());
  expect(connects).toEqual([
    expect.objectContaining({ return_to: "meetings" }),
  ]);
});

it("explains a missing saved calendar and saves only once another is chosen", async () => {
  const user = userEvent.setup();
  const server = mount({ provider: "gcal", calendar_id: "deleted" });
  expect(
    await screen.findByText(/saved event calendar is unavailable/),
  ).toBeTruthy();
  expect(saveBar()).toBeNull();
  await pick(user, "Add booked meetings to", "Work calendar");
  await user.click(await enabledSave());
  await waitFor(() => expect(server.profileWrites).toHaveLength(1));
  expect(server.profileWrites[0]).toMatchObject({ calendar_id: "work" });
});

it("says bookings are live with nothing landing in an expired calendar, and pauses them", async () => {
  const user = userEvent.setup();
  const server = mount(
    { provider: "gcal", enabled: true },
    {
      "GET /connectors": () =>
        jsonResponse({
          data: [{ ...bookingConnection, status: "reauth_required" }],
        }),
    },
  );
  expect(
    await screen.findByText("Guests can book, but no invitations go out"),
  ).toBeTruthy();
  expect(
    screen.getByText(/Your Google Calendar connection has expired/),
  ).toBeTruthy();
  expect(screen.getByText(/The connection has expired/)).toBeTruthy();
  expect(screen.getAllByText("Needs attention").length).toBeGreaterThan(0);
  expect(
    await screen.findByText(
      /You can save this while the calendar is being reconnected/,
    ),
  ).toBeTruthy();
  await user.click(screen.getByRole("button", { name: "Pause bookings" }));
  await waitFor(() => expect(server.profileWrites).toHaveLength(1));
  expect(server.profileWrites[0]).toMatchObject({ enabled: false });
});

it("writes only the changed rule over another tab's latest profile", async () => {
  const user = userEvent.setup();
  const server = mount({ provider: "gcal", enabled: true });
  await pick(user, await ruleName("Break between meetings"), "15 minutes");
  server.profile = {
    ...server.profile,
    enabled: false,
    title: "Renamed in another tab",
    horizon_days: 90,
  };
  await user.click(await enabledSave());
  await waitFor(() => expect(server.profileWrites).toHaveLength(1));
  expect(server.profileWrites[0]).toMatchObject({
    buffer_minutes: 15,
    enabled: false,
    title: "Renamed in another tab",
    horizon_days: 90,
  });
  expect(server.hourWrites).toEqual([]);
});

it("saves a paused page's rules with no calendar connected", async () => {
  const user = userEvent.setup();
  const server = mount(
    {},
    { "GET /connectors": () => jsonResponse({ data: [] }) },
  );
  await pick(user, await ruleName("Minimum notice"), "1 day");
  await user.click(await enabledSave());
  await waitFor(() => expect(server.profileWrites).toHaveLength(1));
  expect(server.profileWrites[0]).toMatchObject({
    provider: "",
    notice_minutes: 1440,
    enabled: false,
  });
});

it("saves never-chosen hours as offered, without a change first", async () => {
  const user = userEvent.setup();
  const server = mount(
    { provider: "gcal" },
    {
      "GET /me/working-hours": () =>
        jsonResponse({ ...bookingHours, chosen: false }),
    },
  );
  await user.click(await enabledSave());
  await waitFor(() => expect(server.hourWrites).toHaveLength(1));
  expect(server.hourWrites[0]).toMatchObject({
    start_time: "09:00",
    end_time: "17:00",
  });
  expect(server.profileWrites).toEqual([]);
});

it.each([
  ["90", 90],
  ["10080", 10080],
])("saves an Other notice of %s minutes", async (typed, minutes) => {
  const user = userEvent.setup();
  const server = mount({ provider: "gcal" });
  await pick(user, await ruleName("Minimum notice"), "Other");
  const field = screen.getByRole("spinbutton", { name: "Minutes" });
  await user.clear(field);
  await user.type(field, typed);
  await user.click(await enabledSave());
  await waitFor(() => expect(server.profileWrites).toHaveLength(1));
  expect(server.profileWrites[0]).toMatchObject({ notice_minutes: minutes });
});

it.each(["10081", "1.5", ""])(
  "refuses an Other notice of %j minutes before anything is sent",
  async (typed) => {
    const user = userEvent.setup();
    const server = mount({ provider: "gcal" });
    await pick(user, await ruleName("Minimum notice"), "Other");
    const field = screen.getByRole("spinbutton", { name: "Minutes" });
    await user.clear(field);
    if (typed) await user.type(field, typed);
    expect(
      await screen.findByText("Enter a whole number from 0 to 10,080."),
    ).toBeTruthy();
    await user.click(barSave());
    expect(server.profileWrites).toEqual([]);
  },
);

it("keeps an off-preset value as its own choice and leaves it alone on other saves", async () => {
  const user = userEvent.setup();
  const server = mount({ provider: "gcal", notice_minutes: 50 });
  const notice = await screen.findByRole("combobox", {
    name: "Minimum notice",
  });
  expect(notice.textContent).toContain("50 minutes");
  await pick(user, "How far ahead", "60 days");
  await user.click(await enabledSave());
  await waitFor(() => expect(server.profileWrites).toHaveLength(1));
  expect(server.profileWrites[0]).toMatchObject({
    notice_minutes: 50,
    horizon_days: 60,
  });
});

it("saves hours and rules from one Save, with the days in week order", async () => {
  const user = userEvent.setup();
  const server = mount({ provider: "gcal" });
  const monday = await screen.findByRole("checkbox", { name: "Monday" });
  // Monday re-ticked would land after Friday in a naive draft.
  await user.click(monday);
  await user.click(monday);
  await user.click(screen.getByRole("checkbox", { name: "Saturday" }));
  await pick(user, "Break between meetings", "15 minutes");
  await user.click(await enabledSave());
  await waitFor(() => expect(server.profileWrites).toHaveLength(1));
  expect(server.hourWrites).toEqual([
    expect.objectContaining({ days: [1, 2, 3, 4, 5, 6] }),
  ]);
  expect(server.profileWrites[0]).toMatchObject({ buffer_minutes: 15 });
  await waitFor(() => expect(saveBar()).toBeNull());
});

it("keeps hours the server saved when the rules write after them fails", async () => {
  const user = userEvent.setup();
  const server = mount(
    { provider: "gcal" },
    {
      "PUT /scheduling/profile": () =>
        jsonResponse({ title: "Unavailable", status: 503 }, 503),
    },
  );
  const saturday = await screen.findByRole("checkbox", { name: "Saturday" });
  await user.click(saturday);
  await pick(user, "Break between meetings", "15 minutes");
  await user.click(await enabledSave());
  expect(
    await within(barSave().closest("section") ?? document.body).findByRole(
      "alert",
    ),
  ).toBeTruthy();
  expect(saturday).toHaveProperty("checked", true);
  await user.click(barSave());
  await waitFor(() => expect(screen.getAllByRole("alert")).toHaveLength(1));
  expect(server.hourWrites).toHaveLength(1);
});

it("keeps an availability draft while bookings are switched on and off", async () => {
  const user = userEvent.setup();
  mount({ provider: "gcal" });
  const saturday = await screen.findByRole("checkbox", { name: "Saturday" });
  await user.click(saturday);
  const accept = screen.getByRole("switch", { name: "Accept bookings" });
  await waitFor(() => expect(accept).toHaveProperty("disabled", false));
  await user.click(accept);
  await waitFor(() => expect(accept.getAttribute("aria-checked")).toBe("true"));
  await user.click(accept);
  await waitFor(() =>
    expect(accept.getAttribute("aria-checked")).toBe("false"),
  );
  expect(saturday).toHaveProperty("checked", true);
});

it("orders the cards and copies and previews the booking link from its row", async () => {
  const user = userEvent.setup();
  const copy = stubClipboard("accepts");
  mount({ provider: "gcal" });
  expect(await screen.findByText(bookingProfile.public_url ?? "")).toBeTruthy();
  const headings = screen
    .getAllByRole("heading")
    .map((heading) => heading.textContent);
  const order = [
    "Booking links",
    "Calendar",
    "When people can book",
    "What guests see",
  ].map((title) => headings.indexOf(title));
  expect(order).toEqual([...order].sort((a, b) => a - b));
  expect(order).not.toContain(-1);
  expect(headings).not.toContain("Connect a calendar to take bookings");
  expect(screen.getAllByText("Google Meet").length).toBeGreaterThan(0);
  await user.click(screen.getByRole("button", { name: "Copy link" }));
  expect(copy.written).toEqual([bookingProfile.public_url]);
  await user.click(
    screen.getAllByRole("button", { name: "Preview public page" })[0],
  );
  expect(window.location.hash).toBe("#/book/preview");
});

it("names Microsoft Teams as an Outlook calendar's video app", async () => {
  mount(
    { provider: "graphcal" },
    {
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
    },
  );
  expect(
    (await screen.findAllByText("Microsoft Teams")).length,
  ).toBeGreaterThan(0);
  expect(screen.queryByText("Google Meet")).toBeNull();
});

it("creates the booking link when none exists yet", async () => {
  const user = userEvent.setup();
  const server = mount({ slug: undefined, public_url: undefined });
  expect(await screen.findByText("No booking link yet.")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Copy link" })).toBeNull();
  await user.click(screen.getByRole("button", { name: "Create booking link" }));
  await waitFor(() => expect(server.profileWrites).toHaveLength(1));
  expect(server.profileWrites[0]).toMatchObject({ enabled: false });
});

it("says a created link has no public address yet rather than offering a copy", async () => {
  mount({ slug: "ada", public_url: undefined });
  expect(await screen.findByText(/Set a public address/)).toBeTruthy();
  expect(screen.getByRole("button", { name: "Copy link" })).toHaveProperty(
    "disabled",
    true,
  );
  expect(screen.queryByText("No booking link yet.")).toBeNull();
});

it("leads with connecting a calendar when none is connected, and connects back to Meetings", async () => {
  const user = userEvent.setup();
  const connects: unknown[] = [];
  mount(
    {},
    {
      "GET /connectors": () =>
        jsonResponse({
          data: [],
          providers: [{ provider: "gmail", reason: "ready" }],
        }),
      "POST /connectors/gcal/connect": (body) => {
        connects.push(body);
        return jsonResponse({ authorize_url: "https://accounts.google/x" });
      },
    },
  );
  const lead = await screen.findByRole("heading", {
    name: "Connect a calendar to take bookings",
  });
  expect(screen.getAllByRole("heading")[0]).toBe(lead);
  expect(screen.queryByRole("heading", { name: "Calendar" })).toBeNull();
  expect(await screen.findByText("Your hours are saved.")).toBeTruthy();
  expect(screen.getByText(/Your link already exists/)).toBeTruthy();
  expect(
    screen.getByText(/Available once a calendar is connected/),
  ).toBeTruthy();
  const assign = vi.fn();
  vi.stubGlobal("location", locationDouble({ assign }));
  await user.click(screen.getByRole("button", { name: "Connect calendar" }));
  await waitFor(() =>
    expect(assign).toHaveBeenCalledWith("https://accounts.google/x"),
  );
  expect(connects).toEqual([
    expect.objectContaining({ return_to: "meetings" }),
  ]);
});

it("reads the company and the host's name from where they are kept", async () => {
  const user = userEvent.setup();
  const server = mount({ logo_url: "" });
  expect(await screen.findByText("Company name and logo")).toBeTruthy();
  expect(screen.getByText(/No logo is uploaded yet/)).toBeTruthy();
  expect(
    screen.getByText(`${bookingProfile.host_name}, from your account.`),
  ).toBeTruthy();
  expect(screen.queryByRole("textbox", { name: "Your name" })).toBeNull();
  await user.click(screen.getByRole("button", { name: "Add logo" }));
  expect(window.location.hash).toBe("#/settings/company");
  await user.click(screen.getByRole("button", { name: "Change" }));
  expect(window.location.hash).toBe("#/settings/account");
  expect(server.profileWrites).toEqual([]);
});

it("says the booking page has no company to show rather than leaving it blank", async () => {
  mount({ company_name: "", logo_url: "" });
  expect(
    await screen.findByText(
      "Not set in the company profile yet, so guests see your name only.",
    ),
  ).toBeTruthy();
  expect(screen.queryByText(/No logo is uploaded yet/)).toBeNull();
});

it("turns the reminder email on only with a mailbox to send it from", async () => {
  const user = userEvent.setup();
  const server = mount(
    { provider: "gcal" },
    {
      "GET /connectors": () =>
        jsonResponse({
          data: [
            bookingConnection,
            { ...bookingConnection, id: "mail", provider: "gmail" },
          ],
        }),
    },
  );
  const reminder = await screen.findByRole("switch", {
    name: "Reminder email",
  });
  await waitFor(() => expect(reminder).toHaveProperty("disabled", false));
  expect(screen.getByText(/sent from ada@example.test/)).toBeTruthy();
  await user.click(reminder);
  await waitFor(() => expect(server.profileWrites).toHaveLength(1));
  expect(server.profileWrites[0]).toMatchObject({ email_reminder: true });
});

it("gives a member without the booking grant their hours alone", async () => {
  let profileReads = 0;
  mount(
    {},
    {
      "GET /me": meRoute({}),
      "GET /scheduling/profile": () => {
        profileReads += 1;
        return jsonResponse(bookingProfile);
      },
    },
  );
  expect(
    await screen.findByText(
      "Booking meetings needs permission from an administrator.",
    ),
  ).toBeTruthy();
  expect(
    await screen.findByRole("checkbox", { name: "Saturday" }),
  ).toBeTruthy();
  expect(screen.getByText("When people can book")).toBeTruthy();
  expect(screen.queryByText("Booking rules")).toBeNull();
  expect(screen.queryByText("Booking links")).toBeNull();
  expect(profileReads).toBe(0);
});

it("reports a calendar connected from this page on its landing", async () => {
  window.location.hash = "#/settings/meetings/ok/gcal";
  mount({ provider: "gcal" });
  const dismiss = await screen.findByRole("button", { name: "Dismiss" });
  await userEvent.setup().click(dismiss);
  expect(screen.queryByRole("button", { name: "Dismiss" })).toBeNull();
});

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { RecordZoneProvider } from "../app/recordzone";
import { stubClipboard } from "../design-system/clipboard-testing";
import { formatDayMonth } from "../format/format";
import { formatTimeRange } from "../format/meetingtime";
import { viewerZone } from "../format/timezone";
import { LocaleProvider, translate } from "../i18n";
import { bookingProfile, bookingProposals, bookingSlots } from "./book.testkit";
import { proposalEmailBody } from "./booking-proposal-message";
import { ContactMeetingsTab } from "./contactmeetings";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
} from "./story-utils";

type Proposal = components["schemas"]["MeetingProposal"];

// The composer is the boundary here: what this tab owes it is the message and
// the address it opens on, which is what the stand-in shows.
vi.mock("./compose", () => ({
  ComposeModal: ({
    initialMessage,
    recordAddress,
  }: Readonly<{
    initialMessage?: { subject: string; body: string };
    recordAddress?: string;
  }>) => (
    <div role="dialog" aria-label="Compose">
      <p>{recordAddress}</p>
      <p>{initialMessage?.subject}</p>
      <pre>{initialMessage?.body}</pre>
    </div>
  ),
}));

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

const view: components["schemas"]["Contact360"] = {
  as_of: "2026-09-30T09:00:00Z",
  sections_omitted: [],
  contact: {
    id: "p-1",
    full_name: "Dana Buyer",
    first_name: "Dana",
    primary_email: "dana@buyer.example",
    source: "manual",
    captured_by: "human:u-1",
    created_at: "2026-06-01T08:00:00Z",
    updated_at: "2026-08-01T08:00:00Z",
  },
  activities: { data: [], page: { has_more: false } },
};

const [proposed, personal] = bookingProposals;

function mount(
  proposals: () => Proposal[],
  routes: RouteMap = {},
  allow: Parameters<typeof meRoute>[0] = { activity: ["create"] },
  shown: components["schemas"]["Contact360"] = view,
) {
  installFetchStub({
    "GET /me": meRoute(allow, { seat: "full" }),
    "GET /scheduling/proposals": () => jsonResponse({ data: proposals() }),
    ...routes,
  });
  const requests = vi.spyOn(globalThis, "fetch");
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <RecordZoneProvider zone="UTC">
          <ContactMeetingsTab view={shown} />
        </RecordZoneProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { client, requests };
}

it("lists the invitations still waiting on the contact, between upcoming and held", async () => {
  const { requests } = mount(() => [proposed, personal]);
  expect(await screen.findByText("Project discovery")).toBeTruthy();
  expect(
    screen.getAllByRole("heading").map((heading) => heading.textContent),
  ).toEqual(["Upcoming", "Waiting on Dana", "Held"]);
  // A link that offers no times says what it is instead.
  const intro = screen.getByText("Intro call").closest("article");
  expect(intro?.textContent).toContain("Personal link");
  const zone = viewerZone();
  // Each offered time is its own label, so a reader scans them as a set.
  for (const slot of bookingSlots)
    expect(
      screen.getByText(
        `${formatDayMonth(slot.start, "en", zone)} · ${formatTimeRange(slot.start, slot.end, "en", zone)}`,
      ),
    ).toBeTruthy();
  expect(
    screen.getByText(
      `Sent ${formatDayMonth(proposed.created_at, "en", zone)} · expires ${formatDayMonth(proposed.expires_at, "en", zone)}`,
    ),
  ).toBeTruthy();
  // The list is asked for this contact, not for every open proposal.
  const asked = requests.mock.calls
    .map(([input, init]) =>
      input instanceof Request ? input : new Request(input, init),
    )
    .find((request) => request.url.includes("/scheduling/proposals"));
  expect(new URL(asked?.url ?? "").searchParams.get("contact_id")).toBe("p-1");
});

it("hides the waiting section when nothing is waiting", async () => {
  const { client } = mount(() => []);
  await waitFor(() =>
    expect(client.getQueryState(["meeting-proposals", "p-1"])?.status).toBe(
      "success",
    ),
  );
  expect(screen.getByText("No upcoming meetings.")).toBeTruthy();
  expect(screen.queryByRole("heading", { name: /Waiting on/ })).toBeNull();
});

it("withdraws an invitation by archiving it, then reads the list and the timeline again", async () => {
  const user = userEvent.setup();
  let open = [proposed, personal];
  const archived: string[] = [];
  const { client } = mount(() => open, {
    [`DELETE /activities/${proposed.id}`]: () => {
      archived.push(proposed.id);
      open = [personal];
      return jsonResponse({ id: proposed.id });
    },
  });
  const row = (await screen.findByText("Project discovery")).closest("article");
  if (!row) throw new Error("Each proposal is drawn as its own card");
  client.setQueryData(["contact360", "p-1"], view);
  // The rare verb is folded into the row's own menu.
  await user.click(
    within(row).getByRole("button", {
      name: "More actions for Project discovery",
    }),
  );
  await user.click(await screen.findByRole("button", { name: "Withdraw" }));
  const dialog = await screen.findByRole("dialog");
  expect(within(dialog).getByText("Withdraw this invitation?")).toBeTruthy();
  await user.click(
    within(dialog).getByRole("button", { name: "Withdraw invitation" }),
  );
  await waitFor(() => expect(archived).toEqual([proposed.id]));
  await waitFor(() =>
    expect(screen.queryByText("Project discovery")).toBeNull(),
  );
  expect(screen.getByText("Intro call")).toBeTruthy();
  // The Withdraw button that opened the dialog went with its card.
  await waitFor(() =>
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "Book a meeting" }),
    ),
  );
  // The archived proposal still sits on the contact's timeline until it is read again.
  expect(client.getQueryState(["contact360", "p-1"])?.isInvalidated).toBe(true);
});

it("returns focus to the Withdraw button when the withdrawal is cancelled", async () => {
  const user = userEvent.setup();
  mount(() => [proposed]);
  const row = (await screen.findByText("Project discovery")).closest("article");
  if (!row) throw new Error("Each proposal is drawn as its own card");
  await user.click(
    within(row).getByRole("button", {
      name: "More actions for Project discovery",
    }),
  );
  const opener = await screen.findByRole("button", { name: "Withdraw" });
  await user.click(opener);
  const dialog = await screen.findByRole("dialog");
  await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
  await waitFor(() => expect(document.activeElement).toBe(opener));
});

it("resends an invitation through the composer with the email it was sent with", async () => {
  const user = userEvent.setup();
  mount(() => [proposed]);
  const row = (await screen.findByText("Project discovery")).closest("article");
  if (!row) throw new Error("Each proposal is drawn as its own card");
  await user.click(within(row).getByRole("button", { name: "Resend" }));
  const composer = await screen.findByRole("dialog", { name: "Compose" });
  expect(within(composer).getByText("dana@buyer.example")).toBeTruthy();
  expect(within(composer).getByText("Project discovery")).toBeTruthy();
  // The same words the proposal was first sent with; the list carries no
  // description, so that paragraph is the one left out.
  const body = proposalEmailBody(
    (key, params) => translate("en", key, params),
    { description: "", options: proposed.options, url: proposed.url },
    "en",
    viewerZone(),
  );
  expect(composer.querySelector("pre")?.textContent).toBe(body);
  expect(body).toContain(proposed.url);
});

it("offers the host's booking link while the page takes bookings", async () => {
  mount(() => [], {
    "GET /scheduling/profile": () => jsonResponse(bookingProfile),
  });
  expect(
    await screen.findByRole("button", { name: "Copy my booking link" }),
  ).toBeTruthy();
});

it("lets only the link copied last read Copied, since the clipboard holds one", async () => {
  const user = userEvent.setup();
  const clipboard = stubClipboard("accepts");
  mount(() => [proposed], {
    "GET /scheduling/profile": () => jsonResponse(bookingProfile),
  });
  await user.click(
    await screen.findByRole("button", { name: "Copy my booking link" }),
  );
  await screen.findByRole("button", { name: "Copied" });
  await user.click(screen.getByRole("button", { name: "Copy link" }));

  await waitFor(() =>
    expect(clipboard.written).toEqual([
      bookingProfile.public_url,
      proposed.url,
    ]),
  );
  expect(screen.getAllByRole("button", { name: "Copied" })).toHaveLength(1);
  expect(
    screen.getByRole("button", { name: "Copy my booking link" }),
  ).toBeTruthy();
});

it("keeps a paused booking link to itself", async () => {
  const { client } = mount(() => [], {
    "GET /scheduling/profile": () =>
      jsonResponse({ ...bookingProfile, enabled: false }),
  });
  await waitFor(() =>
    expect(client.getQueryState(["scheduling-profile"])?.status).toBe(
      "success",
    ),
  );
  expect(
    screen.queryByRole("button", { name: "Copy my booking link" }),
  ).toBeNull();
});

it("does not ask for proposals when the reader may not book", async () => {
  const asked = vi.fn(() => [proposed]);
  mount(asked, {}, {});
  expect(
    await screen.findByText(
      "An administrator must grant permission to book meetings.",
    ),
  ).toBeTruthy();
  expect(asked).not.toHaveBeenCalled();
  expect(screen.queryByRole("heading", { name: /Waiting on/ })).toBeNull();
});

type Activity = components["schemas"]["Activity"];

function meeting(
  row: Pick<Activity, "id" | "occurred_at"> & Partial<Activity>,
) {
  return {
    kind: "meeting",
    subject: "Weekly call",
    duration_seconds: 1800,
    is_done: false,
    source: "manual",
    captured_by: "human:u-1",
    content_state: "available",
    created_at: "2026-09-01T08:00:00Z",
    updated_at: "2026-09-01T08:00:00Z",
    ...row,
  } satisfies Activity;
}

function meetingsPage(rows: Activity[], nextCursor?: string) {
  return jsonResponse({
    data: rows,
    page: { has_more: Boolean(nextCursor), next_cursor: nextCursor ?? null },
  });
}

const booked = meeting({
  id: "0198f011-cccc-7000-8000-000000000001",
  occurred_at: "2026-10-02T09:00:00Z",
  meeting_status: "booked",
  invitation_status: "confirmed",
});

const withBooking: components["schemas"]["Contact360"] = {
  ...view,
  next_meeting: {
    activity_id: booked.id,
    starts_at: booked.occurred_at,
    subject: "Weekly call",
    participants: [{ contact_id: "p-1", full_name: "Dana Buyer" }],
  },
};

it("offers the next meeting's join link and its own page, from the invitation the calendar made", async () => {
  mount(
    () => [],
    {
      "GET /activities": () => meetingsPage([booked]),
      [`GET /scheduling/invitations/${booked.id}`]: () =>
        jsonResponse({
          id: booked.id,
          status: "confirmed",
          start: "2026-10-02T09:00:00Z",
          end: "2026-10-02T09:45:00Z",
          subject: "Weekly call",
          location: "",
          version: 1,
          provider: "gcal",
          video_call: true,
          video_url: "https://meet.google.com/abc-defg-hij",
        }),
    },
    undefined,
    withBooking,
  );
  const join = await screen.findByRole("link", { name: "Join Google Meet" });
  expect(join.getAttribute("href")).toBe(
    "https://meet.google.com/abc-defg-hij",
  );
  // The end comes from the invitation, which the calendar keeps current.
  expect(
    screen.getByText(
      formatTimeRange(
        "2026-10-02T09:00:00Z",
        "2026-10-02T09:45:00Z",
        "en",
        "UTC",
      ),
    ),
  ).toBeTruthy();
  expect(
    screen.getByRole("link", { name: "Weekly call" }).getAttribute("href"),
  ).toBe(`#/book/meeting-${booked.id}`);
});

it("asks for no invitation for a meeting that was not booked through Margince", async () => {
  const invitation = vi.fn(() => jsonResponse({}));
  mount(
    () => [],
    {
      "GET /activities": () =>
        meetingsPage([
          { ...booked, invitation_status: null },
          meeting({
            id: "m-held",
            subject: "Discovery call",
            occurred_at: "2026-09-24T08:00:00Z",
          }),
        ]),
      [`GET /scheduling/invitations/${booked.id}`]: invitation,
    },
    undefined,
    withBooking,
  );
  // The list has arrived, so the card knows how the meeting was booked.
  await screen.findByText("Discovery call");
  expect(screen.getByText("Weekly call")).toBeTruthy();
  expect(screen.queryByRole("link", { name: "Weekly call" })).toBeNull();
  expect(invitation).not.toHaveBeenCalled();
});

it("says what became of a meeting only where it is not the ordinary answer", async () => {
  mount(() => [], {
    "GET /activities": () =>
      meetingsPage([
        meeting({
          id: "m-held",
          subject: "Discovery call",
          occurred_at: "2026-09-24T08:00:00Z",
          meeting_status: "held",
        }),
        meeting({
          id: "m-missed",
          subject: "Intro call",
          occurred_at: "2026-09-17T08:00:00Z",
          meeting_status: "no_show",
        }),
      ]),
  });
  const held = await screen.findByRole("region", { name: "Held" });
  await within(held).findByText("Intro call");
  expect(within(held).getAllByText("No-show")).toHaveLength(1);
  expect(
    within(held).queryByText("Held", { selector: ".badge-label" }),
  ).toBeNull();
});

it("reads older meetings when the reader asks for them", async () => {
  const user = userEvent.setup();
  const pages = [
    meetingsPage(
      [
        meeting({
          id: "m-new",
          subject: "Review",
          occurred_at: "2026-09-24T08:00:00Z",
        }),
      ],
      "c-2",
    ),
    meetingsPage([
      meeting({
        id: "m-old",
        subject: "Kick-off",
        occurred_at: "2026-06-03T08:00:00Z",
      }),
    ]),
  ];
  let read = 0;
  const { requests } = mount(() => [], {
    "GET /activities": () => pages[Math.min(read++, pages.length - 1)],
  });
  await screen.findByText("Review");
  expect(screen.queryByText("Kick-off")).toBeNull();
  await user.click(screen.getByRole("button", { name: "Load more" }));
  expect(await screen.findByText("Kick-off")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Load more" })).toBeNull();
  // Both reads are this contact's meetings; the second continues from the
  // first page's cursor.
  const asked = requests.mock.calls
    .map(([input, init]) =>
      input instanceof Request ? input : new Request(input, init),
    )
    .map((request) => new URL(request.url))
    .filter((url) => url.pathname.endsWith("/activities"));
  expect(asked).toHaveLength(2);
  for (const url of asked) {
    expect(url.searchParams.get("entity_id")).toBe("p-1");
    expect(url.searchParams.get("kind")).toBe("meeting");
  }
  expect(asked[0].searchParams.get("cursor")).toBeNull();
  expect(asked[1].searchParams.get("cursor")).toBe("c-2");
});

it("does not ask for meetings the reader's role may not see", async () => {
  const listed = vi.fn(() => meetingsPage([]));
  mount(() => [], { "GET /activities": listed }, undefined, {
    ...view,
    sections_omitted: ["activities"],
  });
  const held = await screen.findByRole("region", { name: "Held" });
  expect(within(held).getByText("Hidden for your role")).toBeTruthy();
  expect(listed).not.toHaveBeenCalled();
});

it("says how far off each meeting ahead is, on the read's own clock", async () => {
  const soon = meeting({ id: "m-soon", occurred_at: "2026-10-01T23:00:00Z" });
  mount(
    () => [],
    {
      "GET /activities": () =>
        meetingsPage([
          meeting({
            id: "m-later",
            subject: "Rollout",
            occurred_at: "2026-10-09T09:00:00Z",
          }),
          meeting({
            id: "m-next",
            subject: "Review",
            occurred_at: "2026-10-02T09:00:00Z",
          }),
          soon,
        ]),
    },
    undefined,
    {
      ...view,
      // Late in the evening, so a meeting at 23:00 is still today and one the
      // next morning is tomorrow, whatever the clock of the machine reading it.
      as_of: "2026-10-01T22:30:00Z",
      next_meeting: {
        activity_id: soon.id,
        starts_at: soon.occurred_at,
        subject: "Weekly call",
      },
    },
  );
  const upcoming = await screen.findByRole("region", { name: "Upcoming" });
  await within(upcoming).findByText("Rollout");
  expect(within(upcoming).getByText("Today")).toBeTruthy();
  expect(within(upcoming).getByText("Tomorrow")).toBeTruthy();
  expect(within(upcoming).getByText("In 8 days")).toBeTruthy();
});

it("lists a meeting that was called off with what became of it, not as upcoming", async () => {
  mount(() => [], {
    "GET /activities": () =>
      meetingsPage([
        meeting({
          id: "m-off",
          subject: "Site visit",
          occurred_at: "2026-10-07T09:00:00Z",
          meeting_status: "canceled",
        }),
      ]),
  });
  const held = await screen.findByRole("region", { name: "Held" });
  await within(held).findByText("Site visit");
  expect(within(held).getByText("Canceled")).toBeTruthy();
  expect(
    within(screen.getByRole("region", { name: "Upcoming" })).queryByText(
      "Site visit",
    ),
  ).toBeNull();
});

it("reads the next meeting's own row when a busy calendar pushed it off the first page", async () => {
  const later = Array.from({ length: 3 }, (_, index) =>
    meeting({
      id: `m-later-${index}`,
      subject: `Later call ${index}`,
      occurred_at: `2026-10-1${index}T09:00:00Z`,
    }),
  );
  const row = vi.fn(() => jsonResponse(booked));
  mount(
    () => [],
    {
      "GET /activities": () => meetingsPage(later, "c-2"),
      [`GET /activities/${booked.id}`]: row,
      [`GET /scheduling/invitations/${booked.id}`]: () =>
        jsonResponse({
          id: booked.id,
          status: "confirmed",
          start: booked.occurred_at,
          end: "2026-10-02T09:30:00Z",
          subject: "Weekly call",
          location: "",
          version: 1,
          provider: "graphcal",
          video_call: true,
          video_url: "https://teams.microsoft.com/l/meetup-join/abc",
        }),
    },
    undefined,
    withBooking,
  );
  expect(
    await screen.findByRole("link", { name: "Join Microsoft Teams" }),
  ).toBeTruthy();
  expect(row).toHaveBeenCalledTimes(1);
});

it("does not call the held list empty, or count what is ahead, while a page is still unread", async () => {
  // A first page of meetings still ahead: the held ones, and perhaps more
  // ahead, are on the next page.
  mount(() => [], {
    "GET /activities": () =>
      meetingsPage(
        [
          meeting({
            id: "m-a",
            subject: "Planning",
            occurred_at: "2026-10-08T09:00:00Z",
          }),
          meeting({
            id: "m-b",
            subject: "Review",
            occurred_at: "2026-10-06T09:00:00Z",
          }),
        ],
        "c-2",
      ),
  });
  const upcoming = await screen.findByRole("region", { name: "Upcoming" });
  await within(upcoming).findByText("Planning");
  expect(within(upcoming).queryByText("2")).toBeNull();
  const held = screen.getByRole("region", { name: "Held" });
  expect(within(held).queryByText("No meetings logged.")).toBeNull();
  expect(within(held).getByRole("button", { name: "Load more" })).toBeTruthy();
});

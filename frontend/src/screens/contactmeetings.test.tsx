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
import { stubClipboard } from "../design-system/clipboard-testing";
import { formatDayMonth, formatTimeOfDay } from "../format/format";
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
        <ContactMeetingsTab view={view} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { client, requests };
}

it("lists the invitations still waiting on the contact, between upcoming and held", async () => {
  const { requests } = mount(() => [proposed, personal]);
  expect(
    await screen.findByText("Proposed 2 times · Project discovery"),
  ).toBeTruthy();
  expect(
    screen.getAllByRole("heading").map((heading) => heading.textContent),
  ).toEqual(["Upcoming", "Waiting on Dana", "Held"]);
  expect(screen.getByText("0 upcoming · 2 awaiting reply")).toBeTruthy();
  expect(screen.getByText("Personal link · Intro call")).toBeTruthy();
  const zone = viewerZone();
  const offered = bookingSlots
    .map(
      (slot) =>
        `${formatDayMonth(slot.start, "en", zone)} ${formatTimeOfDay(slot.start, "en", zone)}`,
    )
    .join(" · ");
  expect(screen.getByText(offered)).toBeTruthy();
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
  expect(screen.getByText("0 upcoming")).toBeTruthy();
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
  const row = (
    await screen.findByText("Proposed 2 times · Project discovery")
  ).closest("article");
  if (!row) throw new Error("Each proposal is drawn as its own card");
  client.setQueryData(["contact360", "p-1"], view);
  await user.click(within(row).getByRole("button", { name: "Withdraw" }));
  const dialog = await screen.findByRole("dialog");
  expect(within(dialog).getByText("Withdraw this invitation?")).toBeTruthy();
  await user.click(
    within(dialog).getByRole("button", { name: "Withdraw invitation" }),
  );
  await waitFor(() => expect(archived).toEqual([proposed.id]));
  await waitFor(() =>
    expect(
      screen.queryByText("Proposed 2 times · Project discovery"),
    ).toBeNull(),
  );
  expect(screen.getByText("Personal link · Intro call")).toBeTruthy();
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
  const row = (
    await screen.findByText("Proposed 2 times · Project discovery")
  ).closest("article");
  if (!row) throw new Error("Each proposal is drawn as its own card");
  const opener = within(row).getByRole("button", { name: "Withdraw" });
  await user.click(opener);
  const dialog = await screen.findByRole("dialog");
  await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
  await waitFor(() => expect(document.activeElement).toBe(opener));
});

it("resends an invitation through the composer with the email it was sent with", async () => {
  const user = userEvent.setup();
  mount(() => [proposed]);
  const row = (
    await screen.findByText("Proposed 2 times · Project discovery")
  ).closest("article");
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

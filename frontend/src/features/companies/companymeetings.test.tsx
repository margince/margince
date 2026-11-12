// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import {
  company,
  companyBackstop,
  jsonResponse as companyJson,
} from "../../testing/company.fixtures";
import { stubFetch } from "../../testing/company.testkit";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
} from "../../testing/story-utils";
import { CompanyScreen } from "./companies";
import { type Company360, view } from "./company360.testkit";
import { CompanyMeetingsTab } from "./companymeetings";

type Activity = components["schemas"]["Activity"];

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function meeting(
  row: Pick<Activity, "id" | "occurred_at" | "subject"> & Partial<Activity>,
) {
  return {
    occurred_precision: "instant",
    offers_email: false,
    kind: "meeting",
    duration_seconds: 1800,
    is_done: false,
    source: "manual",
    captured_by: "human:u-1",
    content_state: "available",
    created_at: "2026-05-01T08:00:00Z",
    updated_at: "2026-05-01T08:00:00Z",
    ...row,
  } satisfies Activity;
}

const kickoff = meeting({
  id: "0198f011-cccc-7000-8000-000000000001",
  occurred_at: "2026-06-03T09:00:00Z",
  subject: "Rollout kickoff",
  meeting_status: "booked",
});
const review = meeting({
  id: "0198f011-cccc-7000-8000-000000000002",
  occurred_at: "2026-05-20T09:00:00Z",
  subject: "Quarterly review",
});

const booked: Company360 = view({
  next_meeting: {
    activity_id: kickoff.id,
    starts_at: kickoff.occurred_at,
    occurred_precision: "instant",
    subject: "Rollout kickoff",
    participants: [{ contact_id: "p-1", display_name: "Dana Buyer" }],
  },
});

function mount(shown: Company360 | undefined, routes: RouteMap = {}) {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read"] }, { seat: "full" }),
    "GET /activities": () =>
      jsonResponse({
        data: [kickoff, review],
        page: { has_more: false, next_cursor: null },
      }),
    ...routes,
  });
  const requests = vi.spyOn(globalThis, "fetch");
  const onBriefMeeting = vi.fn();
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <CompanyMeetingsTab
          companyId="o-1"
          view={shown}
          loading={false}
          onBriefMeeting={onBriefMeeting}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  // The client sends a Request, not a bare URL.
  const asked = () =>
    requests.mock.calls.map(
      ([input]) =>
        new URL(input instanceof Request ? input.url : String(input)),
    );
  return { asked, onBriefMeeting };
}

it("reads the account's own meetings and leads with the next one and who is in it", async () => {
  const { asked, onBriefMeeting } = mount(booked);
  expect(await screen.findByText("Quarterly review")).toBeTruthy();
  const read = asked().find((url) => url.pathname.endsWith("/activities"));
  expect(read?.searchParams.get("entity_type")).toBe("company");
  expect(read?.searchParams.get("entity_id")).toBe("o-1");
  expect(read?.searchParams.getAll("kind")).toEqual(["meeting"]);
  // The next meeting is drawn once, on its card, with the account's attendee.
  expect(screen.getAllByText("Rollout kickoff")).toHaveLength(1);
  expect(screen.getByText("Dana Buyer")).toBeTruthy();
  const held = screen.getByRole("heading", { name: "Held" }).closest("section");
  expect(
    within(held as HTMLElement).getByText("Quarterly review"),
  ).toBeTruthy();
  await userEvent
    .setup()
    .click(screen.getAllByRole("button", { name: /Prepare brief/ })[0]);
  expect(onBriefMeeting).toHaveBeenCalledWith(kickoff.id);
});

it("says the meetings are withheld when the account read leaves them out", async () => {
  const { asked } = mount(
    view({ sections_omitted: ["activities", "next_meeting"] }),
  );
  expect((await screen.findAllByText("Hidden for your role")).length).toBe(2);
  expect(asked().some((url) => url.pathname.endsWith("/activities"))).toBe(
    false,
  );
});

describe("on the account's page", () => {
  afterEach(() => {
    window.location.hash = "";
  });

  it("opens the Meetings tab from its address, after Deals and before Tasks", async () => {
    stubFetch(companyBackstop);
    window.location.hash = "#/companies/o-1/meetings";
    renderPage();
    const tab = await screen.findByRole("button", { name: "Meetings" });
    expect(tab.getAttribute("aria-pressed")).toBe("true");
    expect(
      await screen.findByRole("heading", { name: "Upcoming" }),
    ).toBeTruthy();
    const strip = screen.getByTestId("co-tabs");
    const labels = within(strip)
      .getAllByRole("button")
      .map((button) => button.textContent ?? "");
    const at = (label: string) =>
      labels.findIndex((text) => text.startsWith(label));
    expect(at("Meetings")).toBe(at("Deals and projects") + 1);
    expect(at("Tasks")).toBe(at("Meetings") + 1);
  });

  it("sends a link to an archived account's Meetings tab to its Overview", async () => {
    stubFetch(async (url) =>
      url.endsWith("/companies/o-1")
        ? companyJson({ ...company, archived_at: "2026-07-13T00:00:00Z" })
        : companyBackstop(url),
    );
    window.location.hash = "#/companies/o-1/meetings";
    renderPage();
    const overview = await screen.findByRole("button", { name: "Overview" });
    expect(overview.getAttribute("aria-pressed")).toBe("true");
    expect(screen.queryByRole("button", { name: "Meetings" })).toBeNull();
  });
});

function renderPage() {
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <CompanyScreen id="o-1" />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

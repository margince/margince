// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { ACTIVITY_KINDS } from "../../design-system/recordtimeline";
import { pickOption } from "../../design-system/select-testing";
import { isConversationKind } from "../../design-system/timelineentry";
import { UTC_ZONE } from "../../format/timezone";
import { LocaleProvider } from "../../i18n";
import { pretendReaderZone } from "../../testing/readerzone.testkit";
import { jsonResponse, stubWithSession } from "../../testing/story-utils";
import { ContactTimelineTab } from "./contacttabs";

type Contact360 = components["schemas"]["Contact360"];
type Activity = components["schemas"]["Activity"];
type ActivityPage = components["schemas"]["ActivityListResponse"];

const CAPTURED = {
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-08-01T08:00:00Z",
  occurred_precision: "instant",
  offers_email: false,
  is_done: false,
} as const;

const link = (activityId: string) => ({
  id: `l-${activityId}`,
  activity_id: activityId,
  entity_type: "contact" as const,
  entity_id: "p-1",
});

// The newest page the 360 carries: twenty notes and more behind them.
const notes: Activity[] = Array.from({ length: 20 }, (_, index) => ({
  ...CAPTURED,
  id: `n-${index}`,
  kind: "note",
  body: `Note ${index}`,
  links: [link(`n-${index}`)],
  occurred_at: `2026-08-${String(12 - Math.floor(index / 2)).padStart(2, "0")}T09:00:00Z`,
}));

const view: Contact360 = {
  as_of: "2026-08-13T09:00:00Z",
  contact: { id: "p-1", full_name: "Dana Buyer", ...CAPTURED },
  sections_omitted: [],
  activities: {
    data: notes,
    page: { has_more: true, next_cursor: "c-notes" },
  },
};

// Two months back, under the notes: what the server answers the cut.
const thread: ActivityPage = {
  data: [
    {
      ...CAPTURED,
      id: "t-2",
      kind: "email",
      subject: "Re: Fleet renewal",
      direction: "outbound",
      thread_key: "thread-1",
      links: [link("t-2")],
      occurred_at: "2026-06-15T12:00:00Z",
    },
    {
      ...CAPTURED,
      id: "t-1",
      kind: "email",
      subject: "Fleet renewal",
      direction: "inbound",
      thread_key: "thread-1",
      links: [link("t-1")],
      occurred_at: "2026-06-14T12:00:00Z",
    },
  ],
  page: { has_more: false, next_cursor: null },
};

let reads: URL[] = [];

beforeEach(() => {
  pretendReaderZone(UTC_ZONE);
  stubWithSession({ "GET /activities": () => jsonResponse(thread) }, {});
  const routed = globalThis.fetch;
  reads = [];
  vi.stubGlobal("fetch", (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(
      input instanceof Request ? input.url : String(input),
      "https://test.local",
    );
    if (url.pathname.endsWith("/activities")) reads.push(url);
    return routed(input, init);
  });
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function draw() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ContactTimelineTab contactId="p-1" view={view} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("the Threads cut", () => {
  it("asks the server for the conversation kinds and finds a thread past twenty notes", async () => {
    const user = userEvent.setup();
    draw();

    await user.click(await screen.findByRole("button", { name: "Threads" }));

    expect(await screen.findByText("Re: Fleet renewal")).toBeTruthy();
    expect(screen.queryByText("No threads yet.")).toBeNull();
    expect(reads.at(-1)?.searchParams.getAll("kind")).toEqual([
      ...ACTIVITY_KINDS.filter((kind) => isConversationKind(kind)),
    ]);
  });

  it("sends the one kind a reader picks inside the cut", async () => {
    const user = userEvent.setup();
    draw();

    await user.click(await screen.findByRole("button", { name: "Threads" }));
    await pickOption(user, screen.getByLabelText("Activity kind"), "Email");

    await waitFor(() =>
      expect(reads.at(-1)?.searchParams.getAll("kind")).toEqual(["email"]),
    );
  });

  it("reads the whole chronology without a kind on All", async () => {
    draw();

    expect(await screen.findAllByText("Note 0")).not.toHaveLength(0);
    expect(reads).toEqual([]);
  });
});

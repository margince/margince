/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { RecordZoneProvider } from "../app/recordzone";
import { LocaleProvider } from "../i18n";
import { LoadMoreButton } from "../screens/common";
import { withEmailOpener } from "../screens/openemail";
import { groupChronology } from "../screens/timelinegroups";
import { activityTimeline } from "./activitytimeline";
import { GroupedTimelineList } from "./composed";
import {
  dayStartIso,
  TIMELINE_PAGE_SIZE,
  useRecordTimeline,
  useTimelineFilters,
} from "./recordtimeline";
import { pickOption } from "./select-testing";
import { TimelineFilterBar } from "./timelinefilterbar";

// The record timeline is exercised the way a record page composes it — the
// hook, the filter bar, the grouped list and the Load more button together —
// because what a page owes the reader is one list that grows and narrows on
// the SERVER, and a test that drove the hook alone would prove the wiring and
// miss the request it exists to send.

type Activity = components["schemas"]["Activity"];
type ActivityPage = components["schemas"]["ActivityListResponse"];

const CAPTURED = {
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-08-01T08:00:00Z",
} as const;

function activity(
  row: Pick<Activity, "id" | "subject" | "occurred_at"> & Partial<Activity>,
): Activity {
  return { kind: "email", is_done: false, ...CAPTURED, ...row };
}

const NEWEST = activity({
  id: "a-1",
  subject: "Fleet renewal",
  occurred_at: "2026-08-12T09:00:00Z",
});
const OLDER = activity({
  id: "a-2",
  subject: "Invoice question",
  occurred_at: "2026-07-02T09:00:00Z",
});

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

/**
 * activityFeed answers the activity list by cursor and records every URL it
 * was asked, so a test can say exactly which parameters the page sent.
 */
function activityFeed(pages: Record<string, ActivityPage>) {
  const calls: URL[] = [];
  const fetcher = vi.fn(async (input: RequestInfo | URL) => {
    const url = new URL(String(input instanceof Request ? input.url : input));
    if (!url.pathname.endsWith("/activities")) {
      return jsonResponse({});
    }
    calls.push(url);
    const cursor = url.searchParams.get("cursor") ?? "first";
    return jsonResponse(
      pages[cursor] ?? { data: [], page: { has_more: false } },
    );
  });
  return {
    fetcher,
    calls,
    last: () => calls[calls.length - 1],
  };
}

// The installation's zone, stated by this suite rather than taken from a
// default. It is deliberately NOT the fallback: a from/to filter cut on the
// fallback clock and asserted against the fallback clock would agree with
// itself no matter which zone the code actually read, so the one thing this
// file has to prove — that the picked day is cut in the INSTALLATION's zone —
// would be exactly what it could not see.
const INSTALLATION_ZONE = "Asia/Ho_Chi_Minh";

function Harness({ firstPage }: Readonly<{ firstPage?: ActivityPage }>) {
  const [filters, setFilters] = useTimelineFilters("p-1");
  const timeline = useRecordTimeline("person", "p-1", { filters, firstPage });
  const entries = activityTimeline(timeline.activities, "human:u-1", (row) => (
    <button type="button">relink {row.id}</button>
  ));
  return (
    <>
      <TimelineFilterBar value={filters} onChange={setFilters} />
      <GroupedTimelineList
        groups={groupChronology(entries, timeline.hasNextPage)}
        zone={INSTALLATION_ZONE}
      />
      <LoadMoreButton query={timeline} />
    </>
  );
}

/**
 * LeadHarness is the LEAD page's own wiring: the same hook, the same mapper,
 * the same grouped list, on entityType "lead" — plus withEmailOpener, which is
 * what makes a row openable.
 *
 * A separate harness rather than a parameter on the one above, because what it
 * proves is that nothing on the path is person-shaped. The hook takes the
 * entity kind, the mapper reads the server's own summary, and the row is the
 * design system's — so a lead reaches the canonical row for the same reason a
 * contact does, and this fails if any of the three grows a person branch.
 */
function LeadHarness({
  onOpen,
}: Readonly<{ onOpen?: (activityId: string) => void }>) {
  const timeline = useRecordTimeline("lead", "l-1", {});
  const entries = withEmailOpener(
    activityTimeline(timeline.activities, "human:u-1"),
    onOpen ?? (() => {}),
  );
  return (
    <GroupedTimelineList
      groups={groupChronology(entries, timeline.hasNextPage)}
      zone={INSTALLATION_ZONE}
    />
  );
}

function newQueryClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
}

// `client` is a parameter, not always a fresh `newQueryClient()`, so a test
// can mount the same record twice against ONE cache — the only way to prove
// a second mount's query key doesn't collide with what the first left behind.
function mount(firstPage?: ActivityPage, client = newQueryClient()) {
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <RecordZoneProvider zone={INSTALLATION_ZONE}>
          <Harness firstPage={firstPage} />
        </RecordZoneProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return client;
}

/**
 * IdsHarness mounts the hook alone, each returned activity as its own
 * element in id order — so a test can read back exactly which activities
 * came out, by id, rather than trusting a count or a subject line that two
 * different rows could happen to share.
 */
function IdsHarness({ firstPage }: Readonly<{ firstPage?: ActivityPage }>) {
  const [filters] = useTimelineFilters("p-1");
  const timeline = useRecordTimeline("person", "p-1", { filters, firstPage });
  return (
    <>
      {timeline.activities.map((row, index) => (
        // biome-ignore lint/suspicious/noArrayIndexKey: the index is the point — a duplicated id must render twice, not collapse to once.
        <span key={index} data-testid="activity-id">
          {row.id}
        </span>
      ))}
    </>
  );
}

function mountIds(client: QueryClient, firstPage?: ActivityPage) {
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <RecordZoneProvider zone={INSTALLATION_ZONE}>
          <IdsHarness firstPage={firstPage} />
        </RecordZoneProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

function renderedActivityIds(): string[] {
  return screen.getAllByTestId("activity-id").map((el) => el.textContent);
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("a record timeline you can work in", () => {
  it("opens on one page of the size the mobile budget is measured against, then loads the next by cursor and appends it", async () => {
    const user = userEvent.setup();
    const feed = activityFeed({
      first: { data: [NEWEST], page: { has_more: true, next_cursor: "c-1" } },
      "c-1": { data: [OLDER], page: { has_more: false } },
    });
    vi.stubGlobal("fetch", feed.fetcher);
    mount();

    expect(await screen.findByText("Fleet renewal")).toBeTruthy();
    // The open is exactly one request, with nothing a filter would add.
    expect(feed.calls).toHaveLength(1);
    expect([...feed.last().searchParams.keys()].sort()).toEqual([
      "entity_id",
      "entity_type",
      "limit",
    ]);
    expect(feed.last().searchParams.get("limit")).toBe(
      String(TIMELINE_PAGE_SIZE),
    );

    await user.click(screen.getByRole("button", { name: "Load more" }));
    expect(await screen.findByText("Invoice question")).toBeTruthy();
    expect(feed.last().searchParams.get("cursor")).toBe("c-1");
    // Appended, not replaced.
    expect(screen.getByText("Fleet renewal")).toBeTruthy();
    // The server said that was the last page.
    expect(screen.queryByRole("button", { name: "Load more" })).toBeNull();
  });

  it("offers Load more only while the server holds another page", async () => {
    const feed = activityFeed({
      first: { data: [NEWEST], page: { has_more: false } },
    });
    vi.stubGlobal("fetch", feed.fetcher);
    mount();

    expect(await screen.findByText("Fleet renewal")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Load more" })).toBeNull();
  });

  it("seeded from the 360's own page, fetches nothing on open and continues from that page's cursor", async () => {
    const user = userEvent.setup();
    const feed = activityFeed({
      "c-360": { data: [OLDER], page: { has_more: false } },
    });
    vi.stubGlobal("fetch", feed.fetcher);
    mount({ data: [NEWEST], page: { has_more: true, next_cursor: "c-360" } });

    expect(screen.getByText("Fleet renewal")).toBeTruthy();
    expect(feed.calls).toHaveLength(0);

    await user.click(screen.getByRole("button", { name: "Load more" }));
    expect(await screen.findByText("Invoice question")).toBeTruthy();
    expect(feed.calls).toHaveLength(1);
    expect(feed.last().searchParams.get("cursor")).toBe("c-360");
  });

  it("a seed that is the whole history still gets its own cache key, so a later seedless read for the same record doesn't bleed into it", async () => {
    const feed = activityFeed({
      first: { data: [NEWEST], page: { has_more: false } },
    });
    vi.stubGlobal("fetch", feed.fetcher);

    // A seedless mount for this record fetches page one and leaves it in the
    // cache — the same cache the next mount below reads from.
    const client = mount();
    await screen.findByText("Fleet renewal");
    cleanup();

    // A record whose 360 seed holds the WHOLE history has `has_more: false`
    // and so no next cursor — the seed alone can't tell this key apart from
    // the seedless read above, and the query must be told some other way.
    mountIds(client, {
      data: [NEWEST, OLDER],
      page: { has_more: false },
    });

    expect(renderedActivityIds().sort()).toEqual([NEWEST.id, OLDER.id].sort());
  });

  it("sends the kind to the server rather than filtering the page it holds", async () => {
    const user = userEvent.setup();
    const feed = activityFeed({
      first: { data: [NEWEST], page: { has_more: false } },
    });
    vi.stubGlobal("fetch", feed.fetcher);
    mount();
    await screen.findByText("Fleet renewal");

    await pickOption(user, screen.getByLabelText("Activity kind"), "Tasks");
    await waitFor(() =>
      expect(feed.last().searchParams.get("kind")).toBe("task"),
    );
  });

  it("sends a search as q and says that limited conversations are left out", async () => {
    const user = userEvent.setup();
    const feed = activityFeed({
      first: { data: [NEWEST], page: { has_more: false } },
    });
    vi.stubGlobal("fetch", feed.fetcher);
    mount();
    await screen.findByText("Fleet renewal");
    const note = /Conversations whose content you may not open/;
    expect(screen.queryByText(note)).toBeNull();

    await user.type(
      screen.getByLabelText("Search this timeline"),
      "cutover{Enter}",
    );
    await waitFor(() =>
      expect(feed.last().searchParams.get("q")).toBe("cutover"),
    );
    expect(screen.getByText(note)).toBeTruthy();
  });

  it("sends a date range as an inclusive start and an exclusive next-day end", async () => {
    const user = userEvent.setup();
    const feed = activityFeed({
      first: { data: [NEWEST], page: { has_more: false } },
    });
    vi.stubGlobal("fetch", feed.fetcher);
    mount();
    await screen.findByText("Fleet renewal");

    await user.type(screen.getByLabelText("From"), "2026-03-03");
    await user.type(screen.getByLabelText("To"), "2026-03-05");
    await waitFor(() =>
      expect(feed.last().searchParams.get("occurred_before")).not.toBeNull(),
    );
    const params = feed.last().searchParams;
    expect(params.get("occurred_after")).toBe(
      dayStartIso("2026-03-03", INSTALLATION_ZONE),
    );
    expect(params.get("occurred_before")).toBe(
      dayStartIso("2026-03-05", INSTALLATION_ZONE, 1),
    );
  });

  it("folds a conversation into one row that opens, with the conversation's own verbs on it", async () => {
    const user = userEvent.setup();
    const thread = ["a-1", "a-2", "a-3"].map((id, index) =>
      activity({
        id,
        subject: index === 0 ? "Re: Cutover plan" : "Cutover plan",
        occurred_at: `2026-08-1${index}T09:00:00Z`,
        thread_key: "t-1",
      }),
    );
    const feed = activityFeed({
      first: { data: thread, page: { has_more: false } },
    });
    vi.stubGlobal("fetch", feed.fetcher);
    mount();

    expect(await screen.findByText("3 messages")).toBeTruthy();
    // One row for three mails, titled by the newest.
    expect(screen.getAllByText(/Cutover plan/)).toHaveLength(1);
    // Relink is reachable from the group without opening it.
    expect(screen.getByRole("button", { name: "relink a-1" })).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "Open" }));
    expect(screen.getAllByText(/Cutover plan/)).toHaveLength(4);
  });

  // The LEAD page's own path. Every surface it uses is the shared one, so a
  // lead's mail reads exactly as a contact's does — and a person branch
  // appearing anywhere on that path fails here.
  it("draws a lead's email with the canonical row, openable", async () => {
    const opened: string[] = [];
    const feed = activityFeed({
      first: {
        data: [
          activity({
            id: "a-1",
            subject: "Pricing for 40 seats",
            occurred_at: "2026-08-12T09:00:00Z",
            body: "Long body the server already trimmed for the preview",
            email_summary: {
              activity_id: "a-1",
              occurred_at: "2026-08-12T09:00:00Z",
              version: 1,
              subject: "Pricing for 40 seats",
              preview: "What would this cost for 40 seats?",
              counterparty: "Dung Ly",
              direction: "inbound",
              display_status: "team",
              move: "needs_reply",
              attachment_count: 0,
            },
          }),
        ],
        page: { has_more: false },
      },
    });
    vi.stubGlobal("fetch", feed.fetcher);
    render(
      <QueryClientProvider client={newQueryClient()}>
        <LocaleProvider initial="en">
          <RecordZoneProvider zone={INSTALLATION_ZONE}>
            <LeadHarness onOpen={(id) => opened.push(id)} />
          </RecordZoneProvider>
        </LocaleProvider>
      </QueryClientProvider>,
    );

    // The read went to the LEAD's timeline, not a person's.
    await waitFor(() =>
      expect(feed.last().searchParams.get("entity_type")).toBe("lead"),
    );
    // The canonical row, with the server's preview rather than the raw body.
    const row = await screen.findByText("What would this cost for 40 seats?");
    expect(row).toBeTruthy();
    expect(
      screen.queryByText(/Long body the server already trimmed/),
    ).toBeNull();

    // And openable: a row that looks pressable and opens nothing teaches a
    // reader the product does not work.
    const openable = document.querySelector("button.emailentry--open");
    expect(openable).not.toBeNull();
    if (openable) {
      await userEvent.click(openable);
    }
    expect(opened).toEqual(["a-1"]);
  });

  it("draws a collapsed conversation of emails with the canonical row", async () => {
    // The defect this holds: a group wrote its own subject-and-preview markup
    // for the newest message while a lone email beside it drew EmailEntry, so
    // one page showed two readings of a message. The preview differs from the
    // body on purpose — a row rendering the body where the server's preview
    // belongs is the drift, and a fixture whose two texts agreed could not tell
    // them apart.
    const thread = ["a-1", "a-2"].map((id, index) =>
      activity({
        id,
        subject: "Cutover plan",
        body: "Long body with a signature the server already trimmed",
        occurred_at: `2026-08-1${index}T09:00:00Z`,
        thread_key: "t-1",
        email_summary: {
          activity_id: id,
          occurred_at: `2026-08-1${index}T09:00:00Z`,
          version: 1,
          subject: "Cutover plan",
          preview: "Are we still moving on the 14th?",
          counterparty: "Dana Buyer",
          direction: "inbound",
          display_status: "team",
          move: "needs_reply",
          attachment_count: 0,
        },
      }),
    );
    const feed = activityFeed({
      first: { data: thread, page: { has_more: false } },
    });
    vi.stubGlobal("fetch", feed.fetcher);
    mount();

    expect(await screen.findByText("2 messages")).toBeTruthy();
    // The canonical row's own parts, on the COLLAPSED group: the server's
    // preview and the access badge. Asserting the preview alone would pass over
    // a hand-written span that happened to print the same string.
    expect(screen.getByText("Are we still moving on the 14th?")).toBeTruthy();
    expect(document.querySelector(".emailentry")).not.toBeNull();
    // And not the body: the group must not fall back to the raw text when the
    // server composed a preview for it.
    expect(screen.queryByText(/Long body with a signature/)).toBeNull();
  });
});

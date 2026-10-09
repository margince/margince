/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  renderHook,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { ENTITY_KINDS, type EntityKind } from "../app/entity";
import { LocaleProvider, useT } from "../i18n";
import { ContactTimelineTab } from "./contacttabs";
import {
  chronologyNotice,
  hasChronologyFooter,
  type RecordChronology,
  type TimelineFilter,
  useRecordChronology,
} from "./recordchronology";

// History is exercised through the contact tab that renders it, and through
// the hook every record page shares where the question is about the feeds.
//
// All is the conversation and Changes is the record's edits. All can go wrong
// by letting a field change back in among the mail. A capped page can go wrong
// by reading as the whole ledger.

type Contact360 = components["schemas"]["Contact360"];
type SectionActivity = NonNullable<Contact360["activities"]>["data"][number];
type FieldChange = components["schemas"]["FieldHistoryEntry"];

const CAPTURED = {
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-08-01T08:00:00Z",
} as const;

function activity(
  row: Pick<SectionActivity, "id" | "kind" | "occurred_at"> &
    Partial<SectionActivity>,
): SectionActivity {
  return { is_done: false, ...CAPTURED, ...row };
}

const change: FieldChange = {
  id: "h-1",
  entity_type: "contact",
  entity_id: "p-1",
  field: "owner_id",
  old_value: null,
  new_value: "Lena Fischer",
  // Between the two activities below, where a time-ordered merge would put it.
  changed_at: "2026-08-10T09:00:00Z",
  actor_type: "human",
  actor_id: "u-1",
};

function viewWith(hasMore: boolean): Contact360 {
  return {
    as_of: "2026-08-13T09:00:00Z",
    contact: { id: "p-1", full_name: "Dana Buyer", ...CAPTURED },
    sections_omitted: [],
    activities: {
      data: [
        activity({
          id: "a-1",
          kind: "email",
          subject: "Fleet renewal",
          occurred_at: "2026-08-11T12:00:00Z",
        }),
        activity({
          id: "a-2",
          kind: "email",
          subject: "Depot access",
          occurred_at: "2026-08-09T08:00:00Z",
        }),
      ],
      page: { has_more: hasMore },
    },
  };
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

/**
 * changeFeed answers the two change reads a record page can make and nothing
 * else. They are the field-history feed and the record history behind the
 * contact's Changes view. A stub answering every URL with one body would pass
 * while the page asked for something else. A stub answering with a shape the
 * endpoint never returns would report a crash the product cannot have.
 */
function changeFeed(status = 200) {
  const calls: string[] = [];
  const fetcher = vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input instanceof Request ? input.url : input);
    calls.push(url);
    if (url.includes("/field-history")) {
      return jsonResponse(
        status === 200
          ? { data: [change], page: { has_more: false } }
          : { title: "boom" },
        status,
      );
    }
    if (/\/records\/[^/]+\/[^/]+\/history/.test(url)) {
      return jsonResponse({ data: [], page: { has_more: false } }, status);
    }
    return jsonResponse({});
  });
  return {
    fetcher,
    changesRead: () => calls.some((url) => url.includes("/field-history")),
    historyRead: () =>
      calls.some((url) => /\/records\/[^/]+\/[^/]+\/history/.test(url)),
  };
}

function withProviders(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{node}</LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the record's History", () => {
  it("opens on All, which holds the exchanges and no field change", async () => {
    const feed = changeFeed();
    vi.stubGlobal("fetch", feed.fetcher);
    withProviders(
      <ContactTimelineTab contactId="p-1" view={viewWith(false)} />,
    );

    expect(screen.getByText("Fleet renewal")).toBeTruthy();
    expect(screen.getByText("Depot access")).toBeTruthy();
    expect(screen.queryByText("Owner")).toBeNull();
    expect(feed.changesRead()).toBe(false);
  });

  it("offers All, Threads and Changes, and no Activities cut", () => {
    withProviders(
      <ContactTimelineTab contactId="p-1" view={viewWith(false)} />,
    );

    const cuts = screen
      .getByRole("group", { name: "Timeline filter" })
      .querySelectorAll("button");
    expect([...cuts].map((cut) => cut.textContent)).toEqual([
      "All",
      "Threads",
      "Changes",
    ]);
  });

  it("reads the record's history only once the reader asks for the changes", async () => {
    const user = userEvent.setup();
    const feed = changeFeed();
    vi.stubGlobal("fetch", feed.fetcher);
    withProviders(
      <ContactTimelineTab contactId="p-1" view={viewWith(false)} />,
    );

    expect(feed.historyRead()).toBe(false);

    await user.click(screen.getByRole("button", { name: "Changes" }));

    await waitFor(() => expect(feed.historyRead()).toBe(true));
    // An activity showing here would mean the filter narrowed nothing.
    expect(screen.queryByText("Fleet renewal")).toBeNull();
  });

  it("states that a capped page is not the whole ledger", () => {
    withProviders(<ContactTimelineTab contactId="p-1" view={viewWith(true)} />);

    expect(
      screen.getByText("Only the most recent activities are shown."),
    ).toBeTruthy();
  });

  it("keeps that notice off a page the server did not cut", () => {
    withProviders(
      <ContactTimelineTab contactId="p-1" view={viewWith(false)} />,
    );

    expect(
      screen.queryByText(/Only the most recent activities are shown/),
    ).toBeNull();
  });
});

// Every record page reads its History through this hook, so the rule is held
// here once per record kind rather than once per page.
describe("useRecordChronology", () => {
  const activities = [
    activity({
      id: "a-1",
      kind: "email",
      subject: "Fleet renewal",
      occurred_at: "2026-08-11T12:00:00Z",
    }),
  ];

  function wrapper({ children }: Readonly<{ children: ReactNode }>) {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    return (
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">{children}</LocaleProvider>
      </QueryClientProvider>
    );
  }

  function hook(
    kind: EntityKind,
    filter: TimelineFilter,
    status = 200,
    changesInPanel = false,
  ) {
    const feed = changeFeed(status);
    vi.stubGlobal("fetch", feed.fetcher);
    const rendered = renderHook(
      () =>
        useRecordChronology({
          kind,
          recordId: "p-1",
          filter,
          changesInPanel,
          activities,
          activitiesHaveMore: false,
          values: { currency: null, locale: "en", zone: "UTC" },
        }),
      { wrapper },
    );
    return { ...rendered, feed };
  }

  it.each(ENTITY_KINDS)(
    "keeps every field change out of a %s's All and never reads the change feed for it",
    (kind) => {
      const { result, feed } = hook(kind, "all");

      expect(result.current.entries.map((entry) => entry.id)).toEqual(["a-1"]);
      expect(result.current.changes.fetchStatus).toBe("idle");
      expect(feed.changesRead()).toBe(false);
    },
  );

  it.each(ENTITY_KINDS)(
    "lists a %s's field changes under Changes, without the exchanges",
    async (kind) => {
      const { result } = hook(kind, "changes");

      await waitFor(() =>
        expect(result.current.entries.map((entry) => entry.kind)).toEqual([
          "change",
        ]),
      );
    },
  );

  it("says a failed change read failed rather than reporting nothing was ever changed", async () => {
    const { result } = hook("contact", "changes", 500);

    await waitFor(() => expect(result.current.failed).toBe(true));
    expect(result.current.entries).toEqual([]);
  });

  it("leaves Changes to the page's own panel: no read, and no Load more", async () => {
    const { result, feed } = hook("deal", "changes", 200, true);

    expect(result.current.changes.fetchStatus).toBe("idle");
    expect(feed.changesRead()).toBe(false);
    expect(hasChronologyFooter("changes", result.current)).toBe(false);
  });

  it("offers the next page of changes where it draws them itself", async () => {
    const { result } = hook("company", "changes");

    await waitFor(() => expect(result.current.entries).toHaveLength(1));
    expect(hasChronologyFooter("changes", result.current)).toBe(true);
  });

  // Changes is judged by the change feed alone. The activity read failing, or
  // the 360 withholding activities, must not hide changes that loaded.
  const BROKEN_EXCHANGES = { loading: true, failed: true, assembled: false };

  function NoticeOf({
    filter,
    chronology,
  }: Readonly<{ filter: TimelineFilter; chronology: RecordChronology }>) {
    const t = useT();
    return (
      <>
        {chronologyNotice(
          "contact.timeline.empty",
          filter,
          chronology,
          BROKEN_EXCHANGES,
          t,
        ) ?? "rows drawn"}
      </>
    );
  }

  it("shows loaded changes whatever the activity read did", async () => {
    const { result } = hook("company", "changes");
    await waitFor(() => expect(result.current.entries).toHaveLength(1));

    withProviders(<NoticeOf filter="changes" chronology={result.current} />);
    expect(screen.getByText("rows drawn")).toBeTruthy();
  });

  it("still holds the exchange cuts to the activity read", () => {
    const { result } = hook("company", "all");

    withProviders(<NoticeOf filter="all" chronology={result.current} />);
    expect(screen.queryByText("rows drawn")).toBeNull();
  });

  it("reports the change feed's own failure under Changes", async () => {
    const { result } = hook("company", "changes", 500);
    await waitFor(() => expect(result.current.failed).toBe(true));

    withProviders(<NoticeOf filter="changes" chronology={result.current} />);
    expect(screen.getByText(/Some data did not load/)).toBeTruthy();
  });
});

/** @vitest-environment jsdom */
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
import { LocaleProvider } from "../i18n";
import { PersonTimelineTab } from "./persontabs";
import { useRecordChronology } from "./recordchronology";

// The chronology is exercised THROUGH the tab that renders it: what the
// record page owes a reader is one list in one order, and a test that drove
// the hook alone would prove the merge and miss the thing the merge exists
// for. The `narrowed` describe block below is the one exception, and it says
// why it needs the hook directly rather than the tab.
//
// The three filters are three different reads, and each has its own way of
// being wrong: Activities can report the 360's capped page as the whole
// ledger, Changes can read a failed fetch as "nothing was ever changed", and
// All can drop one feed's rows on the floor while looking perfectly ordered.

type Person360 = components["schemas"]["Person360"];
type SectionActivity = NonNullable<Person360["activities"]>["data"][number];
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
  entity_type: "person",
  entity_id: "p-1",
  field: "owner_id",
  old_value: null,
  new_value: "Lena Fischer",
  // BETWEEN the two activities below, so a merge that ordered by time puts it
  // in the middle and one that merely concatenated the feeds does not.
  changed_at: "2026-08-10T09:00:00Z",
  actor_type: "human",
  actor_id: "u-1",
};

function viewWith(hasMore: boolean): Person360 {
  return {
    as_of: "2026-08-13T09:00:00Z",
    person: { id: "p-1", full_name: "Dana Buyer", ...CAPTURED },
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
 * changeFeed answers the two reads this panel makes — the field-history feed
 * the chronology assembles, and the record history the Changes view now IS —
 * and nothing else. A stub that answered every URL with the same body would
 * let a test pass while the page asked for something entirely different, and
 * one that answered an endpoint with a shape it never returns would report a
 * crash the product cannot have.
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

describe("the record's chronology", () => {
  it("opens on the whole chronology, which costs the changes read", async () => {
    const feed = changeFeed();
    vi.stubGlobal("fetch", feed.fetcher);
    withProviders(<PersonTimelineTab personId="p-1" view={viewWith(false)} />);

    expect(screen.getByText("Fleet renewal")).toBeTruthy();
    // What was said and what changed are one order of events, so the record
    // opens on both. The second read is what that costs, and it is the price
    // of not asking a reader to know a cut exists before they can see the
    // whole ledger.
    await waitFor(() => expect(feed.changesRead()).toBe(true));
  });

  it("reads the record's history only once the reader asks for the changes", async () => {
    const user = userEvent.setup();
    const feed = changeFeed();
    vi.stubGlobal("fetch", feed.fetcher);
    withProviders(<PersonTimelineTab personId="p-1" view={viewWith(false)} />);

    // The Changes view IS the record's history now, so the read it must not
    // spend before being asked is that one.
    expect(feed.historyRead()).toBe(false);

    await user.click(screen.getByRole("button", { name: "Changes" }));

    await waitFor(() => expect(feed.historyRead()).toBe(true));
    // An activity showing here would mean the filter narrowed nothing.
    expect(screen.queryByText("Fleet renewal")).toBeNull();
  });

  it("puts both feeds in one order under All, newest first", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", changeFeed().fetcher);
    withProviders(<PersonTimelineTab personId="p-1" view={viewWith(false)} />);

    await user.click(screen.getByRole("button", { name: "All" }));

    await waitFor(() => expect(screen.getByText("owner id")).toBeTruthy());
    const rows = screen.getAllByRole("listitem");
    const text = rows.map((row) => row.textContent ?? "");
    const mailFirst = text.findIndex((row) => row.includes("Fleet renewal"));
    const changed = text.findIndex((row) => row.includes("owner id"));
    const mailLast = text.findIndex((row) => row.includes("Depot access"));
    // 11 Aug, then the 10 Aug change, then 9 Aug: interleaved by time rather
    // than one feed appended to the other.
    expect(mailFirst).toBeLessThan(changed);
    expect(changed).toBeLessThan(mailLast);
  });

  it("says a failed change read failed rather than reporting nothing was ever changed", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", changeFeed(500).fetcher);
    withProviders(<PersonTimelineTab personId="p-1" view={viewWith(false)} />);

    // Under All, where the chronology still assembles both feeds.
    await user.click(screen.getByRole("button", { name: "All" }));

    await waitFor(() =>
      expect(screen.getByText("This section did not load.")).toBeTruthy(),
    );
    expect(
      screen.queryByText(
        "No field on this record has been changed since it was created.",
      ),
    ).toBeNull();
  });

  it("states that a capped page is not the whole ledger, in the words of the cut", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", changeFeed().fetcher);
    withProviders(<PersonTimelineTab personId="p-1" view={viewWith(true)} />);

    expect(screen.getByText("Fleet renewal")).toBeTruthy();
    // On the combined cut the honest sentence is about BOTH feeds: the merge
    // can only order rows it can prove are in order, so what it drops is not
    // "older activities" but older entries of either kind.
    await waitFor(() =>
      expect(screen.getByText(/Older entries are not shown here/)).toBeTruthy(),
    );

    await user.click(screen.getByRole("button", { name: "Activities" }));

    // On the activities cut the same fact has a narrower and more useful
    // sentence, because there is only one feed to be cut.
    expect(
      screen.getByText(
        "There are more activities here than fit. Only the most recent ones are listed.",
      ),
    ).toBeTruthy();
  });

  it("keeps that notice off a page the server did not cut", () => {
    withProviders(<PersonTimelineTab personId="p-1" view={viewWith(false)} />);

    expect(screen.queryByText(/more activities here than fit/)).toBeNull();
  });
});

// `narrowed` is tested against the hook directly rather than through a tab: a
// narrowed read only exists once the reader has already typed a search or
// picked a kind, and by the time a component mount could reach that state the
// unnarrowed render before it would already have fired the changes fetch the
// assertion is about. The hook takes `narrowed` as a starting condition, which
// only a direct render can hold from the first render.
describe("useRecordChronology's narrowed reads", () => {
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

  function hook(narrowed: boolean) {
    const feed = changeFeed();
    vi.stubGlobal("fetch", feed.fetcher);
    const rendered = renderHook(
      () =>
        useRecordChronology({
          kind: "person",
          recordId: "p-1",
          filter: "all",
          narrowed,
          activities,
          activitiesHaveMore: false,
          // The reading context every change row needs. A person record holds
          // no money of its own, and the zone comes from the harness's own
          // provider the same way a record page takes it from the workspace.
          values: { currency: null, locale: "en", zone: "UTC" },
        }),
      { wrapper },
    );
    return { ...rendered, feed };
  }

  it("drops the record's own field-change rows once the read is narrowed", async () => {
    const { result } = hook(true);

    // The activity is still on screen — narrowing removes the change rows, not
    // the whole merge.
    await waitFor(() =>
      expect(result.current.entries.some((entry) => entry.id === "a-1")).toBe(
        true,
      ),
    );
    // A field edit is not a meeting: a narrowed search for Meetings must never
    // surface "owner id was changed" beside them, even though the change feed
    // above is stubbed to answer with exactly that row.
    expect(
      result.current.entries.some((entry) => entry.kind === "change"),
    ).toBe(false);
  });

  it("keeps the change rows in the merge when the read is not narrowed", async () => {
    const { result } = hook(false);

    // Same activities, same stubbed change feed, `narrowed: false` — the
    // change row appears, which is what proves the first test's empty change
    // list came from the narrowing and not from the feed having nothing to
    // give.
    await waitFor(() =>
      expect(
        result.current.entries.some((entry) => entry.kind === "change"),
      ).toBe(true),
    );
  });

  it("never asks the field-history endpoint for a narrowed read", async () => {
    const { result, feed } = hook(true);

    await waitFor(() =>
      expect(result.current.entries.some((entry) => entry.id === "a-1")).toBe(
        true,
      ),
    );
    // A feed whose rows can never appear must not be fetched at all — the
    // query itself stays off, not merely filtered after the fact.
    expect(result.current.changes.fetchStatus).toBe("idle");
    expect(feed.changesRead()).toBe(false);
  });
});

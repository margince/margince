/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Approval } from "./approvals.queries";
import {
  useDecidedApprovals,
  usePendingApprovals,
  useTargetApprovals,
} from "./approvals.queries";

// The two things this family exists to get right, and neither is visible from a
// screen test:
//
// A page cap of 50 is not the answer. The pending/decided partition needs the
// WHOLE set to sort and filter correctly, so a 51st approval must arrive — and a
// walk that stops at the first page loses it silently, which is the shape of
// every "the list looked complete" defect.
//
// An EXPIRED row is wired back on the status=pending response, because the
// server computes expiry lazily at read time and there is no status=expired to
// query. So the same response feeds two tabs: Pending must drop it (it cannot be
// acted on) and Decided must salvage it (it happened, and a reader looking for
// what became of it finds it nowhere else).

// The wire is the boundary worth standing in for: stubbing `api.GET` would let a
// fixture claim a body the contract does not describe.
function stubPages(pages: Record<string, unknown>) {
  const seen: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = new URL(
        input instanceof Request ? input.url : String(input),
        "https://test.local",
      );
      const status = url.searchParams.get("status") ?? "";
      const cursor = url.searchParams.get("cursor") ?? "";
      const target = url.searchParams.get("target_entity_id") ?? "";
      const key = [status, cursor, target].filter(Boolean).join("|");
      seen.push(key);
      const body = pages[key] ?? { data: [], page: { has_more: false } };
      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
  return seen;
}

function approval(over: Partial<Approval>): Approval {
  return {
    id: "ap-1",
    kind: "send_email",
    status: "pending",
    proposed_by: "agent:runner",
    summary: "Send the follow-up",
    created_at: "2026-08-01T09:00:00Z",
    ...over,
  } as unknown as Approval;
}

function wrapper() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("usePendingApprovals", () => {
  it("walks every page rather than answering with the first one", async () => {
    const seen = stubPages({
      pending: {
        data: [approval({ id: "ap-1" })],
        page: { has_more: true, next_cursor: "c2" },
      },
      "pending|c2": {
        data: [approval({ id: "ap-2" })],
        page: { has_more: false, next_cursor: null },
      },
    });

    const { result } = renderHook(() => usePendingApprovals(), {
      wrapper: wrapper(),
    });

    await waitFor(() => expect(result.current.data).toBeDefined());
    expect(result.current.data?.data.map((a) => a.id)).toEqual([
      "ap-1",
      "ap-2",
    ]);
    // The second request is the point: a cursor the server minted was followed.
    expect(seen).toContain("pending|c2");
  });

  it("drops a lazily-expired row, which is not something to act on", async () => {
    stubPages({
      pending: {
        data: [
          approval({ id: "live" }),
          approval({ id: "aged", status: "expired" }),
        ],
        page: { has_more: false },
      },
    });

    const { result } = renderHook(() => usePendingApprovals(), {
      wrapper: wrapper(),
    });

    await waitFor(() => expect(result.current.data).toBeDefined());
    expect(result.current.data?.data.map((a) => a.id)).toEqual(["live"]);
  });

  it("reports a refusal rather than an empty queue", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({ title: "Denied", status: 403, code: "forbidden" }),
            { status: 403, headers: { "Content-Type": "application/json" } },
          ),
      ),
    );

    const { result } = renderHook(() => usePendingApprovals(), {
      wrapper: wrapper(),
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    // An empty page and a refused read are different answers, and a queue that
    // renders the first for the second tells a reader nothing is waiting.
    expect(result.current.data).toBeUndefined();
  });
});

describe("useDecidedApprovals", () => {
  it("merges approved, rejected and the salvaged expired, newest first", async () => {
    stubPages({
      approved: {
        data: [
          approval({
            id: "yes",
            status: "approved",
            decided_at: "2026-08-03T09:00:00Z",
          }),
        ],
        page: { has_more: false },
      },
      rejected: {
        data: [
          approval({
            id: "no",
            status: "rejected",
            decided_at: "2026-08-05T09:00:00Z",
          }),
        ],
        page: { has_more: false },
      },
      pending: {
        data: [
          approval({ id: "live" }),
          approval({
            id: "aged",
            status: "expired",
            expires_at: "2026-08-04T09:00:00Z",
          }),
        ],
        page: { has_more: false },
      },
    });

    const { result } = renderHook(() => useDecidedApprovals(), {
      wrapper: wrapper(),
    });

    await waitFor(() => expect(result.current.data).toBeDefined());
    // Newest decision first, and the expired row sorts by when it aged out —
    // not by when it was created, which would file an old-but-just-expired item
    // as the oldest thing in the list.
    expect(result.current.data?.data.map((a) => a.id)).toEqual([
      "no",
      "aged",
      "yes",
    ]);
  });

  it("answers nothing at all while any of its three reads is in flight", async () => {
    stubPages({
      approved: { data: [], page: { has_more: false } },
      rejected: { data: [], page: { has_more: false } },
      pending: { data: [], page: { has_more: false } },
    });

    const { result } = renderHook(() => useDecidedApprovals(), {
      wrapper: wrapper(),
    });

    // A partial merge is worse than a pending state: it would render as "these
    // are the decisions" while two of the three sources had not answered.
    expect(result.current.isPending).toBe(true);
    expect(result.current.data).toBeUndefined();
    await waitFor(() => expect(result.current.isPending).toBe(false));
  });

  it("does not fetch its decided-only reads until the tab is open", async () => {
    const seen = stubPages({
      pending: { data: [], page: { has_more: false } },
    });

    const { result } = renderHook(() => useDecidedApprovals(false), {
      wrapper: wrapper(),
    });

    await waitFor(() => expect(seen).toContain("pending"));
    expect(seen).not.toContain("approved");
    expect(seen).not.toContain("rejected");
    // Gated reads never resolve, so the merged view stays pending rather than
    // claiming an empty decided list.
    expect(result.current.isPending).toBe(true);
  });

  it("refetches all three sources when the reader asks again", async () => {
    const seen = stubPages({
      approved: { data: [], page: { has_more: false } },
      rejected: { data: [], page: { has_more: false } },
      pending: { data: [], page: { has_more: false } },
    });

    const { result } = renderHook(() => useDecidedApprovals(), {
      wrapper: wrapper(),
    });

    await waitFor(() => expect(result.current.data).toBeDefined());
    const before = seen.length;
    result.current.refetch();
    await waitFor(() => expect(seen.length).toBeGreaterThan(before + 2));
  });
});

describe("useTargetApprovals", () => {
  it("asks the server for one record's queue and keeps only what is pending", async () => {
    const seen = stubPages({
      "pending|org-1": {
        data: [
          approval({ id: "live" }),
          approval({ id: "aged", status: "expired" }),
        ],
        page: { has_more: false },
      },
    });

    const { result } = renderHook(
      () => useTargetApprovals("organization", "org-1"),
      { wrapper: wrapper() },
    );

    await waitFor(() => expect(result.current.data).toBeDefined());
    expect(result.current.data?.data.map((a) => a.id)).toEqual(["live"]);
    // The record is a SERVER-side filter, not a client-side one: reading the
    // whole workspace queue and narrowing here would page past the record's own
    // approvals long before it found them.
    expect(seen).toContain("pending|org-1");
  });

  it("asks nothing while it is gated", async () => {
    const seen = stubPages({});

    const { result } = renderHook(
      () => useTargetApprovals("organization", "org-1", false),
      { wrapper: wrapper() },
    );

    expect(result.current.isPending).toBe(true);
    expect(seen).toEqual([]);
  });
});

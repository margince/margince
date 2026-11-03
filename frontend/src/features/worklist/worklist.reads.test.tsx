// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { day, row } from "../../testing/worklist.testkit";
import {
  accumulatedDay,
  loadedQueue,
  readWorklistPage,
  useWorklist,
} from "./worklist.reads";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("continues an empty snapshot page when the server supplies a cursor", async () => {
  const asked: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input instanceof Request ? input.url : input);
      asked.push(url);
      const resumed =
        new URL(url).searchParams.get("cursor") === "after-departed";
      return new Response(
        JSON.stringify(
          day(
            resumed
              ? {
                  queue: [
                    row({ id: "still-pending", title: "Send the proposal" }),
                  ],
                }
              : { queue: [], next_cursor: "after-departed" },
          ),
        ),
        {
          status: 200,
          headers: { "content-type": "application/json" },
        },
      );
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  const { result } = renderHook(() => useWorklist("mine", "all"), { wrapper });
  await waitFor(() => expect(result.current.isSuccess).toBe(true));
  expect(result.current.hasNextPage).toBe(true);
  await act(async () => {
    await result.current.fetchNextPage();
  });
  expect(asked.some((url) => url.includes("cursor=after-departed"))).toBe(true);
  await waitFor(() =>
    expect(
      loadedQueue(result.current.data?.pages ?? []).map((item) => item.id),
    ).toEqual(["still-pending"]),
  );
  expect(result.current.hasNextPage).toBe(false);
  client.clear();
});

it("keeps later source failures and limits visible across the frozen walk", () => {
  const first = day({
    counts: [
      { category: "tasks", considered: 2, shown: 1, more_available: false },
    ],
    reach: [{ source: "task", considered: 2, shown: 1, more_available: false }],
    queue: [row({ id: "one" })],
  });
  const later = day({
    sources_unavailable: [
      { source: "approval", reason: "failed", category: "decisions" },
    ],
    counts: [
      { category: "tasks", considered: 2, shown: 1, more_available: true },
    ],
    reach: [{ source: "task", considered: 2, shown: 1, more_available: true }],
    queue: [row({ id: "two" })],
  });
  const combined = accumulatedDay([first, later]);
  expect(combined?.sources_unavailable).toEqual(later.sources_unavailable);
  expect(combined?.counts[0]).toEqual({
    category: "tasks",
    considered: 2,
    shown: 2,
    more_available: true,
  });
  expect(combined?.reach?.[0]?.more_available).toBe(true);
  expect(combined?.queue.map((item) => item.id)).toEqual(["one", "two"]);
});

// A fetch that hangs until its request is aborted, recording each request.
function hangingFetch(requests: Request[]) {
  return vi.fn(
    (input: RequestInfo | URL, init?: RequestInit) =>
      new Promise<Response>((_, reject) => {
        const request = new Request(input, init);
        requests.push(request);
        request.signal.addEventListener("abort", () =>
          reject(new DOMException("aborted", "AbortError")),
        );
      }),
  );
}

it("aborts the day's read when the last screen observing it leaves, and not before", async () => {
  const requests: Request[] = [];
  vi.stubGlobal("fetch", hangingFetch(requests));
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  const home = renderHook(() => useWorklist("mine", "all"), { wrapper });
  const drawer = renderHook(() => useWorklist("mine", "all"), { wrapper });
  await waitFor(() => expect(requests).toHaveLength(1));

  drawer.unmount();
  expect(requests[0].signal.aborted).toBe(false);

  home.unmount();
  await waitFor(() => expect(requests[0].signal.aborted).toBe(true));
});

/** Answers every read with one page and records the query it was asked. */
function serve(page: ReturnType<typeof day>): URLSearchParams[] {
  const asked: URLSearchParams[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      asked.push(
        new URL(String(input instanceof Request ? input.url : input))
          .searchParams,
      );
      return new Response(JSON.stringify(page), {
        status: 200,
        headers: { "content-type": "application/json" },
      });
    }),
  );
  return asked;
}

it("sends Decisions its own filter, so a server without views still narrows", async () => {
  const asked = serve(day({ view: "decisions" }));
  await readWorklistPage(
    "mine",
    "all",
    undefined,
    undefined,
    undefined,
    "decisions",
  );
  expect(asked[0]?.get("view")).toBe("decisions");
  expect(asked[0]?.get("filter")).toBe("decisions");
});

it("keeps only Today's rows when the server echoes no view", async () => {
  serve(
    day({
      queue: [
        row({ id: "t1", category: "tasks" }),
        row({ id: "p1", source: "dedupe_candidate", category: "decisions" }),
      ],
    }),
  );
  const page = await readWorklistPage(
    "mine",
    "all",
    undefined,
    undefined,
    undefined,
    "today",
  );
  expect(page.queue.map((item) => item.id)).toEqual(["t1"]);
});

it("trusts a server that echoes the view", async () => {
  serve(
    day({
      view: "today",
      queue: [
        row({ id: "p1", source: "dedupe_candidate", category: "decisions" }),
      ],
    }),
  );
  const page = await readWorklistPage(
    "mine",
    "all",
    undefined,
    undefined,
    undefined,
    "today",
  );
  expect(page.queue).toHaveLength(1);
});

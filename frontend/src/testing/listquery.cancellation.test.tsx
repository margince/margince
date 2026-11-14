/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import {
  type ListPage,
  type ListQuery,
  useListQuery,
} from "../components/listquery";
import { fetchLeadsPage } from "../features/leads/leads.list";
import { cancellationClient, holdReads } from "./cancellation";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  window.history.replaceState(null, "", "#/");
});

it("aborts an abandoned list read and does not cache its late answer", async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const signals: AbortSignal[] = [];
  let release!: (page: ListPage<{ id: string }>) => void;
  const fetchPage = vi.fn(
    (_query: ListQuery, _cursor: string | null, signal: AbortSignal) => {
      signals.push(signal);
      return new Promise<ListPage<{ id: string }>>((resolve) => {
        release = resolve;
      });
    },
  );
  function wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
  }
  const view = renderHook(
    () => useListQuery({ key: "cancel-list", fetchPage }),
    { wrapper },
  );
  await waitFor(() => expect(fetchPage).toHaveBeenCalledTimes(1));
  view.unmount();
  expect(signals[0]?.aborted).toBe(true);
  release({
    data: [{ id: "late" }],
    page: { next_cursor: null, has_more: false },
  });
  await waitFor(() =>
    expect(
      client.getQueryCache().find({ queryKey: ["cancel-list"], exact: false })
        ?.state.fetchStatus,
    ).toBe("idle"),
  );
  expect(
    client
      .getQueriesData({ queryKey: ["cancel-list"] })
      .every(([, data]) => data === undefined),
  ).toBe(true);
  client.clear();
});

it("aborts the old filter's HTTP read and accepts only the new filter's answer", async () => {
  const reads = holdReads((url) => url.pathname === "/v1/leads");
  const { client, wrapper } = cancellationClient();
  const view = renderHook(
    () => useListQuery({ key: "filtered-leads", fetchPage: fetchLeadsPage }),
    { wrapper },
  );
  await waitFor(() => expect(reads).toHaveLength(1));
  act(() => view.result.current.setQuery((query) => ({ ...query, q: "new" })));
  await waitFor(() => expect(reads).toHaveLength(2));
  expect(reads[0]?.request.signal.aborted).toBe(true);
  expect(
    new URL(reads[1]?.request.url ?? "http://localhost").searchParams.get("q"),
  ).toBe("new");
  await act(async () => {
    reads[0]?.answer({ data: [{ id: "old" }], page: { has_more: false } });
    reads[1]?.answer({ data: [{ id: "new" }], page: { has_more: false } });
  });
  await waitFor(() =>
    expect(view.result.current.rows.map((row) => row.id)).toEqual(["new"]),
  );
  expect(
    client
      .getQueriesData({ queryKey: ["filtered-leads"] })
      .filter(([, data]) => data !== undefined),
  ).toHaveLength(1);
  view.unmount();
  client.clear();
  vi.restoreAllMocks();
});

it("keeps the first page when a later page is abandoned and answers late", async () => {
  const reads = holdReads((url) => url.pathname === "/v1/leads");
  const { client, wrapper } = cancellationClient();
  const view = renderHook(
    () => useListQuery({ key: "paged-leads", fetchPage: fetchLeadsPage }),
    { wrapper },
  );
  await waitFor(() => expect(reads).toHaveLength(1));
  await act(async () => {
    reads[0]?.answer({
      data: [{ id: "first" }],
      page: { has_more: true, next_cursor: "next" },
    });
  });
  await waitFor(() => expect(view.result.current.hasMore).toBe(true));
  act(() => {
    view.result.current.loadMore();
  });
  await waitFor(() => expect(reads).toHaveLength(2));
  expect(
    new URL(reads[1]?.request.url ?? "http://localhost").searchParams.get(
      "cursor",
    ),
  ).toBe("next");
  const before = client.getQueriesData({ queryKey: ["paged-leads"] });
  view.unmount();
  expect(reads[1]?.request.signal.aborted).toBe(true);
  await act(async () => {
    reads[1]?.answer({ data: [{ id: "late" }], page: { has_more: false } });
  });
  expect(client.getQueriesData({ queryKey: ["paged-leads"] })).toEqual(before);
  client.clear();
  vi.restoreAllMocks();
});

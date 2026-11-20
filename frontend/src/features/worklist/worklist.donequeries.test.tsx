// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Done on more rows than one call takes: runs of at most 200, all or none.

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { homeDayOptions } from "../../lib/worklistday";
import { taskRow } from "../../testing/brief.fixtures";
import { MAX_DONE_ROWS, useMarkRowsDone } from "./worklist.donequeries";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const rows = Array.from({ length: MAX_DONE_ROWS + 1 }, (_, at) => ({
  source: "deal_at_risk",
  row_id: `row-${at}`,
}));

function callsWith(failOn?: number) {
  const calls: { path: string; count: number }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request) => {
      const body = JSON.parse(await input.clone().text()) as {
        rows: unknown[];
      };
      calls.push({
        path: `${input.method} ${new URL(input.url).pathname}`,
        count: body.rows.length,
      });
      return calls.length === failOn
        ? new Response(JSON.stringify({ title: "nope" }), { status: 500 })
        : new Response(null, { status: 204 });
    }),
  );
  return calls;
}

function hook() {
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  });
  return renderHook(() => useMarkRowsDone(), {
    wrapper: ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    ),
  });
}

describe("Done on more rows than one call takes", () => {
  it("sends them in runs of at most 200", async () => {
    const calls = callsWith();
    const { result } = hook();
    await act(() => result.current.mutateAsync({ rows }));
    expect(calls).toEqual([
      { path: "PUT /v1/worklist/done", count: MAX_DONE_ROWS },
      { path: "PUT /v1/worklist/done", count: 1 },
    ]);
  });

  it("takes back the runs already written when a later one fails", async () => {
    const calls = callsWith(2);
    const { result } = hook();
    await act(async () => {
      await expect(result.current.mutateAsync({ rows })).rejects.toThrow();
    });
    expect(calls).toEqual([
      { path: "PUT /v1/worklist/done", count: MAX_DONE_ROWS },
      { path: "PUT /v1/worklist/done", count: 1 },
      { path: "POST /v1/worklist/done/undo", count: MAX_DONE_ROWS },
    ]);
  });
});

describe("Done on a Home row", () => {
  it("takes the row off the cached Home day before the server answers", async () => {
    const client = new QueryClient({
      defaultOptions: { mutations: { retry: false } },
    });
    const key = homeDayOptions().queryKey;
    const item = (id: string) => ({
      ...taskRow(id, "Deal"),
      source: "deal_at_risk" as const,
    });
    client.setQueryData(key, {
      pages: [
        {
          queue: [item("row-1"), item("row-2")],
        } as components["schemas"]["Worklist"],
      ],
      pageParams: [undefined],
    });
    const answer: { release: () => void } = { release: () => {} };
    vi.stubGlobal(
      "fetch",
      vi.fn(
        () =>
          new Promise<Response>((resolve) => {
            answer.release = () => resolve(new Response(null, { status: 204 }));
          }),
      ),
    );
    const { result } = renderHook(() => useMarkRowsDone(), {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryClientProvider client={client}>{children}</QueryClientProvider>
      ),
    });
    const ids = () =>
      (
        client.getQueryData<{ pages: { queue: { id: string }[] }[] }>(key)
          ?.pages[0]?.queue ?? []
      ).map((row) => row.id);
    let sent: Promise<void> = Promise.resolve();
    act(() => {
      sent = result.current.mutateAsync({
        rows: [{ source: "deal_at_risk", row_id: "row-1" }],
      });
    });
    await vi.waitFor(() => expect(ids()).toEqual(["row-2"]));
    answer.release();
    await act(() => sent);
  });
});

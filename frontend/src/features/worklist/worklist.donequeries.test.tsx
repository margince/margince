// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Done on more rows than one call takes: runs of at most 200, all or none.

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
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

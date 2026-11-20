// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Setting a row aside on Home: it leaves the cached day at once.

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { homeDayOptions } from "../../lib/worklistday";
import { useSetDisposition } from "./worklist.writes";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("Hide on a Home row", () => {
  it("takes the row off the cached Home day and marks the day,", async () => {
    const client = new QueryClient({
      defaultOptions: { mutations: { retry: false } },
    });
    const key = homeDayOptions().queryKey;
    client.setQueryData(key, {
      pages: [
        {
          queue: [
            { id: "mail-1", source: "awaiting_reply" },
            { id: "mail-2", source: "awaiting_reply" },
          ],
        },
      ],
      pageParams: [undefined],
    });
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response(null, { status: 204 })),
    );
    const { result } = renderHook(() => useSetDisposition(), {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryClientProvider client={client}>{children}</QueryClientProvider>
      ),
    });
    await act(() =>
      result.current.mutateAsync({
        activityId: "mail-1",
        source: "awaiting_reply",
        disposition: "dismissed" as never,
      }),
    );
    const page = client.getQueryData<{
      pages: { queue: { id: string }[]; served_stale?: boolean }[];
    }>(key)?.pages[0];
    expect(page?.queue.map((row) => row.id)).toEqual(["mail-2"]);
    expect(page?.served_stale).toBe(true);
  });
});

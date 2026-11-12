/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useReadsTeamProgress } from "./progressreader";

const get = vi.hoisted(() => vi.fn(() => new Promise<never>(() => {})));
vi.mock("../api/client", () => ({ api: { GET: get } }));
vi.mock("./accessgrants", () => ({ grants: () => true }));

describe("useReadsTeamProgress", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    get.mockClear();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it("asks for the frame only after the screen's own reads have rested", async () => {
    const client = new QueryClient();
    let finishScreenRead: (rows: string[]) => void = () => {};
    const screenRead = client.fetchQuery({
      queryKey: ["home"],
      queryFn: () =>
        new Promise<string[]>((resolve) => {
          finishScreenRead = resolve;
        }),
    });
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
    const { result } = renderHook(() => useReadsTeamProgress(), { wrapper });

    await act(() => vi.advanceTimersByTimeAsync(2_000));
    expect(get).not.toHaveBeenCalled();
    expect(result.current).toBe(false);

    finishScreenRead([]);
    await screenRead;
    await act(() => vi.advanceTimersByTimeAsync(300));
    expect(get).toHaveBeenCalledTimes(1);
    expect(get).toHaveBeenCalledWith("/analytics/context", expect.anything());
  });
});

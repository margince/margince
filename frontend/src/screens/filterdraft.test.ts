/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  QueryClient,
  QueryClientProvider,
  useQuery,
} from "@tanstack/react-query";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { useFirstAnswer } from "./filterdraft";

afterEach(cleanup);

type Opened = Readonly<{ version: number }>;

/** An opening read gated the way a page gates it, on whether its surface is on. */
function useOpening(enabled: boolean): Opened | undefined {
  const read = useQuery({
    queryKey: ["opened"],
    enabled,
    queryFn: async (): Promise<Opened> => ({ version: 2 }),
  });
  return useFirstAnswer(read);
}

describe("the first answer an opening settles on", () => {
  it("waits out a disabled read rather than taking the copy its cache holds", async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    // Read earlier on another page; the server has moved on to version 2.
    client.setQueryData<Opened>(["opened"], { version: 1 });
    const wrapper = ({ children }: { children: ReactNode }) =>
      createElement(QueryClientProvider, { client }, children);
    const { result, rerender } = renderHook(
      ({ enabled }) => useOpening(enabled),
      { wrapper, initialProps: { enabled: false } },
    );

    expect(result.current).toBeUndefined();

    rerender({ enabled: true });

    // The version a save is held to is the one the server answered this opening.
    await waitFor(() => expect(result.current).toEqual({ version: 2 }));
  });
});

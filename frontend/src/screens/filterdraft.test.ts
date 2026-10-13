/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  QueryClient,
  QueryClientProvider,
  useQuery,
} from "@tanstack/react-query";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { useFirstAnswer } from "./filterdraft";

afterEach(cleanup);

type Opened = Readonly<{ version: number }>;

/** What the server answers the opening read with, changed between reads. */
type Server = { answer: () => Opened };

/** An opening read gated the way a page gates it, on whether its surface is on. */
function useOpening(enabled: boolean, server: Server): Opened | undefined {
  const read = useQuery({
    queryKey: ["opened"],
    enabled,
    queryFn: async (): Promise<Opened> => server.answer(),
  });
  return useFirstAnswer(read);
}

function mount(server: Server, enabled: boolean, cached?: Opened) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  if (cached) {
    client.setQueryData<Opened>(["opened"], cached);
  }
  const wrapper = ({ children }: { children: ReactNode }) =>
    createElement(QueryClientProvider, { client }, children);
  const hook = renderHook((props) => useOpening(props.enabled, server), {
    wrapper,
    initialProps: { enabled },
  });
  return { client, ...hook };
}

describe("the first answer an opening settles on", () => {
  it("waits out a disabled read rather than taking the copy its cache holds", async () => {
    // Read earlier on another page; the server has moved on to version 2.
    const { result, rerender } = mount(
      { answer: () => ({ version: 2 }) },
      false,
      { version: 1 },
    );

    expect(result.current).toBeUndefined();

    rerender({ enabled: true });

    // The version a save is held to is the one the server answered this opening.
    await waitFor(() => expect(result.current).toEqual({ version: 2 }));
  });

  it("keeps that answer while a later read answers a newer version, or fails", async () => {
    const server: Server = { answer: () => ({ version: 2 }) };
    const { client, result } = mount(server, true);
    await waitFor(() => expect(result.current).toEqual({ version: 2 }));

    server.answer = () => ({ version: 3 });
    await act(() => client.refetchQueries({ queryKey: ["opened"] }));
    expect(client.getQueryData(["opened"])).toEqual({ version: 3 });
    expect(result.current).toEqual({ version: 2 });

    server.answer = () => {
      throw new Error("The server is not answering.");
    };
    await act(() => client.refetchQueries({ queryKey: ["opened"] }));
    expect(client.getQueryState(["opened"])?.status).toBe("error");
    expect(result.current).toEqual({ version: 2 });
  });
});

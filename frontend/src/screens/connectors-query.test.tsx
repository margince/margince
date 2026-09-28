/** @vitest-environment happy-dom */
import {
  focusManager,
  QueryClient,
  QueryClientProvider,
} from "@tanstack/react-query";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { useConnectors } from "./connectors";
import { installFetchStub, jsonResponse } from "./story-utils";

afterEach(() => {
  cleanup();
  focusManager.setFocused(undefined);
  vi.unstubAllGlobals();
});

for (const refreshOnReturn of [false, true]) {
  it(`preserves the client's focus policy unless booking opts in (${refreshOnReturn})`, async () => {
    let reads = 0;
    installFetchStub({
      "GET /connectors": () => {
        reads++;
        return jsonResponse({ data: [] });
      },
    });
    const client = new QueryClient({
      defaultOptions: {
        queries: { retry: false, refetchOnWindowFocus: false },
      },
    });
    focusManager.setFocused(false);
    const { result, unmount } = renderHook(
      () =>
        useConnectors(
          refreshOnReturn ? { refetchOnWindowFocus: "always" } : undefined,
        ),
      {
        wrapper: ({ children }: { children: ReactNode }) => (
          <QueryClientProvider client={client}>{children}</QueryClientProvider>
        ),
      },
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(reads).toBe(1);
    await act(async () => {
      focusManager.setFocused(true);
    });
    await waitFor(() => expect(reads).toBe(refreshOnReturn ? 2 : 1));
    unmount();
    client.clear();
  });
}

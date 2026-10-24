/** @vitest-environment happy-dom */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { project360Key } from "./project360.key";
import {
  projectHealthKey,
  useCorrectProjectHealth,
  useRecordProjectHealth,
} from "./projecthealth.queries";

// The project page reads its verdict from the 360 payload, so a reading
// recorded or corrected must mark that exact query stale. Seeding the cache
// under the page's own key is what makes a misspelt key fail here.

const PROJECT_ID = "11111111-1111-1111-1111-111111111111";

afterEach(() => {
  vi.unstubAllGlobals();
});

function seededClient() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(project360Key(PROJECT_ID), { as_of: "seed" });
  client.setQueryData(projectHealthKey(PROJECT_ID), []);
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify({ id: "h1" }), {
          status: 201,
          headers: { "Content-Type": "application/json" },
        }),
    ),
  );
  const wrapper = ({ children }: Readonly<{ children: ReactNode }>) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  return { client, wrapper };
}

function staleAfterWrite(client: QueryClient) {
  return {
    page: client.getQueryState(project360Key(PROJECT_ID))?.isInvalidated,
    history: client.getQueryState(projectHealthKey(PROJECT_ID))?.isInvalidated,
  };
}

describe("project health writes refresh the project page", () => {
  it("recording a reading marks the 360 read stale", async () => {
    const { client, wrapper } = seededClient();
    const { result } = renderHook(() => useRecordProjectHealth(PROJECT_ID), {
      wrapper,
    });
    result.current.mutate({ state: "at_risk", note: "Feed is late." });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(staleAfterWrite(client)).toEqual({ page: true, history: true });
  });

  it("correcting a reading marks the 360 read stale", async () => {
    const { client, wrapper } = seededClient();
    const { result } = renderHook(() => useCorrectProjectHealth(PROJECT_ID), {
      wrapper,
    });
    result.current.mutate({
      assessmentId: "22222222-2222-2222-2222-222222222222",
      body: { state: "on_track" },
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(staleAfterWrite(client)).toEqual({ page: true, history: true });
  });
});

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useSetProviderKey } from "./ai-provider-key-hooks";
import { useTestProviderKey } from "./ai-provider-key-test";
import { useSetProviderSettings } from "./ai-provider-settings";

// The Providers badge reads the server's health record, which a key save, a
// settings save and a successful key test all change; each must refetch it
// instead of leaving the badge to the next poll.

const HEALTH_KEY = ["ai-provider-health"];

// The document a settings save answers with (AiRouting).
const ROUTING = {
  profile: "cloud_frontier",
  tiers: {},
  embeddings: {
    provider: "ollama",
    model: "nomic-embed-text",
    base_url: "http://localhost:11434",
    dimensions: 768,
  },
};

function setup() {
  const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    const url = input instanceof Request ? input.url : String(input);
    if (url.endsWith("/test")) {
      return new Response(
        JSON.stringify({ provider: "gemini", ok: true, model_count: 3 }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      );
    }
    // Only the settings save answers with the routing document; the key save
    // answers a bare success.
    if (url.includes("/ai/provider-settings/")) {
      return new Response(JSON.stringify(ROUTING), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }
    return new Response(null, { status: 204 });
  });
  vi.stubGlobal("fetch", fetchMock);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  client.setQueryData(HEALTH_KEY, { providers: [] });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  return { client, wrapper };
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("provider health after a change to a provider", () => {
  it("is marked stale after a key save", async () => {
    const { client, wrapper } = setup();
    const { result } = renderHook(() => useSetProviderKey(), { wrapper });

    await act(() =>
      result.current.mutateAsync({
        provider: "gemini",
        kind: "api_key",
        secret: "k",
      }),
    );

    await waitFor(() =>
      expect(client.getQueryState(HEALTH_KEY)?.isInvalidated).toBe(true),
    );
  });

  it("is marked stale after a provider settings save", async () => {
    const { client, wrapper } = setup();
    const { result } = renderHook(() => useSetProviderSettings(), { wrapper });

    await act(() =>
      result.current.mutateAsync({ provider: "gemini", settings: {} }),
    );

    await waitFor(() =>
      expect(client.getQueryState(HEALTH_KEY)?.isInvalidated).toBe(true),
    );
  });

  it("is marked stale after a key test", async () => {
    const { client, wrapper } = setup();
    const { result } = renderHook(() => useTestProviderKey(), { wrapper });

    await act(() => result.current.mutateAsync({ provider: "gemini" }));

    await waitFor(() =>
      expect(client.getQueryState(HEALTH_KEY)?.isInvalidated).toBe(true),
    );
  });
});

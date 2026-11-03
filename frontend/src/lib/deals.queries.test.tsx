/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { type DealFilters, useDeals } from "./deals.queries";

afterEach(() => {
  vi.unstubAllGlobals();
});

const noDials: DealFilters = {
  pipelineId: "pl",
  q: "",
  sort: "",
  includeArchived: false,
  filters: {},
};

describe("useDeals", () => {
  // A table the reader left must not keep its page read running.
  it("aborts its page read when the table unmounts", async () => {
    const signals: AbortSignal[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL) => {
        if (input instanceof Request && input.url.includes("/deals")) {
          signals.push(input.signal);
          return new Promise<Response>(() => {});
        }
        return Promise.resolve(Response.json({}));
      }),
    );
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
    const { unmount } = renderHook(() => useDeals(noDials), { wrapper });
    await waitFor(() => expect(signals).toHaveLength(1));

    unmount();

    expect(signals[0]?.aborted).toBe(true);
  });
});

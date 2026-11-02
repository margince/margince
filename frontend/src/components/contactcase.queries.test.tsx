/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { useContactCase } from "./contactcase.queries";

const ANA = "01919999-0000-7000-8000-00000000ana1";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function wrap() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return ({ children }: Readonly<{ children: ReactNode }>) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
}

// A contact in no case is the common contact, and the route answers 404 for it.
// That is the absence, not a failure, and the screen reads it as "no case" —
// which is why the read checks the status before unwrapping. An unwrap would
// raise the 404 as a problem and the page would show an error for a contact
// with nothing wrong with it.
it("reads a 404 as no case rather than as a failure", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response(null, { status: 404 })),
  );
  const { result } = renderHook(() => useContactCase(ANA), {
    wrapper: wrap(),
  });
  await waitFor(() => expect(result.current.isSuccess).toBe(true));
  expect(result.current.data).toBeNull();
  expect(result.current.error).toBeNull();
});

it("reads a visible case as the case", async () => {
  const body = { id: "case-1", anchor_id: ANA, members: [], pairs: [] };
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify(body), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
    ),
  );
  const { result } = renderHook(() => useContactCase(ANA), {
    wrapper: wrap(),
  });
  await waitFor(() => expect(result.current.isSuccess).toBe(true));
  expect(result.current.data?.id).toBe("case-1");
});

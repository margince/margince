/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { useApproval } from "../../lib/approvalread";
import { useApproval as useApprovalFromWorklist } from "./worklist.writes";

const ID = "01919999-0000-7000-8000-0000000appr1";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function stub() {
  const fetchMock = vi.fn(
    async () =>
      new Response(JSON.stringify({ id: ID, state: "pending" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
  );
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

// The worklist and the record page show the same approval. They asked under
// keys of their own, so two open together paid twice. Invalidating one left
// the other stale.
it("serves the worklist and the record page from one read", async () => {
  const fetchMock = stub();
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const wrapper = ({ children }: Readonly<{ children: ReactNode }>) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );

  const page = renderHook(() => useApproval(ID), { wrapper });
  const worklist = renderHook(() => useApprovalFromWorklist(ID, true), {
    wrapper,
  });

  await waitFor(() => expect(page.result.current.isSuccess).toBe(true));
  await waitFor(() => expect(worklist.result.current.isSuccess).toBe(true));

  expect(page.result.current.data?.id).toBe(ID);
  expect(worklist.result.current.data?.id).toBe(ID);
  expect(fetchMock).toHaveBeenCalledTimes(1);
});

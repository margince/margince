/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  invalidateApprovalViews,
  type PendingApprovalCount,
  untilNextExpiry,
  usePendingApprovalCount,
} from "./approvals.queries";

// Answers the count endpoint in order. Every path asked is kept, so a page
// walk would show.
function stubCount(...answers: Array<PendingApprovalCount | number>) {
  const paths: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      paths.push(
        new URL(input instanceof Request ? input.url : String(input)).pathname,
      );
      const answer = answers[Math.min(paths.length, answers.length) - 1];
      return typeof answer === "number"
        ? new Response(
            JSON.stringify({
              code: "permission_denied",
              title: "denied",
              status: answer,
            }),
            { status: answer, headers: { "Content-Type": "application/json" } },
          )
        : new Response(JSON.stringify(answer), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          });
    }),
  );
  return paths;
}

function setup() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  return { client, wrapper };
}

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("usePendingApprovalCount", () => {
  it("asks for one count and walks no page", async () => {
    const paths = stubCount({ count: 412, as_of: "2026-10-01T09:00:00Z" });
    const { wrapper } = setup();
    const { result } = renderHook(() => usePendingApprovalCount(), { wrapper });
    await waitFor(() => expect(result.current.data?.count).toBe(412));
    expect(paths).toEqual(["/v1/approvals/pending-count"]);
  });

  it("reads again when an approval is decided", async () => {
    const paths = stubCount(
      { count: 2, as_of: "2026-10-01T09:00:00Z" },
      { count: 1, as_of: "2026-10-01T09:00:05Z" },
    );
    const { client, wrapper } = setup();
    const { result } = renderHook(() => usePendingApprovalCount(), { wrapper });
    await waitFor(() => expect(result.current.data?.count).toBe(2));
    await act(() => invalidateApprovalViews(client));
    await waitFor(() => expect(result.current.data?.count).toBe(1));
    expect(paths).toHaveLength(2);
  });

  // The client clock is a day off the server's: the wait is measured from
  // as_of, so the refresh still lands just after the server's expiry.
  it("reads again when the first counted approval expires, on the server's clock", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-10-02T09:00:00Z"));
    const paths = stubCount(
      {
        count: 3,
        as_of: "2026-10-01T09:00:00Z",
        next_expiry_at: "2026-10-01T09:05:00Z",
      },
      { count: 2, as_of: "2026-10-01T09:05:01Z" },
    );
    const { wrapper } = setup();
    const { result } = renderHook(() => usePendingApprovalCount(), { wrapper });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(result.current.data?.count).toBe(3);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5 * 60_000);
    });
    expect(paths).toHaveLength(1);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1_000);
    });
    expect(paths).toHaveLength(2);
    await vi.waitFor(() => expect(result.current.data?.count).toBe(2));
  });

  it("answers no count, not zero, when the read is refused", async () => {
    stubCount(403);
    const { wrapper } = setup();
    const { result } = renderHook(() => usePendingApprovalCount(), { wrapper });
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.data).toBeUndefined();
  });
});

describe("untilNextExpiry", () => {
  it("sets no timer when nothing counted can expire", () => {
    expect(
      untilNextExpiry({ count: 4, as_of: "2026-10-01T09:00:00Z" }),
    ).toBeNull();
  });

  it("bounds a distant expiry to the longest timer the browser keeps", () => {
    expect(
      untilNextExpiry({
        count: 1,
        as_of: "2026-10-01T09:00:00Z",
        next_expiry_at: "2027-10-01T09:00:00Z",
      }),
    ).toBe(2_147_483_647);
  });
});

/** @vitest-environment happy-dom */
import { focusManager, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { createQueryClient } from "../app/queryclient";
import { useHandledForYou } from "../features/worklist/worklist.reads";
import { usePendingApprovals } from "../lib/approvals.queries";
import { useMagic } from "../lib/magic.queries";
import { proposal } from "./brief.fixtures";
import { jsonResponse, line, receipt } from "./magic.testkit";
import { automaticDateReceipt } from "./worklist.receiptreview.fixtures";

let client: ReturnType<typeof createQueryClient>;
let settled = false;
let reads: string[] = [];
function wrapper({ children }: { children: ReactNode }) {
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}
function useHome() {
  const magic = useMagic();
  const handled = useHandledForYou();
  const pending = usePendingApprovals();
  return { magic: magic.data, handled: handled.data, pending: pending.data };
}
async function advance(ms: number) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
}
beforeEach(() => {
  vi.useFakeTimers();
  focusManager.setFocused(true);
  client = createQueryClient();
  settled = false;
  reads = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const path = new URL(request.url).pathname;
      reads.push(path);
      if (path.endsWith("/approvals"))
        return jsonResponse({
          data: settled ? [] : [proposal("waiting", "Move deal to Won")],
          page: { has_more: false, next_cursor: null },
        });
      return path.endsWith("/magic")
        ? jsonResponse(
            receipt({
              needs_you: settled ? [] : [line({ lane: "needs_you" })],
            }),
          )
        : jsonResponse({
            as_of: "2026-09-13T08:00:00Z",
            truncated: false,
            receipts: [
              {
                ...automaticDateReceipt,
                review: {
                  ...automaticDateReceipt.review,
                  reversed: settled,
                  can_accept: !settled,
                  can_undo: !settled,
                },
                undo: { ...automaticDateReceipt.undo, reversed: settled },
              },
            ],
          });
    }),
  );
});
afterEach(() => {
  cleanup();
  client.clear();
  focusManager.setFocused(undefined);
  vi.useRealTimers();
  vi.unstubAllGlobals();
});
it("refreshes externally decided actions and reversed receipts in an open Home", async () => {
  const { result } = renderHook(useHome, { wrapper });
  await advance(0);
  expect(result.current.magic?.needs_you).toHaveLength(1);
  expect(result.current.handled?.receipts[0]?.review?.can_undo).toBe(true);
  settled = true;
  await advance(60_001);
  expect(result.current.magic?.needs_you).toEqual([]);
  expect(result.current.pending?.data).toEqual([]);
  expect(result.current.handled?.receipts).toHaveLength(1);
  expect(result.current.handled?.receipts[0]?.review?.reversed).toBe(true);
  expect(result.current.handled?.receipts[0]?.review?.can_undo).toBe(false);
});
it("pauses hidden reads and refreshes on return even inside the cache window", async () => {
  const { result } = renderHook(useHome, { wrapper });
  await advance(0);
  focusManager.setFocused(false);
  settled = true;
  await advance(10_000);
  expect(reads).toHaveLength(3);
  await act(async () => {
    focusManager.setFocused(true);
  });
  await advance(0);
  expect(result.current.magic?.needs_you).toEqual([]);
  expect(result.current.pending?.data).toEqual([]);
  expect(result.current.handled?.receipts[0]?.review?.reversed).toBe(true);
  focusManager.setFocused(false);
  const before = reads.length;
  await advance(120_000);
  expect(reads).toHaveLength(before);
});
it("refreshes a remounted Home even when its cache is still fresh", async () => {
  const first = renderHook(useHome, { wrapper });
  await advance(0);
  first.unmount();
  settled = true;
  const second = renderHook(useHome, { wrapper });
  await advance(0);
  expect(second.result.current.magic?.needs_you).toEqual([]);
  expect(second.result.current.handled?.receipts[0]?.review?.reversed).toBe(
    true,
  );
});

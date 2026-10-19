/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  focusManager,
  QueryClient,
  QueryClientProvider,
} from "@tanstack/react-query";
import { act, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { jsonResponse, run, stubRoutes } from "./import.testkit";
import { IMPORT_POLL_MS, useImportRun } from "./importrun";

function reads(sent: ReturnType<typeof stubRoutes>): number {
  return sent.filter((s) => s.path === `GET /imports/${run.id}`).length;
}

function mount() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return renderHook(() => useImportRun(run.id), {
    wrapper: ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    ),
  });
}

describe("reading a run", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
    focusManager.setFocused(undefined);
  });

  it("polls every 5 s while moving and stops when settled", async () => {
    let status = "validating";
    const sent = stubRoutes({
      [`GET /imports/${run.id}`]: () => jsonResponse({ ...run, status }),
    });
    mount();
    await vi.advanceTimersByTimeAsync(0);
    expect(reads(sent)).toBe(1);
    await vi.advanceTimersByTimeAsync(IMPORT_POLL_MS - 1);
    expect(reads(sent)).toBe(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(reads(sent)).toBe(2);

    status = "awaiting_approval";
    await vi.advanceTimersByTimeAsync(IMPORT_POLL_MS);
    expect(reads(sent)).toBe(3);
    await vi.advanceTimersByTimeAsync(IMPORT_POLL_MS * 3);
    expect(reads(sent)).toBe(3);
  });

  it("stops in a hidden tab", async () => {
    const sent = stubRoutes({
      [`GET /imports/${run.id}`]: () =>
        jsonResponse({ ...run, status: "running" }),
    });
    mount();
    await vi.advanceTimersByTimeAsync(0);
    act(() => focusManager.setFocused(false));
    await vi.advanceTimersByTimeAsync(IMPORT_POLL_MS * 3);
    expect(reads(sent)).toBe(1);
  });

  it("refetches on return", async () => {
    const sent = stubRoutes({
      [`GET /imports/${run.id}`]: () =>
        jsonResponse({ ...run, status: "awaiting_approval" }),
    });
    mount();
    await vi.advanceTimersByTimeAsync(0);
    act(() => focusManager.setFocused(false));
    act(() => focusManager.setFocused(true));
    await vi.advanceTimersByTimeAsync(0);
    expect(reads(sent)).toBe(2);
  });
});

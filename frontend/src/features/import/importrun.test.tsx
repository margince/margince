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
import { SHOWN_REFUSALS } from "../../lib/problem";
import { jsonResponse, run, stubRoutes } from "./import.testkit";
import {
  IMPORT_POLL_MS,
  IMPORTS_KEY,
  useImportReport,
  useImportRun,
} from "./importrun";
import type { ImportRun } from "./importtypes";

function reads(sent: ReturnType<typeof stubRoutes>): number {
  return sent.filter((s) => s.path === `GET /imports/${run.id}`).length;
}

function mountWith<T>(hook: () => T) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return renderHook(hook, {
    wrapper: ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    ),
  });
}

function mount() {
  return mountWith(() => useImportRun(run.id));
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

describe("reading a run's report", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  async function reportReads(status: ImportRun["status"]): Promise<number> {
    const sent = stubRoutes();
    mountWith(() => useImportReport(run.id, status, run.version));
    await vi.advanceTimersByTimeAsync(0);
    return sent.filter((s) => s.path === `GET /imports/${run.id}/report`)
      .length;
  }

  it("asks for no report of a draft, a test being read or a cancelled run", async () => {
    expect(await reportReads("pending")).toBe(0);
    expect(await reportReads("validating")).toBe(0);
    expect(await reportReads("cancelled")).toBe(0);
  });

  it("asks for the report of a tested run", async () => {
    expect(await reportReads("awaiting_approval")).toBe(1);
  });
});

describe("a run read that names no run", () => {
  it("names 404 and 422 as refusals its page shows, not failures to log", () => {
    stubRoutes();
    const client = new QueryClient();
    renderHook(() => useImportRun(run.id), {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryClientProvider client={client}>{children}</QueryClientProvider>
      ),
    });
    const read = client
      .getQueryCache()
      .find({ queryKey: [IMPORTS_KEY, "run", run.id] });
    expect(read?.meta?.[SHOWN_REFUSALS]).toEqual([404, 422]);
    client.clear();
  });
});

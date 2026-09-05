/** @vitest-environment jsdom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { type AccountScan, useAccountScan } from "./accountscan";
import { jsonResponse } from "./company.fixtures";

// The scan is asked for once on open and polled only while a read runs. The
// server decides whether an open costs a model call; the page's part is to
// ask exactly once and to stop asking the moment the read has settled.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

function scan(state: AccountScan["state"]): AccountScan {
  return { organization_id: "o-1", state, findings: [], findings_dropped: 0 };
}

function Probe({ enabled }: Readonly<{ enabled: boolean }>) {
  const held = useAccountScan("o-1", enabled);
  return <output>{held?.state ?? "none"}</output>;
}

function mount(enabled: boolean) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <Probe enabled={enabled} />
    </QueryClientProvider>,
  );
}

// Answers the ensure with a queued read, then each poll in turn from the
// states handed in, recording what was asked.
function stubScan(polls: AccountScan["state"][]) {
  const calls: string[] = [];
  const remaining = [...polls];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = new Request(input, init);
      calls.push(`${request.method} ${new URL(request.url).pathname}`);
      if (request.method === "POST") {
        return jsonResponse(scan("queued"));
      }
      return jsonResponse(scan(remaining.shift() ?? "done"));
    }),
  );
  return calls;
}

describe("useAccountScan", () => {
  it("asks once on open and takes the answer as the scan", async () => {
    const calls = stubScan([]);
    mount(true);
    await waitFor(() =>
      expect(screen.getByRole("status").textContent).toBe("queued"),
    );
    expect(calls.filter((call) => call.startsWith("POST"))).toEqual([
      "POST /v1/organizations/o-1/scan",
    ]);
  });

  it("polls while the read runs and stops once it has settled", async () => {
    vi.useFakeTimers();
    const calls = stubScan(["running", "done", "done"]);
    mount(true);
    const flush = () =>
      act(async () => {
        await vi.advanceTimersByTimeAsync(1);
      });
    await flush();
    await flush();
    expect(screen.getByRole("status").textContent).toBe("queued");

    // Each poll interval brings the next state; the poll ends on "done".
    await act(async () => {
      await vi.advanceTimersByTimeAsync(3000);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(3000);
    });
    expect(screen.getByRole("status").textContent).toBe("done");
    const settledPolls = calls.filter((call) => call.startsWith("GET")).length;
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10_000);
    });
    expect(calls.filter((call) => call.startsWith("GET")).length).toBe(
      settledPolls,
    );
  });

  it("holds no scan when the server refuses, rather than a broken one", async () => {
    const calls: string[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const request = new Request(input, init);
        calls.push(`${request.method} ${new URL(request.url).pathname}`);
        return new Response(
          JSON.stringify({ title: "Unavailable", code: "internal" }),
          {
            status: 500,
            headers: { "content-type": "application/problem+json" },
          },
        );
      }),
    );
    mount(true);
    await waitFor(() =>
      expect(calls).toContain("POST /v1/organizations/o-1/scan"),
    );
    await waitFor(() =>
      expect(calls).toContain("GET /v1/organizations/o-1/scan"),
    );
    await act(async () => {});
    expect(screen.getByRole("status").textContent).toBe("none");
  });

  it("asks nothing where the page cannot show it", async () => {
    const calls = stubScan([]);
    mount(false);
    await act(async () => {});
    expect(calls).toEqual([]);
    expect(screen.getByRole("status").textContent).toBe("none");
  });
});

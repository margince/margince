/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { jsonResponse } from "../../testing/company.fixtures";
import { type AccountScan, useAccountScan } from "./accountscan";

// The scan is asked for once on open and polled only while a read runs. The
// server decides whether an open costs a model call; the page's part is to
// ask exactly once and to stop asking the moment the read has settled.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

function scan(state: AccountScan["state"]): AccountScan {
  return { company_id: "o-1", state, findings: [], findings_dropped: 0 };
}

function Probe({ enabled }: Readonly<{ enabled: boolean }>) {
  const held = useAccountScan("o-1", enabled);
  return <output>{held?.state ?? "none"}</output>;
}

// Two consumers of one account in one cache, which is what a record page with
// both the tasks panel and a scan-reading sibling is.
function mountTwo() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <Probe enabled />
      <Probe enabled />
    </QueryClientProvider>,
  );
}

// One consumer, then a second into the same cache once the first has settled,
// which is a sibling panel opening after the scan came back.
function mountThenMountAgain() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const first = render(
    <QueryClientProvider client={client}>
      <Probe enabled />
    </QueryClientProvider>,
  );
  return {
    second: () =>
      first.rerender(
        <QueryClientProvider client={client}>
          <Probe enabled />
          <Probe enabled />
        </QueryClientProvider>,
      ),
  };
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
      "POST /v1/companies/o-1/scan",
    ]);
  });

  it("asks no second time for a consumer that opens after the answer", async () => {
    const calls = stubScan(["queued"]);
    const { second } = mountThenMountAgain();
    await waitFor(() =>
      expect(screen.getByRole("status").textContent).toBe("queued"),
    );
    expect(calls.filter((call) => call.startsWith("POST"))).toHaveLength(1);

    // The first ensure has settled, so nothing is in flight to see. What
    // holds the second consumer is the answer left in the cache.
    second();
    await waitFor(() => expect(screen.getAllByRole("status")).toHaveLength(2));
    expect(calls.filter((call) => call.startsWith("POST"))).toHaveLength(1);
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
    await waitFor(() => expect(calls).toContain("POST /v1/companies/o-1/scan"));
    await waitFor(() => expect(calls).toContain("GET /v1/companies/o-1/scan"));
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

  it("asks the server once when two consumers read one account", async () => {
    const calls = stubScan([]);
    mountTwo();
    await waitFor(() =>
      expect(screen.getAllByRole("status")[0]?.textContent).toBe("queued"),
    );
    // The ensure is the expensive half: it is what can cost a model call. Two
    // consumers of one account are one question about it.
    expect(calls.filter((call) => call.startsWith("POST"))).toEqual([
      "POST /v1/companies/o-1/scan",
    ]);
  });
});

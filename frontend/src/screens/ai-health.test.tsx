// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { AiHealthCard } from "./ai-health";

// The distinction this card exists to draw: a lane nobody called and a lane
// that answered nothing are both "no successful calls", and reporting them the
// same way is what makes an outage invisible.

type RungHealth = components["schemas"]["AiRungHealth"];

const ANSWERING: RungHealth = {
  tier: "local_small",
  healthy: true,
  calls: 12,
  failures: 0,
  median_latency_ms: 240,
  last_call_at: "2026-09-01T09:12:00Z",
};

const DEAD: RungHealth = {
  tier: "cloud_large",
  healthy: false,
  calls: 5,
  failures: 5,
  median_latency_ms: 30,
  last_sentinel: "provider_unavailable",
  last_call_at: "2026-09-01T09:10:00Z",
};

function renderCard(rungs: RungHealth[], windowHours = 1) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://test.local",
      );
      const method = request?.method ?? init?.method ?? "GET";
      const key = `${method} ${url.pathname.replace(/^\/v1/, "")}`;
      const body =
        key === "GET /me"
          ? meFixture({})
          : { window_hours: windowHours, rungs };
      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const ui: ReactNode = (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <AiHealthCard />
      </LocaleProvider>
    </QueryClientProvider>
  );
  return render(ui);
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("model lane health", () => {
  it("tells a lane that answered from one that answered nothing", async () => {
    renderCard([ANSWERING, DEAD]);
    expect(await screen.findByText(/^Answering$/)).toBeInTheDocument();
    expect(screen.getByText(/^Not answering$/)).toBeInTheDocument();
  });

  it("names the error a failing lane reported", async () => {
    // The sentinel is the operator's first clue: a budget refusal and an
    // unreachable model are both "not answering" and want different fixes.
    renderCard([DEAD]);
    expect(await screen.findByText("provider_unavailable")).toBeInTheDocument();
  });

  it("says both counts, so a red badge is not left to be interpreted", async () => {
    renderCard([DEAD]);
    expect(await screen.findByText(/5 calls, 5 failed/)).toBeInTheDocument();
  });

  it("reports an unused installation as unused rather than as an outage", async () => {
    // Nobody called a model this hour. That is not a failure, and an empty
    // table would read as one.
    renderCard([]);
    expect(
      await screen.findByText(/no model was called in the last 1 hour/i),
    ).toBeInTheDocument();
  });
});

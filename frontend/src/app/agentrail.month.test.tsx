/** @vitest-environment happy-dom */

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { currentMonth } from "../features/ai/aiusage";
import { LocaleProvider } from "../i18n";
import { ViewerZoneProvider } from "../lib/viewerzone";
import { meFixture } from "../testing/mefixture";
import { AgentRail } from "./agentrail";

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "content-type": "application/json" },
  });
}

// The rail and the usage screen name one month, and only the reader knows
// their zone.
//
// Left to the server the rail reads the installation's UTC month. In the first
// hours of a month east of UTC the two then show different figures.
describe("the agent rail's month", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it("is the month the usage screen names", async () => {
    // Half an hour before December in UTC, and already December in Tokyo. The
    // two readings differ here, so a rail that read UTC's month would ask for
    // November.
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date("2026-11-30T23:30:00Z"));
    const readerZone = "Asia/Tokyo";
    const month = currentMonth(readerZone);
    expect(month.from).toBe("2026-12-01");
    expect(currentMonth("UTC").from).toBe("2026-11-01");

    const summaryQueries: string[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        const url = new URL(request.url);
        if (url.pathname.endsWith("/ai/usage/summary")) {
          summaryQueries.push(url.search);
          return jsonResponse({
            currency: "USD",
            computed_at: "2026-11-03T00:00:00Z",
          });
        }
        if (url.pathname.endsWith("/me")) {
          return jsonResponse(
            meFixture({ allow: { ai_diagnostics: ["read"] } }),
          );
        }
        if (url.pathname.endsWith("/me/ai-activity")) {
          return jsonResponse({ running: [], recent: [] });
        }
        return jsonResponse({ data: [] });
      }),
    );

    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    render(
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <ViewerZoneProvider zone={readerZone}>
            <AgentRail />
          </ViewerZoneProvider>
        </LocaleProvider>
      </QueryClientProvider>,
    );

    await waitFor(() => expect(summaryQueries.length).toBeGreaterThan(0));
    const asked = new URLSearchParams(summaryQueries[0]);
    expect(asked.get("from")).toBe(month.from);
    expect(asked.get("to")).toBe(month.to);
  });
});

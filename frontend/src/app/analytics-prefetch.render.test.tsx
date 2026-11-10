/** @vitest-environment happy-dom */
import { QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { AnalyticsScreen } from "../features/analytics/analytics";
import {
  chooseContext,
  rememberView,
} from "../features/analytics/analytics.overview.context";
import type { Seat } from "../features/analytics/analytics.overview.fixtures";
import { overviewRoutes } from "../features/analytics/analytics.overview.routes.fixtures";
import { bySeat } from "../features/analytics/analytics.overview.testkit";
import { LocaleProvider } from "../i18n";
import { CompanyZoneProvider } from "../lib/companyzone";
import { forgetSeat } from "../lib/storage";
import { REPORTING_FIXTURE_ZONE } from "../testing/reporting.fixtures";
import { installFetchStub, jsonResponse } from "../testing/story-utils";
import { prefetchAnalytics } from "./analytics-prefetch";
import { createQueryClient } from "./queryclient";

const originalFetch = globalThis.fetch;
const clients: ReturnType<typeof createQueryClient>[] = [];
beforeEach(() => {
  forgetSeat();
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-09-22T12:00:00Z"));
});
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => {
    client.clear();
  });
  globalThis.fetch = originalFetch;
  location.hash = "";
  forgetSeat();
  vi.useRealTimers();
});

function start(address: string, seat: Seat = "leader") {
  location.hash = address;
  installFetchStub({
    ...overviewRoutes({ seat }),
    "GET /installation/settings": () =>
      jsonResponse({ timezone: REPORTING_FIXTURE_ZONE }),
  });
  const routed = globalThis.fetch;
  const evaluations: URL[] = [];
  globalThis.fetch = async (input, init) => {
    const url = new URL(input instanceof Request ? input.url : String(input));
    if (url.pathname === "/v1/analytics/evaluate") {
      evaluations.push(url);
      return jsonResponse(bySeat(seat)(url));
    }
    return routed(input, init);
  };
  const client = createQueryClient();
  clients.push(client);
  return { client, evaluations };
}

it.each([
  ["sales", "leader"],
  ["prospecting", "sdr"],
  ["team", "leader"],
] as const)(
  "%s joins the evaluation prepared before rendering",
  async (view, seat) => {
    const { client, evaluations } = start(
      `#/analytics/overview?overview=${view}`,
      seat,
    );
    await prefetchAnalytics(client);
    expect(evaluations).toHaveLength(1);
    const prepared = evaluations[0].href;
    const query = client
      .getQueryCache()
      .findAll({ queryKey: ["reporting-evaluation"] })[0];
    if (!query) throw new Error("Startup did not prepare its evaluation");
    expect(query.getObserversCount()).toBe(0);
    await act(async () => {
      render(
        <QueryClientProvider client={client}>
          <LocaleProvider initial="en">
            <CompanyZoneProvider zone={REPORTING_FIXTURE_ZONE}>
              <AnalyticsScreen />
            </CompanyZoneProvider>
          </LocaleProvider>
        </QueryClientProvider>,
      );
    });
    await waitFor(() => expect(query.getObserversCount()).toBeGreaterThan(0));
    expect(evaluations.filter((url) => url.href === prepared)).toHaveLength(1);
  },
);

it.each([
  "scope=team%3Aunavailable",
  "pipeline=unavailable",
  "period=custom&from=bad&through=2026-10-01",
])("does not substitute a different question for %s", async (dials) => {
  const { client, evaluations } = start(
    `#/analytics/overview?overview=sales&${dials}`,
  );
  await prefetchAnalytics(client);
  expect(evaluations).toHaveLength(0);
});

it("restores the account's kept question through the real memory writer", async () => {
  rememberView("00000000-0000-4000-8000-000000000001", "sales");
  chooseContext("00000000-0000-4000-8000-000000000001", "sales", {
    period: "last_week",
    pipeline: "all",
    country: "DE",
  });
  const { client, evaluations } = start("#/analytics/overview?overview=sales");
  await prefetchAnalytics(client);
  expect(evaluations).toHaveLength(1);
  expect(evaluations[0].searchParams.get("period")).toBe("last_week");
  expect(evaluations[0].searchParams.get("pipeline_id")).toBeNull();
  expect(evaluations[0].searchParams.get("country")).toBe("DE");
});

it("uses the configured company clock for a custom date range", async () => {
  const { client, evaluations } = start(
    "#/analytics/overview?overview=sales&period=custom&from=2026-10-01&through=2026-10-15",
  );
  await prefetchAnalytics(client);
  expect(evaluations).toHaveLength(1);
  expect(evaluations[0].searchParams.get("start_at")).toBe(
    "2026-09-30T22:00:00.000Z",
  );
  expect(evaluations[0].searchParams.get("end_at")).toBe(
    "2026-10-15T22:00:00.000Z",
  );
});

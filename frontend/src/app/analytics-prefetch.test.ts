/** @vitest-environment happy-dom */
import { QueryClient } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { analyticsContextOptions } from "../features/analytics/analytics.context";
import { reportingCatalogOptions } from "../features/analytics/reporting.queries";
import { stable } from "../format/collate";
import { sessionOptions } from "../lib/session";
import { meFixture } from "../testing/mefixture";
import { prefetchAnalytics } from "./analytics-prefetch";

const clients: QueryClient[] = [];
afterEach(() => {
  for (const value of clients.splice(0)) value.clear();
  vi.restoreAllMocks();
  location.hash = "";
});
function client() {
  const value = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  clients.push(value);
  return value;
}

it("shares the authenticated reads with the screen's query options", async () => {
  location.hash = "#/analytics/performance";
  const me = meFixture({
    allow: {
      report_definition: ["read"],
      reporting_framework: ["read"],
      deal: ["read"],
    },
  });
  const paths: string[] = [];
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const request = input instanceof Request ? input : new Request(input);
    const path = new URL(request.url).pathname;
    paths.push(path);
    return Response.json(path.endsWith("/me") ? me : { data: [] });
  });
  const cache = client();
  await prefetchAnalytics(cache);
  expect(paths.sort(stable)).toEqual([
    "/v1/analytics/context",
    "/v1/analytics/framework",
    "/v1/analytics/metrics",
    "/v1/company",
    "/v1/company/context/capabilities",
    "/v1/installation/settings",
    "/v1/me",
    "/v1/pipelines",
  ]);
  await cache.fetchQuery(sessionOptions);
  await cache.fetchQuery(analyticsContextOptions);
  await cache.fetchQuery(reportingCatalogOptions);
  expect(paths).toHaveLength(8);
});

it("does not read protected catalogs, pipelines or company for an ungranted seat", async () => {
  location.hash = "#/analytics/overview";
  const cache = client();
  cache.setQueryData(sessionOptions.queryKey, meFixture({ roles: ["rep"] }));
  const paths: string[] = [];
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    paths.push(
      new URL(input instanceof Request ? input.url : String(input)).pathname,
    );
    return Response.json({ data: [] });
  });
  await prefetchAnalytics(cache);
  expect(paths.sort(stable)).toEqual([
    "/v1/analytics/context",
    "/v1/company/context/capabilities",
    "/v1/installation/settings",
  ]);
});

it.each(["signed out", "left Analytics"])(
  "stops early when %s",
  async (state) => {
    location.hash =
      state === "signed out" ? "#/analytics/overview" : "#/contacts";
    const cache = client();
    if (state === "left Analytics")
      cache.setQueryData(sessionOptions.queryKey, meFixture());
    const fetched = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(Response.json({ detail: "Sign in" }, { status: 401 }));
    await prefetchAnalytics(cache);
    expect(fetched).toHaveBeenCalledTimes(state === "signed out" ? 1 : 0);
  },
);

it("does not fetch the full company profile when the session already names it", async () => {
  location.hash = "#/analytics/performance";
  const cache = client();
  cache.setQueryData(sessionOptions.queryKey, {
    ...meFixture(),
    installation_brand: { display_name: "Demo company" },
  });
  const paths: string[] = [];
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    paths.push(
      new URL(input instanceof Request ? input.url : String(input)).pathname,
    );
    return Response.json({ data: [] });
  });
  await prefetchAnalytics(cache);
  expect(paths).toContain("/v1/analytics/context");
  expect(paths).not.toContain("/v1/company");
});

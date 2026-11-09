/** @vitest-environment happy-dom */
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { analyticsContextOptions } from "../features/analytics/analytics.context";
import {
  reportingCatalogOptions,
  reportingFrameworkOptions,
} from "../features/analytics/reporting.queries";
import {
  COMPANY_CONTEXT_ON,
  INSTALLATION_SETTINGS,
} from "../features/settings/settingspages.fixtures";
import { stable } from "../format/collate";
import { companyContextCapabilitiesOptions } from "../lib/companycontextreach";
import { pipelinesOptions } from "../lib/pipelines.queries";
import { SESSION_STALE_MS, sessionOptions } from "../lib/session";
import { readPreparedAnalyticsSession } from "../lib/session.read";
import { installationSettingsOptions } from "../lib/uploadlimit";
import { meFixture } from "../testing/mefixture";
import {
  interval,
  reportingStoryCatalog,
  reportingStoryFramework,
  reportingStoryScope,
} from "../testing/reporting.fixtures";
import { prefetchAnalytics } from "./analytics-prefetch";
import { createQueryClient } from "./queryclient";

const clients: ReturnType<typeof createQueryClient>[] = [];
const startup: components["schemas"]["AnalyticsStartup"] = {
  context: {
    default_scope: reportingStoryScope,
    allowed_scopes: [reportingStoryScope],
    as_of: interval.end_at,
    timezone: INSTALLATION_SETTINGS.timezone,
    base_currency: "EUR",
    capabilities: {
      view_manager_forecast: true,
      submit_manager_forecast: false,
    },
  },
  catalog: reportingStoryCatalog,
  framework: reportingStoryFramework,
  installation_settings: INSTALLATION_SETTINGS,
  company_context: COMPANY_CONTEXT_ON,
  pipelines: [],
};
const me = {
  ...meFixture({
    allow: {
      report_definition: ["read"],
      reporting_framework: ["read"],
      deal: ["read"],
    },
  }),
  installation_brand: { display_name: "Demo company" },
  analytics_startup: startup,
};
beforeEach(() => {
  location.hash = "#/analytics/performance";
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-09-22T12:00:00Z"));
});
afterEach(() => {
  clients.splice(0).forEach((client) => {
    client.clear();
  });
  vi.restoreAllMocks();
  vi.useRealTimers();
  location.hash = "";
});
function open() {
  const client = createQueryClient();
  clients.push(client);
  const requested: URL[] = [];
  const answers: Record<string, unknown> = {
    "/v1/me": me,
    "/v1/analytics/context": startup.context,
    "/v1/analytics/metrics": startup.catalog,
    "/v1/analytics/framework": startup.framework,
    "/v1/installation/settings": startup.installation_settings,
    "/v1/company/context/capabilities": startup.company_context,
    "/v1/pipelines": { data: [] },
  };
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const url = new URL(input instanceof Request ? input.url : String(input));
    requested.push(url);
    if (!(url.pathname in answers))
      throw new Error(`Unexpected read: ${url.pathname}`);
    return Response.json(answers[url.pathname]);
  });
  return { client, requested, answers };
}
it("opens from the optional session data without repeating its reads", async () => {
  const { client, requested } = open();
  await prefetchAnalytics(client);
  expect(requested.map((url) => url.pathname)).toEqual(["/v1/me"]);
  expect(requested[0].searchParams.get("prepare")).toBe("analytics");
  expect(client.getQueryData(analyticsContextOptions.queryKey)).toEqual(
    startup.context,
  );
  expect(client.getQueryData(reportingCatalogOptions.queryKey)).toEqual(
    startup.catalog,
  );
  expect(client.getQueryData(reportingFrameworkOptions.queryKey)).toEqual(
    startup.framework,
  );
  expect(client.getQueryData(installationSettingsOptions.queryKey)).toEqual(
    startup.installation_settings,
  );
  expect(
    client.getQueryData(companyContextCapabilitiesOptions.queryKey),
  ).toEqual(startup.company_context);
  expect(client.getQueryData(pipelinesOptions.queryKey)).toEqual([]);
});
it("fetches an optional catalog missing from the prepared response", async () => {
  const { client, requested } = open();
  const partial = { ...startup };
  delete partial.pipelines;
  client.setQueryData(sessionOptions.queryKey, {
    ...me,
    analytics_startup: partial,
  });
  await prefetchAnalytics(client);
  expect(requested.map((url) => url.pathname)).toEqual(["/v1/pipelines"]);
  expect(client.getQueryData(pipelinesOptions.queryKey)).toEqual([]);
});
it("keeps newer cache data and does not make an old session snapshot fresh", async () => {
  const { client, requested } = open();
  const now = Date.now();
  client.setQueryData(sessionOptions.queryKey, me, { updatedAt: now - 60_000 });
  const newer = { ...startup.context, base_currency: "USD" };
  client.setQueryData(analyticsContextOptions.queryKey, newer, {
    updatedAt: now - 1_000,
  });
  await prefetchAnalytics(client);
  expect(client.getQueryData(analyticsContextOptions.queryKey)).toEqual(newer);
  expect(
    client.getQueryState(analyticsContextOptions.queryKey)?.dataUpdatedAt,
  ).toBe(now - 1_000);
  expect(requested.map((url) => url.pathname).sort(stable)).toEqual([
    "/v1/analytics/framework",
    "/v1/analytics/metrics",
    "/v1/company/context/capabilities",
    "/v1/installation/settings",
    "/v1/pipelines",
  ]);
  expect(
    client.getQueryState(installationSettingsOptions.queryKey)?.dataUpdatedAt,
  ).toBe(now);
});

it("preserves the receipt time when deferred JavaScript delivers a prepared session", async () => {
  const { client, requested } = open();
  const receivedAt = Date.now();
  const snapshot = await readPreparedAnalyticsSession();
  expect(snapshot.dataUpdatedAt).toBe(receivedAt);
  vi.setSystemTime(receivedAt + 60_000);
  await prefetchAnalytics(client, async () => snapshot);
  expect(client.getQueryState(sessionOptions.queryKey)?.dataUpdatedAt).toBe(
    receivedAt,
  );
  expect(requested.filter((url) => url.pathname === "/v1/me")).toHaveLength(1);
  expect(
    requested.some((url) => url.pathname === "/v1/analytics/metrics"),
  ).toBe(true);
  expect(
    client.getQueryState(reportingCatalogOptions.queryKey)?.dataUpdatedAt,
  ).toBe(Date.now());
});

it("refreshes a prepared session that expired while its JavaScript loaded", async () => {
  const { client, requested, answers } = open();
  const receivedAt = Date.now();
  answers["/v1/me"] = {
    ...me,
    user: { ...me.user, display_name: "Old snapshot" },
  };
  const snapshot = await readPreparedAnalyticsSession();
  answers["/v1/me"] = me;
  vi.setSystemTime(receivedAt + SESSION_STALE_MS + 1);
  await prefetchAnalytics(client, async () => snapshot);
  expect(requested.map((url) => url.pathname)).toEqual(["/v1/me", "/v1/me"]);
  expect(client.getQueryData(sessionOptions.queryKey)?.user.display_name).toBe(
    me.user.display_name,
  );
  expect(client.getQueryState(sessionOptions.queryKey)?.dataUpdatedAt).toBe(
    Date.now(),
  );
});

it("keeps a newer session update made while the prepared response is published", async () => {
  const { client } = open();
  const snapshot = await readPreparedAnalyticsSession();
  vi.setSystemTime(Date.now() + 1_000);
  const updatedAt = Date.now();
  const unsubscribe = client.getQueryCache().subscribe((event) => {
    if (
      event.type === "updated" &&
      event.query.queryKey[0] === "me" &&
      event.action.type === "success" &&
      !event.action.manual
    ) {
      client.setQueryData(
        sessionOptions.queryKey,
        { ...me, user: { ...me.user, display_name: "Newer session" } },
        { updatedAt },
      );
    }
  });
  try {
    await prefetchAnalytics(client, async () => snapshot);
  } finally {
    unsubscribe();
  }
  expect(client.getQueryData(sessionOptions.queryKey)?.user.display_name).toBe(
    "Newer session",
  );
  expect(client.getQueryState(sessionOptions.queryKey)?.dataUpdatedAt).toBe(
    updatedAt,
  );
});

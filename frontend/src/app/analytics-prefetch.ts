import type { DataTag, QueryClient, QueryKey } from "@tanstack/react-query";
import { analyticsContextOptions } from "../features/analytics/analytics.context";
import { prefetchOverview } from "../features/analytics/analytics.overview.prefetch";
import {
  reportingCatalogOptions,
  reportingFrameworkOptions,
} from "../features/analytics/reporting.queries";
import { grants } from "../lib/accessgrants";
import { holdsAdminRole } from "../lib/capability";
import { companyContextCapabilitiesOptions } from "../lib/companycontextreach";
import { companyOptions, sessionHasCompany } from "../lib/installationcompany";
import { pipelinesOptions } from "../lib/pipelines.queries";
import { parseHash } from "../lib/router";
import { readSession, sessionOptions } from "../lib/session";
import { installationSettingsOptions } from "../lib/uploadlimit";
import { currentParams } from "../lib/urlstate";

export async function prefetchAnalytics(client: QueryClient): Promise<void> {
  await client.prefetchQuery({
    ...sessionOptions,
    queryFn: ({ signal }) => readSession(signal, "analytics"),
  });
  const me = client.getQueryData(sessionOptions.queryKey);
  if (!me || parseHash(location.hash).screen !== "analytics") return;
  const startup = me.analytics_startup;
  const updatedAt = client.getQueryState(
    sessionOptions.queryKey,
  )?.dataUpdatedAt;
  if (startup && updatedAt !== undefined) {
    const seed = <T>(
      options: { queryKey: DataTag<QueryKey, T, unknown> },
      value: T,
    ) => {
      if (!client.getQueryState(options.queryKey))
        client.setQueryData(options.queryKey, value, { updatedAt });
    };
    seed(analyticsContextOptions, startup.context);
    seed(reportingCatalogOptions, startup.catalog);
    seed(reportingFrameworkOptions, startup.framework);
    seed(installationSettingsOptions, startup.installation_settings);
    seed(companyContextCapabilitiesOptions, startup.company_context);
    if (startup.pipelines !== undefined)
      seed(pipelinesOptions, startup.pipelines);
  }
  const requests = [
    client.prefetchQuery(analyticsContextOptions),
    client.prefetchQuery(installationSettingsOptions),
  ];
  if (grants(me, "report_definition", "read"))
    requests.push(client.prefetchQuery(reportingCatalogOptions));
  if (grants(me, "deal", "read"))
    requests.push(client.prefetchQuery(pipelinesOptions));
  if (grants(me, "reporting_framework", "read"))
    requests.push(client.prefetchQuery(reportingFrameworkOptions));
  requests.push(client.prefetchQuery(companyContextCapabilitiesOptions));
  if (holdsAdminRole(me.roles) && !sessionHasCompany(me))
    requests.push(client.prefetchQuery(companyOptions));
  const prepare = () => {
    const route = parseHash(location.hash);
    return route.screen === "analytics"
      ? prefetchOverview(client, me, route.id, currentParams())
      : Promise.resolve();
  };
  await Promise.all(requests.map((request) => request.then(prepare)));
}

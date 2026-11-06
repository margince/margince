import type { QueryClient } from "@tanstack/react-query";
import { analyticsContextOptions } from "../features/analytics/analytics.context";
import {
  reportingCatalogOptions,
  reportingFrameworkOptions,
} from "../features/analytics/reporting.queries";
import { grants } from "../lib/accessgrants";
import { holdsAdminRole } from "../lib/capability";
import { companyContextCapabilitiesOptions } from "../lib/companycontextreach";
import { companyOptions } from "../lib/installationcompany";
import { pipelinesOptions } from "../lib/pipelines.queries";
import { parseHash } from "../lib/router";
import { sessionOptions } from "../lib/session";
import { installationSettingsOptions } from "../lib/uploadlimit";

export async function prefetchAnalytics(client: QueryClient): Promise<void> {
  await client.prefetchQuery(sessionOptions);
  const me = client.getQueryData(sessionOptions.queryKey);
  if (!me || parseHash(location.hash).screen !== "analytics") return;
  const requests = [
    client.prefetchQuery(analyticsContextOptions),
    client.prefetchQuery(installationSettingsOptions),
    client.prefetchQuery(companyContextCapabilitiesOptions),
  ];
  if (holdsAdminRole(me.roles))
    requests.push(client.prefetchQuery(companyOptions));
  if (grants(me, "report_definition", "read"))
    requests.push(client.prefetchQuery(reportingCatalogOptions));
  if (grants(me, "reporting_framework", "read"))
    requests.push(client.prefetchQuery(reportingFrameworkOptions));
  if (grants(me, "deal", "read"))
    requests.push(client.prefetchQuery(pipelinesOptions));
  await Promise.all(requests);
}

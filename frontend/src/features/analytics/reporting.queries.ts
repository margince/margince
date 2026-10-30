import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { reloadReportingFigures } from "../../lib/reportingfigures";
import {
  type MetricNames,
  type ReportingSelection,
  reportingQuery,
} from "./reporting.model";

export { reloadReportingFigures };

// The workspace's reporting framework, under one key. The landing tab and the
// framework editor read one answer, and one publish refreshes both.
export function useReportingFramework(enabled = true) {
  return useQuery({
    queryKey: ["reporting-framework"],
    enabled,
    queryFn: async () => {
      return unwrap(await api.GET("/analytics/framework"));
    },
  });
}

/**
 * One evaluation, keyed by the whole selection. A dial that changes the answer
 * changes the key, so an answer is never drawn under another selection's
 * heading. The signal reaches the request, so a selection the reader has
 * already left stops on the server.
 */
export function useReportingEvaluation(
  selection: ReportingSelection,
  enabled = true,
) {
  return useQuery({
    enabled,
    queryKey: ["reporting-evaluation", selection],
    queryFn: async ({ signal }) =>
      unwrap(
        await api.GET("/analytics/evaluate", {
          params: { query: reportingQuery(selection) },
          signal,
        }),
      ),
  });
}

// The metric catalog this seat may read, under one query key so every surface
// that reads it shares one answer.
export const reportingCatalogKey = ["reporting-catalog"];

export function useReportingCatalog(enabled = true) {
  return useQuery({
    queryKey: reportingCatalogKey,
    enabled,
    queryFn: async () => {
      return unwrap(await api.GET("/analytics/metrics"));
    },
  });
}

// The catalog's name for each metric, the one the CSV export writes too. It is
// blank until the catalog answers. A metric the catalog does not offer this
// reader, retired or not readable, is named as unavailable and never by id.
export function useMetricNames(): MetricNames {
  const t = useT();
  const catalog = useReportingCatalog();
  return (metric) => {
    if (catalog.isPending) return "";
    return (
      catalog.data?.metrics.find((definition) => definition.id === metric)
        ?.label ?? t("reporting.metricUnavailable")
    );
  };
}

function firstPage(): string | undefined {
  return undefined;
}

// useReportDetailQueries reads what the report page shows: the definition, its
// live evaluation or one frozen edition, the edition history and the schedules.
export function useReportDetailQueries(
  reportId: string,
  editionId: string | undefined,
  can: Readonly<{ editions: boolean; schedules: boolean }>,
  // An editor's draft, previewed in place of the published revision.
  revision?: number,
) {
  const report = useQuery({
    queryKey: ["reporting-report", reportId],
    queryFn: async () => {
      return unwrap(
        await api.GET("/analytics/reports/{id}", {
          params: { path: { id: reportId } },
        }),
      );
    },
  });
  const live = useQuery({
    enabled: !editionId,
    queryKey: ["reporting-live", reportId, revision ?? "published"],
    queryFn: async () => {
      return unwrap(
        await api.GET("/analytics/reports/{id}/evaluation", {
          params: {
            path: { id: reportId },
            query: revision === undefined ? undefined : { revision },
          },
        }),
      );
    },
  });
  const edition = useQuery({
    enabled: !!editionId,
    queryKey: ["reporting-edition", editionId],
    queryFn: async () => {
      return unwrap(
        await api.GET("/analytics/editions/{id}", {
          params: { path: { id: editionId ?? "" } },
        }),
      );
    },
  });
  const editions = useInfiniteQuery({
    enabled: can.editions,
    queryKey: ["reporting-editions", reportId],
    initialPageParam: firstPage(),
    getNextPageParam: (
      lastPage: components["schemas"]["ReportingEditionList"],
    ) => lastPage.next_cursor,
    queryFn: async ({ pageParam: cursor }) => {
      return unwrap(
        await api.GET("/analytics/reports/{id}/editions", {
          params: { path: { id: reportId }, query: { cursor, limit: 5 } },
        }),
      );
    },
  });
  const schedules = useQuery({
    enabled: can.schedules,
    queryKey: ["reporting-schedules", reportId],
    queryFn: async () => {
      return unwrap(
        await api.GET("/analytics/reports/{id}/schedules", {
          params: { path: { id: reportId } },
        }),
      );
    },
  });
  return { report, live, edition, editions, schedules };
}

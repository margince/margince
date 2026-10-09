import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { unwrap } from "./common";

export function useReportingPipelines() {
  return useQuery({
    queryKey: ["reporting-pipelines"],
    queryFn: async () => {
      return unwrap(
        await api.GET("/pipelines", {
          params: { query: {} },
        }),
      );
    },
  });
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
    queryKey: ["reporting-live", reportId],
    queryFn: async () => {
      return unwrap(
        await api.GET("/analytics/reports/{id}/evaluation", {
          params: { path: { id: reportId } },
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

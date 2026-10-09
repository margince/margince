import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { throwProblem } from "./common";

export function useReportingPipelines() {
  return useQuery({
    queryKey: ["reporting-pipelines"],
    queryFn: async () => {
      const { data, error } = await api.GET("/pipelines", {
        params: { query: {} },
      });
      if (error) throwProblem(error);
      return data;
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
      const { data, error } = await api.GET("/analytics/reports/{id}", {
        params: { path: { id: reportId } },
      });
      if (error) throwProblem(error);
      return data;
    },
  });
  const live = useQuery({
    enabled: !editionId,
    queryKey: ["reporting-live", reportId],
    queryFn: async () => {
      const { data, error } = await api.GET(
        "/analytics/reports/{id}/evaluation",
        { params: { path: { id: reportId } } },
      );
      if (error) throwProblem(error);
      return data;
    },
  });
  const edition = useQuery({
    enabled: !!editionId,
    queryKey: ["reporting-edition", editionId],
    queryFn: async () => {
      const { data, error } = await api.GET("/analytics/editions/{id}", {
        params: { path: { id: editionId ?? "" } },
      });
      if (error) throwProblem(error);
      return data;
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
      const { data, error } = await api.GET(
        "/analytics/reports/{id}/editions",
        { params: { path: { id: reportId }, query: { cursor, limit: 5 } } },
      );
      if (error) throwProblem(error);
      return data;
    },
  });
  const schedules = useQuery({
    enabled: can.schedules,
    queryKey: ["reporting-schedules", reportId],
    queryFn: async () => {
      const { data, error } = await api.GET(
        "/analytics/reports/{id}/schedules",
        { params: { path: { id: reportId } } },
      );
      if (error) throwProblem(error);
      return data;
    },
  });
  return { report, live, edition, editions, schedules };
}

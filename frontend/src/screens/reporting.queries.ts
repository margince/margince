import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
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

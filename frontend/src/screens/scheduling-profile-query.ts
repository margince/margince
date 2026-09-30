import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { throwProblem } from "./common";

export function useSchedulingProfile(refreshOnReturn = false) {
  return useQuery({
    queryKey: ["scheduling-profile"],
    refetchOnWindowFocus: refreshOnReturn ? "always" : false,
    queryFn: async () => {
      const { data, error } = await api.GET("/scheduling/profile");
      if (error) throwProblem(error);
      return data;
    },
  });
}

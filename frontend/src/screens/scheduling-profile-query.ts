import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { unwrap } from "./common";

export function useSchedulingProfile(refreshOnReturn = false) {
  return useQuery({
    queryKey: ["scheduling-profile"],
    refetchOnWindowFocus: refreshOnReturn ? "always" : false,
    queryFn: async () => {
      return unwrap(await api.GET("/scheduling/profile"));
    },
  });
}

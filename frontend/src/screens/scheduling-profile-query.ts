import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { throwProblem } from "./common";

type Profile = components["schemas"]["SchedulingProfile"];

export function useSchedulingProfile({
  refreshOnReturn = false,
  enabled = true,
}: Readonly<{ refreshOnReturn?: boolean; enabled?: boolean }> = {}) {
  return useQuery({
    queryKey: ["scheduling-profile"],
    enabled,
    refetchOnWindowFocus: refreshOnReturn ? "always" : false,
    queryFn: async () => {
      const { data, error } = await api.GET("/scheduling/profile");
      if (error) throwProblem(error);
      return data;
    },
  });
}

/**
 * Writes some fields of the booking profile over the server's latest copy, so a
 * card saving one setting never puts back what another card or tab changed.
 */
export async function putSchedulingChanges(
  changes: Partial<Profile>,
): Promise<Profile> {
  const latest = await api.GET("/scheduling/profile");
  if (latest.error) throwProblem(latest.error);
  const { data, error } = await api.PUT("/scheduling/profile", {
    body: { ...latest.data, ...changes },
  });
  if (error) throwProblem(error);
  return data;
}

export function usePatchSchedulingProfile() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: putSchedulingChanges,
    onSuccess: async (profile) => {
      client.setQueryData(["scheduling-profile"], profile);
      await client.invalidateQueries({ queryKey: ["reliable-availability"] });
    },
  });
}

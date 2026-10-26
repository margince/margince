import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { unwrap } from "./problem";

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
      return unwrap(await api.GET("/scheduling/profile"));
    },
  });
}

/**
 * Writes some fields of the booking profile over the server's latest copy.
 * A card saving one setting then never puts back what another card or tab
 * changed.
 */
export async function putSchedulingChanges(
  changes: Partial<Profile>,
): Promise<Profile> {
  const latest = unwrap(await api.GET("/scheduling/profile"));
  return unwrap(
    await api.PUT("/scheduling/profile", {
      body: { ...latest, ...changes },
    }),
  );
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

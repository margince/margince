import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { LEAD_LIST_KEY } from "../../lib/leadkeys";
import { throwProblem, unwrap } from "../../lib/problem";

// Lead handling read from the server: the reasons a lead may be closed with,
// and whether the first-response target is tracked at all. Where a lead came
// from is in leadorigins.ts. Every lead surface reads these through here so the
// detail page and the settings cards cannot disagree about the list.

export type LeadDisqualifyReason =
  components["schemas"]["LeadDisqualifyReason"];
export type LeadSettings = components["schemas"]["LeadSettings"];

export const LEAD_DISQUALIFY_REASONS_KEY = ["lead-disqualify-reasons"] as const;
export const LEAD_SETTINGS_KEY = ["lead-settings"] as const;

export function useLeadDisqualifyReasons() {
  return useQuery({
    queryKey: LEAD_DISQUALIFY_REASONS_KEY,
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/lead-disqualify-reasons",
      );
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data.data;
    },
  });
}

export function useLeadSettings() {
  return useQuery({
    queryKey: LEAD_SETTINGS_KEY,
    queryFn: async () => {
      const { data, error, response } = await api.GET("/leads/settings");
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
  });
}

export function useUpdateLeadSettings() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (
      body: components["schemas"]["UpdateLeadSettingsRequest"],
    ) => {
      return unwrap(await api.PATCH("/leads/settings", { body }));
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: LEAD_SETTINGS_KEY });
      // The list renders the SLA column and the Overdue view off this
      // setting, so the leads it already holds are stale the moment it flips.
      void queryClient.invalidateQueries({ queryKey: LEAD_LIST_KEY });
    },
  });
}

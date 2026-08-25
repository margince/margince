import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { throwProblem } from "../common";

// Saving the member's own LinkedIn answer (ADR-0078 §2.1b).
//
// The onboarding act asks for a profile URL and an authorization. Before this
// existed the question was asked and the answer thrown away, so the settings
// tab could not show a member what the CRM believed about their own account,
// and a reload lost the consent entirely.
//
// ONE hook, for the two places that save it: the onboarding act and the
// settings row. There were two, and they had already drifted — the settings
// copy could not carry an authorization and the onboarding copy left the
// settings tab reading a stale account until something else happened to
// invalidate it.

type LinkedInAccount = components["schemas"]["LinkedInAccount"];

/**
 * The key the member's own account is cached under. It lives beside the WRITE
 * so a save cannot land under a key the reader is not watching — the failure
 * that made a saved profile invisible until a reload.
 */
export const LINKEDIN_ACCOUNT_KEY = ["linkedin-account"] as const;

/**
 * useSaveLinkedInAccount writes the caller's own LinkedIn profile.
 *
 * `connected` is an INPUT rather than a constant, because the two callers are
 * answering different questions: the onboarding act carries the authorization
 * the member just gave, and an edit of the URL alone carries `false`. Passing
 * `false` never revokes — the store keeps an existing authorization, so
 * correcting a typo is not disconnecting.
 */
export function useSaveLinkedInAccount() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      profileUrl: string;
      connected: boolean;
    }): Promise<LinkedInAccount> => {
      const { data, error } = await api.PUT("/me/linkedin-account", {
        body: { profile_url: input.profileUrl, connected: input.connected },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    // The server's answer IS the account now, whichever surface asked. Written
    // rather than invalidated: the response carries the stored row, so a
    // refetch would ask again for something already in hand.
    onSuccess: (data) => client.setQueryData(LINKEDIN_ACCOUNT_KEY, data),
  });
}

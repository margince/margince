// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { unwrap } from "./problem";
import { INSTALLATION_SETTINGS_KEY } from "./uploadlimit";

/** The narrow sign-in read, keyed apart from the installation aggregate. */
export const AUTHENTICATION_POLICY_KEY = ["authentication-policy"] as const;

type SignInPolicyChange = Pick<
  components["schemas"]["UpdateInstallationSettingsRequest"],
  "enabled_oidc_providers" | "oidc_group_role_map" | "require_mfa"
>;

export function useSaveSignInPolicy() {
  const queryClient = useQueryClient();
  return useMutation({
    // The change is a variable rather than closed-over render state, so a click
    // cannot submit a value older than the control it came from.
    mutationFn: async (change: SignInPolicyChange) => {
      unwrap(await api.PATCH("/installation/settings", { body: change }));
    },
    onSuccess: async () => {
      // Both caches: the aggregate holds a copy of these settings, and the
      // card reads the narrow projection.
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: INSTALLATION_SETTINGS_KEY }),
        queryClient.invalidateQueries({ queryKey: AUTHENTICATION_POLICY_KEY }),
      ]);
    },
  });
}

/** The Google directory connection's status, keyed apart from the policy. */
export const GOOGLE_DIRECTORY_KEY = [
  "authentication-policy",
  "google-directory",
] as const;

/** Whether the connection is set up and how its newest check went. */
export function useGoogleDirectory(enabled: boolean) {
  return useQuery({
    queryKey: GOOGLE_DIRECTORY_KEY,
    enabled,
    queryFn: async () =>
      unwrap(
        await api.GET("/installation/authentication-policy/google-directory"),
      ),
  });
}

/**
 * Stores the credential (the server checks it against Google first) or removes
 * it. Never cached: the variables hold the key file.
 */
export function useChangeGoogleDirectory() {
  const queryClient = useQueryClient();
  const refresh = () =>
    queryClient.invalidateQueries({ queryKey: GOOGLE_DIRECTORY_KEY });
  const store = useMutation({
    gcTime: 0,
    mutationFn: async (credential: {
      service_account_json: string;
      admin_subject: string;
    }) =>
      unwrap(
        await api.PUT("/installation/authentication-policy/google-directory", {
          body: credential,
        }),
      ),
    onSuccess: refresh,
  });
  const remove = useMutation({
    mutationFn: async () => {
      unwrap(
        await api.DELETE(
          "/installation/authentication-policy/google-directory",
        ),
      );
    },
    onSuccess: refresh,
  });
  return { store, remove };
}

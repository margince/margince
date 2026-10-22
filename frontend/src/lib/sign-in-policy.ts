// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
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

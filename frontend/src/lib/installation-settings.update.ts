// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useT } from "../i18n";
import {
  LANGUAGE_DEPENDENT_QUERY_PREFIXES,
  SETTINGS_IN_SERVER_TEXT,
} from "./aicaches";
import { unwrap } from "./problem";
import { INSTALLATION_SETTINGS_KEY } from "./uploadlimit";

type Patch = components["schemas"]["UpdateInstallationSettingsRequest"];

// Settings and onboarding share this mutation and its cache invalidation.
export function useUpdateInstallationSettings(onSaved?: () => void) {
  const t = useT();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (patch: Patch) => {
      // Thrown as a ProblemError, so a 422's `details.errors[]` lands on the field
      // it names and the failure sink does not log a refusal as a bug.
      const data = unwrap(
        await api.PATCH("/installation/settings", {
          body: patch,
        }),
        t,
      );
      return data;
    },
    onSuccess: (data, patch) => {
      queryClient.setQueryData(INSTALLATION_SETTINGS_KEY, data);
      // These texts are cached by record id alone, and focus does not refetch.
      // An open page would keep text written in the old language or zone.
      // The switch changes what every reading may show, so nothing cached
      // under the old answer may be drawn again.
      if (patch.payment_long_overdue_days !== undefined) {
        void queryClient.resetQueries({ queryKey: ["customer-payments"] });
      }
      if (
        patch.individual_performance !== undefined ||
        patch.forecast_self_correcting !== undefined ||
        patch.forecast_likely_threshold !== undefined
      ) {
        void queryClient.invalidateQueries();
      }
      if (SETTINGS_IN_SERVER_TEXT.some((key) => patch[key] !== undefined)) {
        for (const prefix of LANGUAGE_DEPENDENT_QUERY_PREFIXES) {
          void queryClient.invalidateQueries({ queryKey: prefix });
        }
      }
      onSaved?.();
    },
    // Refetch after refusal so every form shows the saved settings.
    onError: () => {
      void queryClient.invalidateQueries({
        queryKey: ["installation-settings"],
      });
    },
  });
}

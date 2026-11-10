// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { throwProblem } from "../../lib/problem";

// The shell gate and checkpoint writer share live state; restore keeps its mount snapshot.
export const ONBOARDING_PROGRESS_KEY = ["onboarding-progress"] as const;

export async function loadWizardState(): Promise<
  components["schemas"]["OnboardingState"] | null
> {
  const { data, error, response } = await api.GET("/onboarding/state");
  if (error) {
    if (response.status === 404) return null;
    throwProblem(error);
  }
  return data;
}

export function useOnboardingProgress(enabled: boolean) {
  return useQuery({
    queryKey: ONBOARDING_PROGRESS_KEY,
    enabled,
    queryFn: loadWizardState,
  });
}

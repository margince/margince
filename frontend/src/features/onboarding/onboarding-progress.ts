// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { queryOptions, useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { throwProblem } from "../../lib/problem";
import { signInShowsRefusal } from "../../lib/session.read";

async function loadWizardState(): Promise<
  components["schemas"]["OnboardingState"] | null
> {
  const { data, error, response } = await api.GET("/onboarding/state");
  if (response.status === 204) return null;
  if (error) throwProblem(error);
  return data ?? null;
}

// The shell gate and checkpoint writer share live state; restore keeps its mount snapshot.
export const onboardingProgressOptions = queryOptions({
  queryKey: ["onboarding-progress"],
  meta: signInShowsRefusal,
  queryFn: loadWizardState,
});

export function useOnboardingProgress(enabled: boolean) {
  return useQuery({ ...onboardingProgressOptions, enabled });
}

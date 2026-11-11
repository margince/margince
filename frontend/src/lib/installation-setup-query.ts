// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { throwProblem } from "./problem";

type Setup = components["schemas"]["InstallationSetup"];

/** What the installation still has to be configured with, as the server
 *  answers it: the first-run gate and the setup checklist read the same one. */
export function useInstallationSetup(enabled = true) {
  return useQuery({
    queryKey: ["installation-setup"],
    enabled,
    queryFn: async (): Promise<Setup> => {
      const { data, error, response } = await api.GET("/installation/setup");
      if (error || !response.ok || !data) {
        throwProblem(error);
      }
      return data;
    },
  });
}

/** Whether the installation can call a model: a binding, and a key for it. */
export function modelBound(setup: Setup | undefined): boolean {
  return (
    setup?.steps?.some((s) => s.step === "ai_models" && s.configured) ?? false
  );
}

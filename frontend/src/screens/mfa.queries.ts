// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { throwProblem } from "./common";

export type MfaStatus = components["schemas"]["MfaStatus"];

export const MY_MFA_KEY = ["me", "mfa"] as const;

/** The reader's own second factor. */
export function useMyMfa(enabled = true) {
  return useQuery({
    queryKey: MY_MFA_KEY,
    enabled,
    queryFn: async () => {
      const { data, error, response } = await api.GET("/me/mfa");
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
  });
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useHoldsAdminRole } from "../lib/capability";
import { unwrap } from "../lib/problem";

type Capabilities = components["schemas"]["CompanyContextCapabilities"];

// The rollout answer every surface that gates on the flag shares — the card
// here and the onboarding journey. Named so a caller can ask
// the cache whether the answer has LANDED, which is a different question from
// whether the request went out.
export const companyContextCapabilitiesQueryKey = [
  "company-context-capabilities",
];

export function useCompanyContextCapabilities(enabled = true) {
  return useQuery({
    queryKey: companyContextCapabilitiesQueryKey,
    enabled,
    queryFn: async (): Promise<Capabilities> => {
      return unwrap(await api.GET("/company/context/capabilities"));
    },
  });
}

/** Whether the company-context card draws: for an admin, unless the rollout
 *  has turned reading it off. Before the answer lands, the card draws. */
export function companyContextShown(
  holdsAdminRole: boolean,
  capabilities: Capabilities | undefined,
): boolean {
  return holdsAdminRole && capabilities?.read_enabled !== false;
}

/** `companyContextShown` for the reader signed in. */
export function useCompanyContextShown(): boolean {
  const capabilities = useCompanyContextCapabilities();
  return companyContextShown(useHoldsAdminRole(), capabilities.data);
}

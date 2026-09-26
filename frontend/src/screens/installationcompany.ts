// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useHoldsAdminRole } from "../app/capability";
import { throwProblem, useMe } from "./common";

type CompanyProfile = components["schemas"]["CompanyProfile"];

// useCompany reads the installation's own company, or null when it has not
// saved one yet: GET /company 404s until a human does, and that 404 IS the
// onboarding signal — there is no separate "onboarded" flag that could drift
// from the records it claims to describe. The app shell's gate, the journey
// and the settings card share it, so one cache entry answers all of them.
// Only an admin asks: the read is admin-only (contacts'
// requireAnchorAdministrator), so any other seat's request can only be refused.
export function useCompany(enabled: boolean) {
  const isAdmin = useHoldsAdminRole();
  return useQuery({
    queryKey: ["company"],
    enabled: enabled && isAdmin,
    queryFn: async (): Promise<CompanyProfile | null> => {
      const { data, error, response } = await api.GET("/company");
      if (error) {
        if (response.status === 404) {
          return null;
        }
        throwProblem(error);
      }
      return data;
    },
  });
}

// Whether the installation has described itself, or undefined until that is
// known. An admin learns it from GET /company. Any other seat implies it: the
// identity invite and former-seat paths refuse while no anchor company exists.
// `pending` is a read in flight: a seat that may not ask leaves it idle, and an
// idle read never answers.
export function useInstallationDescribed(enabled: boolean) {
  const session = useMe();
  const isAdmin = useHoldsAdminRole();
  const company = useCompany(enabled);
  let described: boolean | undefined;
  if (enabled && session.data !== undefined && !isAdmin) {
    described = true;
  } else if (company.isSuccess) {
    described = company.data !== null;
  }
  const pending = company.isPending && company.fetchStatus !== "idle";
  return { company, described, pending };
}

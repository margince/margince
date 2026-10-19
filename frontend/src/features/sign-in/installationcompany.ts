// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type QueryClient, useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import type { MessageKey } from "../../i18n/en";
import { useHoldsAdminRole } from "../../lib/capability";
import {
  problemFieldErrorsOf,
  problemMessageOf,
  throwProblem,
} from "../../lib/problem";
import { useMe } from "../../lib/session";

type CompanyProfile = components["schemas"]["CompanyProfile"];

// useCompany reads the installation's own company, or null when it has not
// saved one yet: GET /company 404s until a human does, and that 404 IS the
// onboarding signal — there is no separate "onboarded" flag that could drift
// from the records it claims to describe. The app shell's gate, the journey
// and the settings card share it, so one cache entry answers all of them.
// The rail's brand block does not: it reads /me's installation_brand, which
// every seat receives. Only an admin asks: the read is admin-only (contacts'
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

// storeCompany writes a saved profile into the company entry and refreshes /me,
// whose installation_brand is the rail's copy of the same name and marks.
export function storeCompany(
  client: QueryClient,
  profile: CompanyProfile | null,
): void {
  client.setQueryData(["company"], profile);
  void client.invalidateQueries({ queryKey: ["me"] });
}

// A site-read confirm nests the profile, so its name is `profile.display_name`.
const COMPANY_NAME_FIELDS = new Set(["display_name", "profile.display_name"]);

// The name is the installation's too, so the server caps its length. That
// refusal reads in the reader's language; any other keeps the server's words.
export function companySaveMessage(
  error: unknown,
  t: (key: MessageKey) => string,
): string {
  const tooLong = problemFieldErrorsOf(error).some(
    (fault) =>
      COMPANY_NAME_FIELDS.has(fault.field) && fault.code === "too_long",
  );
  return tooLong
    ? t("settings.companyNameTooLong")
    : problemMessageOf(error, t);
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

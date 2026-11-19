// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type QueryClient,
  queryOptions,
  useQuery,
} from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import type { MessageKey } from "../i18n/en";
import { useHoldsAdminRole } from "./capability";
import { coldFieldLabel } from "./coldfieldlabels";
import {
  type FieldLabels,
  problemMessageOf,
  type RefusalSentences,
  refusalSentences,
  throwProblem,
} from "./problem";
import { useMe } from "./session";

type CompanyProfile = components["schemas"]["CompanyProfile"];

// useCompany reads the installation's own company, or null when it has not
// saved one yet: GET /company 404s until a human does, and that 404 IS the
// onboarding signal — there is no separate "onboarded" flag that could drift
// from the records it claims to describe. The app shell's gate, the journey
// and the settings card share it, so one cache entry answers all of them.
// The rail's brand block does not: it reads /me's installation_brand, which
// every seat receives. Only an admin asks: the read is admin-only (contacts'
// requireAnchorAdministrator), so any other seat's request can only be refused.
export const companyOptions = queryOptions({
  queryKey: ["company"],
  queryFn: async (): Promise<CompanyProfile | null> => {
    const { data, error, response } = await api.GET("/company");
    if (error) {
      if (response.status === 404) return null;
      throwProblem(error);
    }
    return data;
  },
});

export function useCompany(enabled: boolean) {
  const isAdmin = useHoldsAdminRole();
  return useQuery({ ...companyOptions, enabled: enabled && isAdmin });
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

// The contract's maxLength for the company's display_name, which is also the
// installation's name.
export const COMPANY_NAME_MAX = 200;

// The profile fields a refusal can name, by wire path. A site-read confirm
// nests the profile, so its name is `profile.display_name`.
function companyLabels(t: (key: MessageKey) => string): FieldLabels {
  const labels: Record<string, string> = {};
  for (const field of COMPANY_FIELDS) {
    const label = coldFieldLabel(field, t);
    labels[field] = label;
    labels[`profile.${field}`] = label;
  }
  return labels;
}

const COMPANY_FIELDS = ["display_name", "offer_summary", "icp"] as const;

/**
 * A refused profile write split between its fields and the foot, keyed by the
 * form's own field: a too-long name names its limit on the name.
 */
export function companyRefusal(
  error: unknown,
  t: (key: MessageKey) => string,
): RefusalSentences {
  const refused = refusalSentences(error, t, companyLabels(t));
  const byField = new Map<string, string>();
  for (const [wire, sentence] of refused.byField) {
    byField.set(wire.replace(/^profile\./, ""), sentence);
  }
  return { byField, rest: refused.rest };
}

// One sentence for a surface with no field to put the refusal on.
export function companySaveMessage(
  error: unknown,
  t: (key: MessageKey) => string,
): string {
  const refused = companyRefusal(error, t);
  return (
    [...refused.byField.values()][0] ??
    refused.rest ??
    problemMessageOf(error, t)
  );
}

// A served brand proves the anchor company exists; an absent field can also
// mean an older server, so it still needs the profile read.
export function sessionHasCompany(
  me: components["schemas"]["MeResponse"] | undefined,
): boolean {
  return me?.installation_brand != null;
}

export function useInstallationDescribed(
  enabled: boolean,
  { readProfile = true }: Readonly<{ readProfile?: boolean }> = {},
) {
  const session = useMe();
  const isAdmin = useHoldsAdminRole();
  const knownFromSession = !readProfile && sessionHasCompany(session.data);
  const company = useCompany(enabled && !knownFromSession);
  let described: boolean | undefined;
  if (enabled && session.data !== undefined && (!isAdmin || knownFromSession)) {
    described = true;
  } else if (company.isSuccess) {
    described = company.data !== null;
  }
  const pending = company.isPending && company.fetchStatus !== "idle";
  return { company, described, pending };
}

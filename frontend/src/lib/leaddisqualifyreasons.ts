// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useT } from "../i18n";
import { throwProblem } from "./problem";
import { withShippedLabels } from "./shippedlabels";

// The reasons a lead may be closed with, in the reader's language. The lead
// pages, the settings card and a record's history all read them here, so a
// reason reads the same wherever it is named.
export type LeadDisqualifyReason =
  components["schemas"]["LeadDisqualifyReason"];

export const LEAD_DISQUALIFY_REASONS_KEY = ["lead-disqualify-reasons"] as const;

export function useLeadDisqualifyReasons(enabled = true) {
  const t = useT();
  return useQuery({
    queryKey: LEAD_DISQUALIFY_REASONS_KEY,
    enabled,
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/lead-disqualify-reasons",
      );
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data.data;
    },
    select: (reasons) => withShippedLabels(reasons, t),
  });
}

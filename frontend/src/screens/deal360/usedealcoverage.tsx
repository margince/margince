// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The one read of a deal's coverage.
//
// Three surfaces on the deal page ask the same question — the readings band,
// the signal chips inside Deal360, and the coverage card below it. Each used to
// spell its own useQuery over the same key, which react-query serves from one
// cache entry, so they agreed by luck rather than by construction: the first
// one to change its `enabled`, its error handling or its withheld reading would
// have made two surfaces disagree about the same deal with nothing to notice.
//
// WITHHELD IS NOT EMPTY, and that is the whole reason this returns a flag
// rather than a list. A caller without the relationship grant is served no
// seats and no risks at all; a surface that read that as "nothing is wrong"
// would report a clean bill of health from a check that never ran. The contract
// withholds all three sections together, so one flag is the honest shape.

import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { DEAL_COVERAGE_KEY } from "../activitykeys";
import { throwProblem } from "../common";

type DealCoverage = components["schemas"]["DealCoverage"];

/**
 * The cache key every coverage reader shares.
 *
 * Built on the prefix the WRITERS name (activitykeys.DEAL_COVERAGE_KEY): a
 * stakeholder edge is created from the person's page as readily as from the
 * deal's, so the writer usually knows only that some deal's seats moved and
 * drops the prefix. Two spellings of it would leave the map on this page
 * confidently stale.
 */
export function dealCoverageKey(dealId: string) {
  return [...DEAL_COVERAGE_KEY, dealId] as const;
}

/**
 * useDealCoverage reads who is on a deal and what is wrong with that.
 *
 * `enabled` is the caller's: overlay mode serves a mirrored deal whose
 * coverage this installation cannot assemble, and a doomed fetch there would
 * render as "nobody is on this deal".
 */
export function useDealCoverage(dealId: string, enabled: boolean) {
  const query = useQuery({
    queryKey: dealCoverageKey(dealId),
    queryFn: async (): Promise<DealCoverage> => {
      const { data, error } = await api.GET("/deals/{id}/coverage", {
        params: { path: { id: dealId } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    enabled,
  });
  return {
    coverage: query.data,
    // The contract withholds stakeholders, our_side and risks together or not
    // at all, so one answer covers every reader.
    withheld: (query.data?.sections_omitted ?? []).length > 0,
    ready: query.isSuccess,
    pending: query.isPending,
    query,
  };
}

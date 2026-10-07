// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCan } from "../app/capability";
import { throwProblem } from "./common";

export type DataCoverageRow =
  components["schemas"]["DataCoverage"]["sources"][number];

// Which connectors the nightly check could read. One key for every reader on
// the page, so the Setup menu and the attention list cannot disagree.
export function useDataCoverage() {
  const allowed = useCan("data_coverage", "read");
  return useQuery({
    enabled: allowed,
    queryKey: ["analytics-coverage"],
    retry: false,
    queryFn: async () => {
      const { data, error, response } = await api.GET("/analytics/coverage");
      if (response.status === 404) {
        // A fresh installation: no run has completed yet. Null, so the view
        // says that in words rather than drawing headers over blank space —
        // "nothing has looked yet" and "everything looked fine" are opposite
        // instructions about whether to trust the numbers elsewhere.
        return null;
      }
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}

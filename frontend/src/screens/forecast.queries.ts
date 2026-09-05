// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The forecast readings, read once and shared.
//
// Two surfaces ask what the pipeline is worth: Analytics, where a manager sets
// a call against it, and the morning's own counter, which says where the reader
// stands before they start. They must not be two reads with two answers, so the
// query lives here and both call it — one key, one cache entry, one figure.

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { type AnalyticsScope, scopeKey, scopeQuery } from "./analytics.context";
import { throwProblem } from "./common";

export type ForecastReadings = components["schemas"]["ForecastReadings"];

/**
 * What the pipeline is worth, for one scope.
 *
 * The population is part of the key: without it a scope change would show the
 * previous population's numbers under the new one's name.
 */
export function useForecastReadings(scope: AnalyticsScope) {
  return useQuery({
    queryKey: ["forecast", scopeKey(scope)],
    queryFn: async () => {
      const { data, error } = await api.GET("/forecast", {
        params: { query: scopeQuery(scope) },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}

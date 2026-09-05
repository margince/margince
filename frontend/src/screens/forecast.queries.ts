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
 *
 * `undefined` is a real argument and means the reader's own default, which only
 * the server can name — `useAnalyticsContext` answers it as `default_scope`.
 * The query does not run until it arrives, because a read sent without it would
 * ask a different question and land under a different key.
 */
export function useForecastReadings(scope: AnalyticsScope | undefined) {
  return useQuery({
    queryKey: ["forecast", scope ? scopeKey(scope) : "unresolved"],
    queryFn: async () => {
      if (!scope) {
        // Unreachable: `enabled` holds the query until the scope is known. The
        // throw is here because a queryFn must return a value, and returning a
        // shaped nothing would put an empty pipeline in the cache under a key a
        // later read would hit.
        throw new Error("the forecast scope is not resolved yet");
      }
      const { data, error } = await api.GET("/forecast", {
        params: { query: scopeQuery(scope) },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    enabled: scope !== undefined,
  });
}

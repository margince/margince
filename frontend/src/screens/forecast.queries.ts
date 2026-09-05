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
import type { components, operations } from "../api/schema";
import { type AnalyticsScope, scopeKey, scopeQuery } from "./analytics.context";
import { throwProblem } from "./common";

export type ForecastReadings = components["schemas"]["ForecastReadings"];

/**
 * The windows a forecast is read over.
 *
 * Read off the generated operation's own query parameter rather than written
 * out, so a window the server gains is one this client cannot silently fail to
 * offer — the control below maps over these values.
 */
export type ForecastPeriod = NonNullable<
  NonNullable<operations["getForecast"]["parameters"]["query"]>["period"]
>;

/**
 * Every window, in the order the control offers them: longest first, because
 * the quarter is the number a forecast conversation opens on.
 *
 * `satisfies` catches ONE direction — a value here the contract does not admit
 * fails to compile. It does not catch the other: a window the server gains and
 * this list never learns is a subset, which TypeScript is happy with. That gap
 * is closed by backend/gates/forecastperiodparity_test.go, which derives the
 * contract's values and requires the server to resolve every one; a window that
 * reaches the wire without a home is caught there rather than here.
 */
export const FORECAST_PERIODS = [
  "quarter",
  "month",
  "week",
] as const satisfies readonly ForecastPeriod[];

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
export function useForecastReadings(
  scope: AnalyticsScope | undefined,
  period: ForecastPeriod = "quarter",
) {
  return useQuery({
    // The PERIOD is part of the key. Two windows are two answers, and caching
    // them together would show a reader last week's figures under the word
    // quarter for as long as the entry stayed fresh.
    queryKey: ["forecast", scope ? scopeKey(scope) : "unresolved", period],
    queryFn: async () => {
      if (!scope) {
        // Unreachable: `enabled` holds the query until the scope is known. The
        // throw is here because a queryFn must return a value, and returning a
        // shaped nothing would put an empty pipeline in the cache under a key a
        // later read would hit.
        throw new Error("the forecast scope is not resolved yet");
      }
      const { data, error } = await api.GET("/forecast", {
        params: { query: { ...scopeQuery(scope), period } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    enabled: scope !== undefined,
  });
}

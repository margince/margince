// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { queryOptions } from "@tanstack/react-query";
import { api } from "../api/client";
import { ANALYTICS_CONTEXT_KEY } from "./analyticskeys";
import { unwrap } from "./problem";

/**
 * The caller's frame, read once.
 *
 * The server decides which population this colleague measures by default and
 * which ones they may choose; nothing here recreates that policy. A screen
 * asking the question itself would be a second answer, and the two would
 * disagree the first time somebody's grants changed.
 */
export const analyticsContextOptions = queryOptions({
  queryKey: ANALYTICS_CONTEXT_KEY,
  queryFn: async ({ signal }) =>
    unwrap(await api.GET("/analytics/context", { signal })),
});

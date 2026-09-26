// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { throwProblem } from "./common";

/**
 * Every pipeline, stages included, in the server's order.
 *
 * ONE key for the plural read, distinct from the single-pipeline reads under
 * `["pipelines", <id>]`: sharing a key would let the cache hold either shape
 * depending on which screen loaded last. A mutation invalidating the
 * `["pipelines"]` prefix still refreshes this entry.
 *
 * `enabled` is for a surface that reads the list only once it is opened.
 */
export function usePipelines(enabled = true) {
  return useQuery({
    queryKey: ["pipelines", "all"],
    enabled,
    queryFn: async () => {
      const { data, error } = await api.GET("/pipelines", {
        params: { query: {} },
      });
      if (error) {
        throwProblem(error);
      }
      return data.data;
    },
  });
}

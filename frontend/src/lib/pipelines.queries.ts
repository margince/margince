// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { queryOptions, useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useLocale } from "../i18n";
import { translatedPipeline } from "./labeltranslation";
import { unwrap } from "./problem";

/** The pipeline a reader is on when nothing names one. */
export function defaultPipelineOf(
  pipelines: readonly components["schemas"]["Pipeline"][],
): components["schemas"]["Pipeline"] | undefined {
  return pipelines.find((each) => each.is_default) ?? pipelines[0];
}

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
export const pipelinesOptions = queryOptions({
  queryKey: ["pipelines", "all"],
  queryFn: async ({ signal }) =>
    unwrap(await api.GET("/pipelines", { params: { query: {} }, signal })).data,
});

/**
 * The pipelines with each stage named in the reader's UI language. Settings
 * reads the stored names through its own query, so it edits what is stored.
 */
export function usePipelines(enabled = true) {
  const { locale } = useLocale();
  return useQuery({
    ...pipelinesOptions,
    enabled,
    select: (pipelines) =>
      pipelines.map((pipeline) => translatedPipeline(pipeline, locale)),
  });
}

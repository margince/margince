// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

type Stage = components["schemas"]["Stage"];
type PipelineStages = {
  name?: string;
  is_default?: boolean;
  archived_at?: string | null;
  stages?: readonly Stage[];
};

/**
 * Every pipeline's stages in one ladder, the default pipeline first. A deal
 * report counts deals in all pipelines, so a row can name a stage outside the
 * default one. Each stage's position becomes its place in the ladder, and a
 * name that two pipelines share is prefixed with its pipeline.
 *
 * An archived pipeline's stages come last and always carry its name, so they
 * never rename a live stage that shares theirs.
 */
export function stagesOfEveryPipeline(
  pipelines: readonly PipelineStages[],
): Stage[] {
  const live = pipelines.filter((pipeline) => !pipeline.archived_at);
  const ordered = [
    ...live.filter((pipeline) => pipeline.is_default),
    ...live.filter((pipeline) => !pipeline.is_default),
    ...pipelines.filter((pipeline) => pipeline.archived_at),
  ];
  const owners = new Map<string, Set<PipelineStages>>();
  for (const pipeline of live) {
    for (const stage of pipeline.stages ?? []) {
      owners.set(
        stage.name,
        (owners.get(stage.name) ?? new Set()).add(pipeline),
      );
    }
  }
  const carriesPipeline = (pipeline: PipelineStages, stage: Stage) =>
    Boolean(pipeline.archived_at) || (owners.get(stage.name)?.size ?? 0) > 1;
  return ordered
    .flatMap((pipeline) =>
      [...(pipeline.stages ?? [])]
        .sort((a, z) => a.position - z.position)
        .map((stage) =>
          pipeline.name && carriesPipeline(pipeline, stage)
            ? { ...stage, name: `${pipeline.name} · ${stage.name}` }
            : stage,
        ),
    )
    .map((stage, place) => ({ ...stage, position: place }));
}

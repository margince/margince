// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

type Stage = components["schemas"]["Stage"];
type PipelineStages = {
  name?: string;
  is_default?: boolean;
  stages?: readonly Stage[];
};

/**
 * Every pipeline's stages in one ladder, the default pipeline first. A deal
 * report counts deals in all pipelines, so a row can name a stage outside the
 * default one. Each stage's position becomes its place in the ladder, and a
 * name that two pipelines share is prefixed with its pipeline.
 */
export function stagesOfEveryPipeline(
  pipelines: readonly PipelineStages[],
): Stage[] {
  const ordered = [
    ...pipelines.filter((pipeline) => pipeline.is_default),
    ...pipelines.filter((pipeline) => !pipeline.is_default),
  ];
  const owners = new Map<string, Set<PipelineStages>>();
  for (const pipeline of ordered) {
    for (const stage of pipeline.stages ?? []) {
      owners.set(
        stage.name,
        (owners.get(stage.name) ?? new Set()).add(pipeline),
      );
    }
  }
  return ordered
    .flatMap((pipeline) =>
      [...(pipeline.stages ?? [])]
        .sort((a, z) => a.position - z.position)
        .map((stage) =>
          pipeline.name && (owners.get(stage.name)?.size ?? 0) > 1
            ? { ...stage, name: `${pipeline.name} · ${stage.name}` }
            : stage,
        ),
    )
    .map((stage, place) => ({ ...stage, position: place }));
}

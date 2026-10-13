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
 * An archived pipeline's stages come last under `archivedName`, so they never
 * rename a live stage. Its name is free for a new pipeline to take.
 */
export function stagesOfEveryPipeline(
  pipelines: readonly PipelineStages[],
  archivedName: (name: string) => string,
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
  const ladder = ordered.flatMap((pipeline) => {
    const archived = Boolean(pipeline.archived_at);
    const owner =
      pipeline.name && archived ? archivedName(pipeline.name) : pipeline.name;
    return [...(pipeline.stages ?? [])]
      .sort((a, z) => a.position - z.position)
      .map((stage) =>
        owner && (archived || (owners.get(stage.name)?.size ?? 0) > 1)
          ? { ...stage, name: `${owner} · ${stage.name}` }
          : stage,
      );
  });
  return distinctNames(ladder).map((stage, place) => ({
    ...stage,
    position: place,
  }));
}

// Two pipelines may share a name, so a label two stages still share takes a
// number. No report row may read as another stage's.
function distinctNames(stages: readonly Stage[]): Stage[] {
  const used = new Set<string>();
  return stages.map((stage) => {
    let name = stage.name;
    for (let n = 2; used.has(name); n += 1) {
      name = `${stage.name} (${n})`;
    }
    used.add(name);
    return name === stage.name ? stage : { ...stage, name };
  });
}

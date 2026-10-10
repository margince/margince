// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

type Stage = components["schemas"]["Stage"];

/**
 * Every pipeline's stages in one ladder, the default pipeline first. A deal
 * report counts deals in all pipelines, so a row can name a stage outside the
 * default one. Each stage's position becomes its place in the ladder.
 */
export function stagesOfEveryPipeline(
  pipelines: readonly { is_default?: boolean; stages?: readonly Stage[] }[],
): Stage[] {
  const ordered = [
    ...pipelines.filter((pipeline) => pipeline.is_default),
    ...pipelines.filter((pipeline) => !pipeline.is_default),
  ];
  return ordered
    .flatMap((pipeline) =>
      [...(pipeline.stages ?? [])].sort((a, z) => a.position - z.position),
    )
    .map((stage, place) => ({ ...stage, position: place }));
}

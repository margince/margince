// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

type Stage = components["schemas"]["Stage"];

/**
 * Every pipeline's stages in one ladder, the default pipeline first. A deal
 * report counts deals in all pipelines, so a row can name a stage the default
 * one does not carry; positions are offset per pipeline to keep each ladder
 * together.
 */
export function stagesOfEveryPipeline(
  pipelines: readonly { is_default?: boolean; stages?: readonly Stage[] }[],
): Stage[] {
  const ordered = [
    ...pipelines.filter((pipeline) => pipeline.is_default),
    ...pipelines.filter((pipeline) => !pipeline.is_default),
  ];
  return ordered.flatMap((pipeline, index) =>
    (pipeline.stages ?? []).map((stage) => ({
      ...stage,
      position: index * 1000 + stage.position,
    })),
  );
}

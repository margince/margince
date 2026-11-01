// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The chance the forecast uses for each open stage, beside the typed one.

import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { unwrap } from "../../lib/problem";

export type StageChance = components["schemas"]["ForecastStageChance"];

/** Every open stage's chance in use, keyed by stage id. */
export function useStageChances() {
  return useQuery({
    queryKey: ["forecast", "stage-chances"],
    queryFn: async () => unwrap(await api.GET("/forecast/stage-chances")),
    select: (data) => ({
      selfCorrecting: data.self_correcting,
      byStage: new Map(data.stages.map((stage) => [stage.stage_id, stage])),
    }),
  });
}

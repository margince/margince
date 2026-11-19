// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../../../api/client";
import type { components } from "../../../api/schema";
import { ifMatch, requireVersion } from "../../../api/version";
import { ListPopover } from "../../../design-system/listpopover";
import { useCloseOverflowMenu } from "../../../design-system/overflowmenu";
import { undoAction, useToast } from "../../../design-system/toast";
import { useT } from "../../../i18n";
import { dealStageMoveKeys } from "../../../lib/activitykeys";
import { useCanWriteRecord } from "../../../lib/capability";
import { usePipelines } from "../../../lib/pipelines.queries";
import { problemMessageOf, unwrap } from "../../../lib/problem";

type Deal = components["schemas"]["Deal"];

type Place = Readonly<{
  pipelineId: string;
  pipelineName: string;
  /** Names the stage to land on; absent, the pipeline's first open stage. */
  stageId?: string;
}>;

type Move = Place &
  Readonly<{
    version: number | undefined;
    /** Where the deal stood before the move, which an Undo returns it to. */
    undo?: Place;
  }>;

// The pipeline and stage the deal stands on now, named so a move can return.
function placeOf(
  deal: Deal,
  pipelines: readonly components["schemas"]["Pipeline"][],
): Place | undefined {
  const name = pipelines.find((each) => each.id === deal.pipeline_id)?.name;
  return deal.pipeline_id && deal.stage_id && name
    ? {
        pipelineId: deal.pipeline_id,
        pipelineName: name,
        stageId: deal.stage_id,
      }
    : undefined;
}

/**
 * Moves an open deal to another live pipeline, onto that pipeline's first open
 * stage. The confirmation offers Undo, which puts the deal back on the pipeline
 * and the stage it left.
 */
export function MovePipelineAction({
  deal,
  disabledReasonId,
}: Readonly<{ deal: Deal; disabledReasonId?: string }>) {
  const t = useT();
  const toast = useToast();
  const queryClient = useQueryClient();
  const closeMenu = useCloseOverflowMenu();
  const canWrite = useCanWriteRecord("deal", deal);
  const pipelines = usePipelines().data ?? [];
  const move = useMutation({
    mutationKey: ["deal-edit", deal.id],
    mutationFn: async (input: Move) =>
      unwrap(
        await api.POST("/deals/{id}/move-pipeline", {
          params: {
            path: { id: deal.id },
            ...ifMatch(requireVersion(input.version)),
          },
          body: { pipeline_id: input.pipelineId, stage_id: input.stageId },
        }),
        t,
      ),
    onError: (error) => {
      toast.show(problemMessageOf(error, t), { tone: "danger" });
      queryClient.invalidateQueries({ queryKey: ["pipelines"] });
    },
    onSuccess: (moved, input) => {
      for (const queryKey of dealStageMoveKeys(deal.id)) {
        queryClient.invalidateQueries({ queryKey });
      }
      queryClient.invalidateQueries({ queryKey: ["deals"] });
      const back = input.undo;
      toast.show(t("deal.movePipelineDone", { pipeline: input.pipelineName }), {
        action: back
          ? undoAction(t("common.undo"), () =>
              move.mutate({ ...back, version: moved.version }),
            )
          : undefined,
      });
    },
  });
  // A closed deal keeps its pipeline: reopening it comes first.
  if (!canWrite || deal.status !== "open" || deal.archived_at) {
    return null;
  }
  const others = pipelines.filter((each) => each.id !== deal.pipeline_id);
  return (
    <ListPopover
      label={t("deal.movePipeline")}
      title={t("deal.movePipeline")}
      searchLabel={t("deal.movePipelinePick")}
      reasonId={disabledReasonId}
      options={others.map((each) => ({ id: each.id, name: each.name }))}
      onPick={(option, done) => {
        done();
        closeMenu();
        move.mutate({
          version: deal.version,
          pipelineId: option.id,
          pipelineName: option.name,
          undo: placeOf(deal, pipelines),
        });
      }}
    />
  );
}

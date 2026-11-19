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

type Target = Readonly<{
  version: number | undefined;
  pipelineId: string;
  pipelineName: string;
  /** The pipeline the deal leaves, named in the Undo's confirmation. */
  fromName?: string;
}>;

/**
 * Moves an open deal to another live pipeline, onto that pipeline's first open
 * stage. The confirmation offers Undo, which asks the server to put the deal
 * back where that move took it from.
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
  const settle = () => {
    for (const queryKey of dealStageMoveKeys(deal.id)) {
      queryClient.invalidateQueries({ queryKey });
    }
    queryClient.invalidateQueries({ queryKey: ["deals"] });
  };
  const refuse = (error: unknown) => {
    toast.show(problemMessageOf(error, t), { tone: "danger" });
    queryClient.invalidateQueries({ queryKey: ["pipelines"] });
  };
  const undo = useMutation({
    mutationKey: ["deal-edit", deal.id],
    mutationFn: async (input: Target) =>
      unwrap(
        await api.POST("/deals/{id}/move-pipeline/undo", {
          params: {
            path: { id: deal.id },
            ...ifMatch(requireVersion(input.version)),
          },
        }),
        t,
      ),
    onError: refuse,
    onSuccess: (_moved, input) => {
      settle();
      toast.show(t("deal.movePipelineDone", { pipeline: input.pipelineName }));
    },
  });
  const move = useMutation({
    mutationKey: ["deal-edit", deal.id],
    mutationFn: async (input: Target) =>
      unwrap(
        await api.POST("/deals/{id}/move-pipeline", {
          params: {
            path: { id: deal.id },
            ...ifMatch(requireVersion(input.version)),
          },
          body: { pipeline_id: input.pipelineId },
        }),
        t,
      ),
    onError: refuse,
    onSuccess: (moved, input) => {
      settle();
      const back = input.fromName;
      toast.show(t("deal.movePipelineDone", { pipeline: input.pipelineName }), {
        action: back
          ? undoAction(t("common.undo"), () =>
              undo.mutate({
                version: moved.version,
                pipelineId: deal.pipeline_id ?? "",
                pipelineName: back,
              }),
            )
          : undefined,
      });
    },
  });
  // A closed deal keeps its pipeline: reopening it comes first.
  if (!canWrite || deal.status !== "open" || deal.archived_at) {
    return null;
  }
  const fromName = pipelines.find((each) => each.id === deal.pipeline_id)?.name;
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
          fromName,
        });
      }}
    />
  );
}

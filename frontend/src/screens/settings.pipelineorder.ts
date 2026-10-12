// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The two reorder writes the Pipelines settings make: one pipeline's stage
// ladder, and the pipeline catalog. Both draw the new order BEFORE the request
// leaves, because a row that snapped back to where it was picked up and then
// jumped to where it was dropped reads as a refusal; both put the old order back
// when the server refuses; and both offer the way back in the confirmation.

import {
  type QueryClient,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch, requireVersion } from "../api/version";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { problemCodeOf, problemMessageOf, unwrap } from "./common";

type Pipeline = components["schemas"]["Pipeline"];

// The settings card's own read: every pipeline, retired ones included. Its own
// key, so the pickers reading ["pipelines","all"] never receive retired rows;
// it still sits under the ["pipelines"] prefix every write here invalidates.
export const SETTINGS_PIPELINES = [
  "pipelines",
  "all",
  "including-retired",
] as const;

// The two refusals that mean "your order was drawn from a list that has since
// changed": the answer is the latest list, not a retry of the same order.
const STALE = new Set(["version_skew", "order_stale"]);

type Reorder = Readonly<{
  reorder: (before: Pipeline, order: readonly string[]) => void;
  pending: boolean;
}>;

type CatalogReorder = Readonly<{
  reorder: (before: readonly Pipeline[], order: readonly string[]) => void;
  pending: boolean;
}>;

function settle(queryClient: QueryClient) {
  return queryClient.invalidateQueries({ queryKey: ["pipelines"] });
}

function withStageOrder(
  pipeline: Pipeline,
  order: readonly string[],
): Pipeline {
  const stages = pipeline.stages ?? [];
  return {
    ...pipeline,
    stages: order.flatMap((id, index) => {
      const stage = stages.find((each) => each.id === id);
      return stage ? [{ ...stage, position: index + 1 }] : [];
    }),
  };
}

function withCatalogOrder(
  pipelines: readonly Pipeline[],
  order: readonly string[],
): Pipeline[] {
  const live = order.flatMap((id, index) => {
    const pipeline = pipelines.find((each) => each.id === id);
    return pipeline ? [{ ...pipeline, position: index + 1 }] : [];
  });
  return [...live, ...pipelines.filter((each) => !order.includes(each.id))];
}

function putPipeline(queryClient: QueryClient, next: Pipeline) {
  queryClient.setQueryData<Pipeline[]>(SETTINGS_PIPELINES, (all) =>
    all?.map((each) => (each.id === next.id ? next : each)),
  );
}

function stageIdsOf(pipeline: Pipeline): string[] {
  return [...(pipeline.stages ?? [])]
    .sort((a, b) => a.position - b.position)
    .map((stage) => stage.id);
}

function useRefusal() {
  const t = useT();
  const toast = useToast();
  return (error: unknown) =>
    toast.show(
      STALE.has(problemCodeOf(error) ?? "")
        ? t("pipeline.orderStale")
        : problemMessageOf(error, t, t("pipeline.orderNotSaved")),
      { tone: "danger", sticky: true },
    );
}

/** Put one pipeline's stages in a new order: the whole ladder, closing stages included. */
export function useStageOrder(): Reorder {
  const queryClient = useQueryClient();
  const toast = useToast();
  const t = useT();
  const refuse = useRefusal();
  const mutation = useMutation({
    mutationFn: async (
      move: Readonly<{
        before: Pipeline;
        order: readonly string[];
        undoable: boolean;
      }>,
    ) => {
      return unwrap(
        await api.PUT("/pipelines/{id}/stage-order", {
          params: {
            path: { id: move.before.id },
            // The version the order was drawn from: a ladder somebody else has
            // reshaped since then refuses instead of being overwritten.
            ...ifMatch(requireVersion(move.before.version)),
          },
          body: { stage_ids: [...move.order] },
        }),
      );
    },
    onSuccess: (saved, move) => {
      putPipeline(queryClient, saved);
      settle(queryClient);
      if (!move.undoable) {
        toast.show(t("pipeline.orderRestored"));
        return;
      }
      const previous = stageIdsOf(move.before);
      toast.show(t("pipeline.orderSaved"), {
        action: {
          kind: "undo",
          label: t("common.undo"),
          // The pipeline as it stands when Undo is pressed, not as it stood at
          // the save: its version has moved since, and the way back is drawn
          // from the latest one.
          onAct: () => {
            const now = queryClient
              .getQueryData<Pipeline[]>(SETTINGS_PIPELINES)
              ?.find((each) => each.id === saved.id);
            if (now) {
              run(now, previous, false);
            }
          },
        },
      });
    },
    onError: (error, move) => {
      putPipeline(queryClient, move.before);
      settle(queryClient);
      refuse(error);
    },
  });
  const run = (
    before: Pipeline,
    order: readonly string[],
    undoable: boolean,
  ) => {
    putPipeline(queryClient, withStageOrder(before, order));
    mutation.mutate({ before, order, undoable });
  };
  return {
    reorder: (before, order) => run(before, order, true),
    pending: mutation.isPending,
  };
}

/** Put the pipelines in use in a new order. Retired ones keep no place in it. */
export function usePipelineOrder(): CatalogReorder {
  const queryClient = useQueryClient();
  const toast = useToast();
  const t = useT();
  const refuse = useRefusal();
  const mutation = useMutation({
    mutationFn: async (
      move: Readonly<{
        before: readonly Pipeline[];
        order: readonly string[];
        undoable: boolean;
      }>,
    ) => {
      unwrap(
        await api.PUT("/pipelines/order", {
          body: { pipeline_ids: [...move.order] },
        }),
      );
    },
    onSuccess: (_saved, move) => {
      settle(queryClient);
      if (!move.undoable) {
        toast.show(t("pipeline.orderRestored"));
        return;
      }
      const previous = move.before
        .filter((each) => !each.archived_at)
        .map((each) => each.id);
      toast.show(t("pipeline.orderSaved"), {
        action: {
          kind: "undo",
          label: t("common.undo"),
          onAct: () =>
            run(
              queryClient.getQueryData<Pipeline[]>(SETTINGS_PIPELINES) ??
                move.before,
              previous,
              false,
            ),
        },
      });
    },
    onError: (error, move) => {
      queryClient.setQueryData(SETTINGS_PIPELINES, move.before);
      settle(queryClient);
      refuse(error);
    },
  });
  const run = (
    before: readonly Pipeline[],
    order: readonly string[],
    undoable: boolean,
  ) => {
    queryClient.setQueryData(
      SETTINGS_PIPELINES,
      withCatalogOrder(before, order),
    );
    mutation.mutate({ before, order, undoable });
  };
  return {
    reorder: (before, order) => run(before, order, true),
    pending: mutation.isPending,
  };
}

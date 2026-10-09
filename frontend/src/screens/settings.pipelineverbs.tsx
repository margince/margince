// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The verbs that change a pipeline's STANDING rather than its ladder: making it
// the default, retiring it, and putting a retired one back. Each is pinned to
// the version the reader was shown, because each is a decision about the whole
// pipeline that somebody else may have just changed.

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch, requireVersion } from "../api/version";
import { Button } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { ArchiveAction } from "./archive";
import { unwrap } from "./common";

type Pipeline = components["schemas"]["Pipeline"];

// Retiring a pipeline, and putting one back. The default is refused by the
// server (`default_pipeline_not_archivable`), and the control says so rather
// than letting the reader find out by pressing it: a control blocked by the
// record's STATE stays visible and disabled WITH the reason, because the
// reason is the information, and its remedy is one this same page offers.
export function PipelineRetirement({
  pipeline,
  canRetire,
  canRestore,
  blockedReasonId,
}: Readonly<{
  pipeline: Pipeline;
  canRetire: boolean;
  canRestore: boolean;
  /** The sentence, drawn by the caller, saying why the default cannot retire. */
  blockedReasonId: string;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const toast = useToast();
  const restore = useMutation({
    mutationFn: async (target: Pipeline) => {
      return unwrap(
        await api.POST("/pipelines/{id}/restore", {
          params: { path: { id: target.id } },
        }),
      );
    },
    onSuccess: (restored) => {
      queryClient.invalidateQueries({ queryKey: ["pipelines"] });
      toast.show(t("pipeline.restored", { name: restored.name }));
    },
  });

  if (pipeline.archived_at) {
    if (!canRestore) {
      return null;
    }
    return (
      <>
        <Button
          onClick={() => restore.mutate(pipeline)}
          pending={restore.isPending}
        >
          {t("pipeline.restore")}
        </Button>
        <ErrorLine inline error={restore.error} />
      </>
    );
  }
  if (!canRetire) {
    return null;
  }
  return (
    <ArchiveAction
      label={t("pipeline.retire")}
      confirmText={t("pipeline.retireConfirm", { name: pipeline.name })}
      // The version travels as If-Match: retiring a pipeline somebody has
      // just renamed or made default is a decision about a record the reader
      // was not looking at, and an unpinned DELETE cannot be taken back.
      archive={async () => {
        unwrap(
          await api.DELETE("/pipelines/{id}", {
            params: {
              path: { id: pipeline.id },
              ...ifMatch(requireVersion(pipeline.version)),
            },
          }),
        );
        return pipeline;
      }}
      invalidate="pipelines"
      recordKey="pipeline"
      archivedMessage={t("pipeline.retired.done", { name: pipeline.name })}
      onArchived={() => {}}
      disabledReasonId={pipeline.is_default ? blockedReasonId : undefined}
    />
  );
}

export function MakeDefault({ pipeline }: Readonly<{ pipeline: Pipeline }>) {
  const t = useT();
  const toast = useToast();
  const queryClient = useQueryClient();
  const promote = useMutation({
    mutationFn: async (target: Pipeline) => {
      return unwrap(
        await api.PATCH("/pipelines/{id}", {
          params: {
            path: { id: target.id },
            ...ifMatch(requireVersion(target.version)),
          },
          body: { is_default: true },
        }),
      );
    },
    onSuccess: (promoted) => {
      queryClient.invalidateQueries({ queryKey: ["pipelines"] });
      toast.show(t("pipeline.defaultSet", { name: promoted.name }));
    },
  });
  return (
    <>
      <Button
        onClick={() => promote.mutate(pipeline)}
        pending={promote.isPending}
      >
        {t("pipeline.makeDefault")}
      </Button>
      <ErrorLine inline error={promote.error} />
    </>
  );
}

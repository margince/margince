// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// "I'm ready" on a meeting the reader attends. The server owns the state: the
// row shows what `preparation` says, and the press sends back the revision the
// row was read at, so a meeting that moved since is never confirmed blind.

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import { Badge, Button } from "../../design-system/atoms";
import { useToast } from "../../design-system/toast";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { problemStatus, throwProblem } from "../../lib/problem";
import { type WorklistItem, worklistKey } from "./worklist.queries";

type Preparation = NonNullable<WorklistItem["preparation"]>;

const FACT_KEY: Record<Preparation["changed"][number], MessageKey> = {
  start_time: "worklist.ready.fact.start_time",
  agenda: "worklist.ready.fact.agenda",
  participants: "worklist.ready.fact.participants",
};

/** Whether the row is the reader's own meeting still to confirm. */
export function needsReadiness(item: WorklistItem): boolean {
  return (
    item.source === "meeting" &&
    !item.batch &&
    item.preparation?.state === "needs_review"
  );
}

/** Confirms, or with `undo` takes back, the reader's readiness at `revision`. */
export function useMeetingReady() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      activityId: string;
      revision: string;
      undo?: boolean;
    }) => {
      const body = {
        activity_id: input.activityId,
        preparation_revision: input.revision,
      };
      const { error } = input.undo
        ? await api.POST("/worklist/ready/undo", { body })
        : await api.PUT("/worklist/ready", { body });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: worklistKey });
    },
  });
}

/** The row's "I'm ready", with an undo on the confirmation. */
export function MeetingReadyVerb({ item }: Readonly<{ item: WorklistItem }>) {
  const t = useT();
  const toast = useToast();
  const ready = useMeetingReady();
  const preparation = item.preparation;
  if (!preparation || !needsReadiness(item)) {
    return null;
  }
  const sent = {
    activityId: preparation.activity_id,
    revision: preparation.revision,
  };
  return (
    <Button
      pending={ready.isPending}
      onClick={() =>
        ready.mutate(sent, {
          onSuccess: () =>
            toast.show(t("worklist.ready.done"), {
              action: {
                kind: "undo",
                label: t("worklist.disposition.undo"),
                onAct: () =>
                  ready.mutate(
                    { ...sent, undo: true },
                    {
                      onError: () =>
                        toast.show(t("worklist.ready.undoFailed"), {
                          tone: "danger",
                        }),
                    },
                  ),
              },
            }),
          onError: (error) =>
            toast.show(
              t(
                problemStatus(error) === 409
                  ? "worklist.ready.stale"
                  : "worklist.ready.failed",
              ),
              { tone: "danger" },
            ),
        })
      }
    >
      {t("worklist.ready.confirm")}
    </Button>
  );
}

/**
 * What the reader's own preparation says under the row: a Ready mark once
 * confirmed, or the facts that changed since an older confirmation.
 */
export function PreparationNote({ item }: Readonly<{ item: WorklistItem }>) {
  const t = useT();
  const preparation = item.source === "meeting" ? item.preparation : undefined;
  if (!preparation) {
    return null;
  }
  if (preparation.state === "ready") {
    return (
      <p className="t-caption">
        <Badge tone="success">{t("worklist.ready.badge")}</Badge>
      </p>
    );
  }
  if (preparation.changed.length === 0) {
    return null;
  }
  const facts = preparation.changed.map((fact) => t(FACT_KEY[fact])).join(", ");
  return <p className="t-caption">{t("worklist.ready.changed", { facts })}</p>;
}

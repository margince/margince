// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import { ifMatch } from "../../api/version";
import { undoAction, useToast } from "../../design-system/toast";
import { formatDateTime } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { isVersionSkewOf, problemMessageOf, unwrap } from "../../lib/problem";
import type { ScheduledSend } from "./scheduledqueue";
import type { Move } from "./scheduledsends.reschedule";

type Withdraw = Readonly<{ id: string }>;

/** The queue's two writes, and the one sentence a refusal of either becomes. */
export function useQueueWrites(readerZone: string, onWithdrawn: () => void) {
  const t = useT();
  const { locale } = useLocale();
  const toast = useToast();
  const queryClient = useQueryClient();

  const move = useMutation({
    mutationFn: async ({ id, version, at }: Move) => {
      unwrap(
        await api.PATCH("/scheduled-sends/{id}", {
          // Through `ifMatch`, not a hand-written header: it is the one spelling of
          // the precondition. `src/api/if-match-coverage.test.ts` reads the AST for
          // that call, so a second spelling reads to the gate as no precondition.
          params: { path: { id }, ...ifMatch(version) },
          // The zone travels as a name beside the instant. So a message scheduled
          // across a DST boundary arrives at the wall time the rep meant, not an
          // hour either side.
          body: { scheduled_at: at, scheduled_tz: readerZone },
        }),
        t,
      );
    },
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ["scheduled-sends"] }),
  });

  const withdraw = useMutation({
    mutationFn: async ({ id }: Withdraw) => {
      unwrap(
        await api.POST("/scheduled-sends/{id}/cancel", {
          params: { path: { id } },
        }),
        t,
      );
    },
    onSuccess: () => {
      onWithdrawn();
      queryClient.invalidateQueries({ queryKey: ["scheduled-sends"] });
    },
  });

  // Moving is confirmed by a toast with the way back rather than by a dialog:
  // a moved message can be moved to where it was. The way back is a second
  // write, so it asks the REFRESHED list for the version it must precede —
  // `onSuccess` above has finished invalidating by the time this one runs, and
  // the version the row was drawn with is already spent.
  const moveWithUndo = (input: Move, send: ScheduledSend) => {
    move.mutate(input, {
      onSuccess: () => {
        toast.show(
          t("sched.moved", {
            when: formatDateTime(input.at, locale, readerZone),
          }),
          {
            action: undoAction(t("sched.undo"), () => {
              const current = queryClient
                .getQueryData<ScheduledSend[]>(["scheduled-sends"])
                ?.find((row) => row.id === send.id);
              if (current) {
                move.mutate({
                  id: send.id,
                  version: current.version,
                  at: send.scheduled_at,
                });
              }
            }),
          },
        );
      },
    });
  };

  // A 409 on either write says one thing: the row on screen is not the row on
  // the server. It already fired, somebody withdrew it, or another surface moved
  // it first — and all three want the same next step, which is to read the list
  // again rather than to retry a write against a version that is gone. It is
  // told apart from every other failure because "reload and look" is useless
  // advice for a 500 and the only useful advice here.
  const skew =
    isVersionSkewOf(move.error) || isVersionSkewOf(withdraw.error)
      ? t("sched.skew")
      : null;
  const writeError =
    skew === null
      ? ((move.isError ? problemMessageOf(move.error, t) : null) ??
        (withdraw.isError ? problemMessageOf(withdraw.error, t) : null))
      : null;

  return { move, withdraw, moveWithUndo, skew, writeError };
}

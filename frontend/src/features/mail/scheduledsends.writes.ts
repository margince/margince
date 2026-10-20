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
    mutationFn: async ({ id, version, at, tz }: Move) => {
      return unwrap(
        await api.PATCH("/scheduled-sends/{id}", {
          // Through `ifMatch`, not a hand-written header: it is the one spelling of
          // the precondition. `src/api/if-match-coverage.test.ts` reads the AST for
          // that call, so a second spelling reads to the gate as no precondition.
          params: { path: { id }, ...ifMatch(version) },
          // The zone travels as a name beside the instant. So a message scheduled
          // across a DST boundary arrives at the wall time the rep meant, not an
          // hour either side.
          body: { scheduled_at: at, scheduled_tz: tz ?? readerZone },
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

  // A move is confirmed by a toast with an undo, not by a dialog.
  // The undo is a second write, so it carries the version the move returned.
  // That keeps it from overwriting a move another surface made in between.
  const moveWithUndo = (input: Move, send: ScheduledSend) => {
    move.mutate(input, {
      onSuccess: (moved) => {
        toast.show(
          t("sched.moved", {
            when: formatDateTime(input.at, locale, readerZone),
          }),
          {
            action: undoAction(t("sched.undo"), () => {
              move.mutate({
                id: send.id,
                version: moved.version,
                at: send.scheduled_at,
                tz: send.scheduled_tz,
              });
            }),
          },
        );
      },
    });
  };

  // A 409 on either write means the row on screen is not the row on the server.
  // It already fired, was withdrawn, or another surface moved it first.
  // All three want a re-read, not a retry against a version that is gone.
  // It is told apart because "reload" is useless advice for a 500.
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

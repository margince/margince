// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import { throwProblem } from "../lib/problem";
import { worklistKey } from "./worklist.queries";

/** One row named the way the server names it: its lane and its id. */
export type WorklistRowRef = Readonly<{ source: string; row_id: string }>;

/** The most rows one call to the server takes. */
export const MAX_DONE_ROWS = 200;

/** Rows in runs the server takes in one call each. */
export function chunksOf<T>(rows: readonly T[], size = MAX_DONE_ROWS): T[][] {
  const out: T[][] = [];
  for (let at = 0; at < rows.length; at += size) {
    out.push(rows.slice(at, at + size));
  }
  return out;
}

async function send(rows: readonly WorklistRowRef[], reopen: boolean) {
  const body = { rows: [...rows] };
  const { error } = reopen
    ? await api.POST("/worklist/done/undo", { body })
    : await api.PUT("/worklist/done", { body });
  if (error) {
    throwProblem(error);
  }
}

/**
 * Done on one row or many: off this reader's own list, all or none. More rows
 * than one call takes go in runs, and a run that fails takes back the runs
 * before it. `reopen` is the undo, and takes the same rows back.
 */
export function useMarkRowsDone() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      rows: readonly WorklistRowRef[];
      reopen?: boolean;
    }) => {
      const reopen = input.reopen ?? false;
      const written: WorklistRowRef[] = [];
      for (const run of chunksOf(input.rows)) {
        try {
          await send(run, reopen);
        } catch (error) {
          if (!reopen) {
            for (const back of chunksOf(written)) {
              await send(back, true);
            }
          }
          throw error;
        }
        written.push(...run);
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: worklistKey });
    },
  });
}

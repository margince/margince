// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../../../api/client";
import type { ProjectPhase } from "../../../format/projectphase";
import { problemStatus, unwrap } from "../../../lib/problem";

// A mirror of StartDeliveryForWonDeal (backend projects/delivery.go);
// backend/gates/frontendwinmovesproject_test.go fails either side's drift.
export const MOVED_BY_A_WIN: readonly ProjectPhase[] = [
  "initiative",
  "pursuing",
];
export const MOVED_INTO_BY_A_WIN = "delivering" satisfies ProjectPhase;

// 403: no project grant. 404: out of the reader's row scope. Never a gone project:
// the product only archives one, and a deleted row clears the deal's link.
const UNREADABLE = new Set([403, 404]);

type AskedAdvance = Readonly<{
  dealId: string;
  toStage: Readonly<{ semantic: string }>;
}>;

/**
 * What winning moves. `project` is the project's name, or null for a project
 * the reader cannot read, which the server moves all the same.
 */
export type WinMove = Readonly<{ project: string | null }>;

/**
 * The project that winning this deal moves to delivering, or null when the win
 * moves none. Read fresh at each opening, since either record may change.
 */
export function useProjectMovedByWin(
  pending: AskedAdvance | null,
): WinMove | null {
  const dealId = pending?.dealId ?? null;
  const move = useQuery({
    queryKey: ["deals", dealId, "win-project"],
    enabled: pending?.toStage.semantic === "won" && dealId !== null,
    gcTime: 0,
    staleTime: 0,
    retry: false,
    queryFn: async ({ signal }) => {
      const { project_id: projectId } = unwrap(
        await api.GET("/deals/{id}", {
          signal,
          params: { path: { id: dealId ?? "" } },
        }),
      );
      return projectId ? projectAWinMoves(projectId, signal) : null;
    },
  });
  return move.isFetching ? null : (move.data ?? null);
}

async function projectAWinMoves(
  projectId: string,
  signal: AbortSignal,
): Promise<WinMove | null> {
  try {
    const read = unwrap(
      await api.GET("/projects/{id}", {
        signal,
        params: { path: { id: projectId } },
      }),
    );
    return !read.archived_at && MOVED_BY_A_WIN.includes(read.phase)
      ? { project: read.name }
      : null;
  } catch (error) {
    if (UNREADABLE.has(problemStatus(error))) {
      return { project: null };
    }
    throw error;
  }
}

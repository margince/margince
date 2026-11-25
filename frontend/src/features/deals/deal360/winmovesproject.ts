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

// The project exists and the reader may not read it: no grant, or out of scope.
const UNREADABLE = new Set([403, 404]);

type AskedAdvance = Readonly<{
  dealId: string;
  toStage: Readonly<{ semantic: string }>;
  // Undefined when the opening surface did not hold the deal: it is read first.
  projectId?: string | null;
}>;

/**
 * What winning moves. `project` is the project's name, or null for a project
 * the reader cannot read, which the server moves all the same.
 */
export type WinMove = Readonly<{ project: string | null }>;

/**
 * The project that winning this deal moves to delivering, or null when the win
 * moves none. Read at each opening and never kept, because the phase may
 * change between two wins.
 */
export function useProjectMovedByWin(
  pending: AskedAdvance | null,
): WinMove | null {
  const dealId = pending?.dealId ?? null;
  const heldProjectId = pending?.projectId;
  const move = useQuery({
    queryKey: ["deals", dealId, "win-project", heldProjectId],
    enabled: pending?.toStage.semantic === "won" && dealId !== null,
    gcTime: 0,
    staleTime: 0,
    retry: false,
    queryFn: async ({ signal }) => {
      const projectId =
        heldProjectId === undefined
          ? unwrap(
              await api.GET("/deals/{id}", {
                signal,
                params: { path: { id: dealId ?? "" } },
              }),
            ).project_id
          : heldProjectId;
      return projectId ? projectAWinMoves(projectId, signal) : null;
    },
  });
  return move.data ?? null;
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

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../../../api/client";
import type { ProjectPhase } from "../../../format/projectphase";
import { unwrap } from "../../../lib/problem";

// The phases the server moves a won deal's project out of, into delivering
// (StartDeliveryForWonDeal in the projects module).
const MOVED_BY_A_WIN: readonly ProjectPhase[] = ["initiative", "pursuing"];

type AskedAdvance = Readonly<{
  dealId: string;
  toStage: Readonly<{ semantic: string }>;
}>;

/**
 * The name of the project that winning this deal moves to delivering. Null
 * means the win moves none, or the reader cannot read the project. Read at
 * each opening and never kept, because the phase may change between two wins.
 */
export function useProjectMovedByWin(
  pending: AskedAdvance | null,
): string | null {
  const dealId = pending?.dealId ?? null;
  const project = useQuery({
    queryKey: ["deals", dealId, "win-project"],
    enabled: pending?.toStage.semantic === "won" && dealId !== null,
    gcTime: 0,
    staleTime: 0,
    retry: false,
    queryFn: async ({ signal }) => {
      const deal = unwrap(
        await api.GET("/deals/{id}", {
          signal,
          params: { path: { id: dealId ?? "" } },
        }),
      );
      if (!deal.project_id) {
        return null;
      }
      return unwrap(
        await api.GET("/projects/{id}", {
          signal,
          params: { path: { id: deal.project_id } },
        }),
      );
    },
  });
  const read = project.data;
  return read && !read.archived_at && MOVED_BY_A_WIN.includes(read.phase)
    ? read.name
    : null;
}

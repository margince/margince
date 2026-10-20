// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type UseQueryResult, useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { unwrap } from "../../lib/problem";

export type Team = components["schemas"]["Team"];
/**
 * The workspace's teams. There is no "my teams" read: `/teams` lists every
 * unarchived team any member may see.
 */
export function useTeams(): UseQueryResult<readonly Team[]> {
  return useQuery({
    queryKey: ["teams", "all"],
    queryFn: async (): Promise<readonly Team[]> => {
      const teams: Team[] = [];
      let cursor: string | undefined;
      do {
        const data = unwrap(
          await api.GET("/teams", {
            params: { query: { limit: 100, cursor } },
          }),
        );
        teams.push(...data.data);
        cursor = data.page.has_more
          ? (data.page.next_cursor ?? undefined)
          : undefined;
      } while (cursor);
      return teams;
    },
  });
}

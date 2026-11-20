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

type TeamReader = Readonly<{
  teams: readonly string[];
  team_week?: components["schemas"]["TeamWeekReach"];
}>;

/**
 * The teams a team-week reader picks from, by the rule `team_week` names: every
 * team for an overseer, otherwise the reader's own teams. Whether the reader
 * may open a team's week at all is `team_week` being other than `none`.
 */
export function weekTeamsFor(
  teams: readonly Team[],
  reader: TeamReader | undefined,
): readonly Team[] {
  const reach = reader?.team_week ?? "none";
  return teams.filter(
    (team) => reach === "every_team" || reader?.teams.includes(team.id),
  );
}

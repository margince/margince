// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type UseQueryResult, useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { throwProblem } from "./common";

export type Team = components["schemas"]["Team"];
export type TeamWeeklyReview = components["schemas"]["TeamWeeklyReview"];
export type TeamWeeklyRep = components["schemas"]["TeamWeeklyRep"];
export type TeamWeeklyFocusKind = TeamWeeklyRep["focus_kind"];
/** Which teams' weeks the server opens for this reader, off the worklist read. */
export type TeamWeekReach = NonNullable<
  components["schemas"]["Worklist"]["team_week"]
>;

/**
 * How a read of a team's frozen week can come back with nothing to draw.
 *
 * The two absences are different facts and the screen says different things
 * about them: `forbidden` is a reader who may open no team's week — they lead
 * no team and hold no oversight grant — and `no_snapshot` is a team whose week
 * has not closed yet, or one a lead is not on. A screen that drew one plate over
 * both would tell a lead they lack permission on a Tuesday in their team's first
 * week.
 */
export type TeamWeeklyAbsence = "forbidden" | "no_snapshot";

export type TeamWeeklyAnswer =
  | { kind: "review"; review: TeamWeeklyReview }
  | { kind: "absent"; why: TeamWeeklyAbsence };

/**
 * One team's week as it was measured when the week closed.
 *
 * `week` omitted means the most recent snapshot the team has. Keyed on both, so
 * moving between weeks does not overwrite the cache of either.
 */
export function useTeamWeeklyReview(
  teamId: string | undefined,
  week?: string,
): UseQueryResult<TeamWeeklyAnswer> {
  return useQuery({
    queryKey: ["weekly-review", "team", teamId, week ?? "latest"],
    enabled: Boolean(teamId),
    queryFn: async (): Promise<TeamWeeklyAnswer> => {
      const { data, error, response } = await api.GET("/weekly-reviews/team", {
        params: { query: { team: teamId ?? "", ...(week ? { week } : {}) } },
      });
      if (response.status === 403) {
        return { kind: "absent", why: "forbidden" };
      }
      if (response.status === 404) {
        return { kind: "absent", why: "no_snapshot" };
      }
      if (error) {
        throwProblem(error);
      }
      if (!data) {
        return { kind: "absent", why: "no_snapshot" };
      }
      return { kind: "review", review: data };
    },
  });
}

/**
 * The workspace's teams, for the picker.
 *
 * There is no "my teams" read: `/teams` lists every unarchived team any member
 * may see. Each view's picker narrows it to the teams its own server rule
 * serves — see `morningTeams` and `weekTeams` beside the picker.
 */
export function useTeams(): UseQueryResult<readonly Team[]> {
  return useQuery({
    queryKey: ["teams", "for-weekly"],
    queryFn: async (): Promise<readonly Team[]> => {
      const teams: Team[] = [];
      let cursor: string | undefined;
      do {
        const { data, error } = await api.GET("/teams", {
          params: { query: { limit: 100, cursor } },
        });
        if (error) throwProblem(error);
        teams.push(...data.data);
        cursor = data.page.has_more
          ? (data.page.next_cursor ?? undefined)
          : undefined;
      } while (cursor);
      return teams;
    },
  });
}

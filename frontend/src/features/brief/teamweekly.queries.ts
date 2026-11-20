// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type UseQueryResult, useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { throwProblem } from "../../lib/problem";

export type TeamWeeklyReview = components["schemas"]["TeamWeeklyReview"];
export type TeamWeeklyRep = components["schemas"]["TeamWeeklyRep"];
export type TeamWeeklyFocusKind = TeamWeeklyRep["focus_kind"];
/** Which teams' weeks the server opens for this reader, off `GET /me`. */
export type TeamWeekReach = components["schemas"]["TeamWeekReach"];

/**
 * How a read of a team's frozen week can come back with nothing to draw.
 *
 * `forbidden` is a reader who may open no team's week any more, such as a lead
 * whose role changed while the page was open. `no_snapshot` is a team whose
 * first week has not closed yet, or one the lead has left. A lead on a team's
 * first Tuesday must not be told they lack permission.
 */
export type TeamWeeklyAbsence = "forbidden" | "no_snapshot";

export type TeamWeeklyAnswer =
  | { kind: "review"; review: TeamWeeklyReview }
  | { kind: "absent"; why: TeamWeeklyAbsence };

/**
 * One team's week as it was measured when the week closed. `week` omitted is
 * the team's most recent snapshot.
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
      if (response.status === 403) return { kind: "absent", why: "forbidden" };
      if (response.status === 404)
        return { kind: "absent", why: "no_snapshot" };
      if (error) throwProblem(error);
      if (!data) return { kind: "absent", why: "no_snapshot" };
      return { kind: "review", review: data };
    },
  });
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { useUrlParams } from "../app/urlstate";
import { Select } from "../design-system/select";
import { SurfaceState } from "../design-system/surfacestate";
import { useT } from "../i18n";
import { useMe } from "./common";
import { type TeamWeekReach, useTeams } from "./teamweekly.queries";

/** Which teams a picker lists: every team, or the ones the reader is on. */
export type TeamList = "every" | "mine";

/**
 * Morning's rule. The team board is served on row scope: a team-scoped seat
 * reads the teams it is on, and any wider seat reads every team.
 */
export function morningTeams(rowScope: string | undefined): TeamList {
  return rowScope === "team" ? "mine" : "every";
}

/**
 * Weekly's rule, as the server answered it. A seat holding the oversight grant
 * opens every team's week; a lead opens the weeks of the teams they are on.
 */
export function weekTeams(reach: TeamWeekReach): TeamList {
  return reach === "every_team" ? "every" : "mine";
}

export function BriefTeamSelect({
  lists,
  children,
}: Readonly<{ lists: TeamList; children: (team: string) => ReactNode }>) {
  const t = useT();
  const teams = useTeams();
  const me = useMe();
  const [params, setParams] = useUrlParams();
  const options = (teams.data ?? [])
    .filter(
      (team) =>
        lists === "every" || (me.data?.teams.includes(team.id) ?? false),
    )
    .map((team) => ({
      value: team.id,
      label: team.name,
    }));
  const asked = params.get("team");
  const chosen = options.some((option) => option.value === asked)
    ? asked
    : !asked && options.length === 1
      ? options[0].value
      : undefined;
  return (
    <SurfaceState
      state={
        teams.isPending
          ? "loading"
          : teams.isError
            ? "failed"
            : options.length === 0
              ? "empty"
              : "ready"
      }
      loadingLabel={t("teamweekly.pickTeam")}
      emptyLabel={t("brief.team.none")}
      detail={{ onRetry: () => void teams.refetch() }}
    >
      <Select
        aria-label={t("teamweekly.pickTeam")}
        placeholder={t("teamweekly.pickTeam")}
        options={options}
        value={chosen ?? ""}
        onChange={(team) => setParams(new Map([...params, ["team", team]]))}
      />
      {chosen ? children(chosen) : <p>{t("teamweekly.chooseTeam")}</p>}
    </SurfaceState>
  );
}

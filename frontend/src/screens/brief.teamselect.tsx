// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { useUrlParams } from "../app/urlstate";
import { Select } from "../design-system/select";
import { SurfaceState } from "../design-system/surfacestate";
import { useT } from "../i18n";
import { useMe } from "./common";
import { useTeams } from "./teamweekly.queries";

export function BriefTeamSelect({
  children,
}: Readonly<{ children: (team: string) => ReactNode }>) {
  const t = useT();
  const teams = useTeams();
  const me = useMe();
  const [params, setParams] = useUrlParams();
  const options = (teams.data ?? [])
    .filter(
      (team) =>
        me.data?.authorization?.row_scope !== "team" ||
        me.data.teams.includes(team.id),
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

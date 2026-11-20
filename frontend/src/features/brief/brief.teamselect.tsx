// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Select } from "../../design-system/select";
import { useT } from "../../i18n";
import { useMe } from "../../lib/session";
import type { BriefAddress } from "./brief.view";
import { useTeams, weekTeamsFor } from "./teams.queries";
import type { TeamWeekReach } from "./teamweekly.queries";

const MINE = "mine";

/**
 * Whose week Weekly shows: the reader's own, or a team's.
 *
 * Drawn only when `team_week` is not `none` and a team is there to pick. A rep
 * is never offered a choice whose every answer is a refusal.
 */
export function WeekTeamSelect({
  team,
  onChange,
}: Readonly<{
  team: string | undefined;
  onChange: (team: string | undefined) => void;
}>) {
  const t = useT();
  const me = useMe();
  const teams = useTeams();
  if (!opensTeamWeeks(me.data?.team_week)) return null;
  const open = weekTeamsFor(teams.data ?? [], me.data);
  if (open.length === 0) return null;
  const options = [
    { value: MINE, label: t("teamweekly.mine") },
    ...open.map((choice) => ({ value: choice.id, label: choice.name })),
  ];
  return (
    <Select
      aria-label={t("teamweekly.whose")}
      options={options}
      placeholder={t("teamweekly.pickTeam")}
      // A team in the address but not on the list stays unpicked, not "My week".
      // The page below shows the server's refusal for it.
      value={
        team === undefined
          ? MINE
          : open.some((choice) => choice.id === team)
            ? team
            : ""
      }
      onChange={(value) => onChange(value === MINE ? undefined : value)}
    />
  );
}

/** Whether the server opens any team's week for this reader. */
export function opensTeamWeeks(reach: TeamWeekReach | undefined): boolean {
  return reach !== undefined && reach !== "none";
}

/**
 * The team whose week the page shows, or none for the reader's own. A team in
 * the address counts only when `team_week` is not `none`. A rep who follows a
 * lead's link reads their own week instead of a refusal.
 */
export function shownTeam(
  team: string | undefined,
  reach: TeamWeekReach | undefined,
): string | undefined {
  return opensTeamWeeks(reach) ? team : undefined;
}

// The address Home draws: a team named in it counts only for a reader who may open team weeks.
export function shownAddress(
  asked: BriefAddress,
  reach: TeamWeekReach | undefined,
): BriefAddress {
  return { ...asked, team: shownTeam(asked.team, reach) };
}

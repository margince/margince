// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import {
  RosterPartialNote,
  useRosterPartial,
  useRosterWalk,
} from "../../components/roster";
import { QueryStates } from "../../design-system/querystates";
import { SetPicker, type SetPickerOption } from "../../design-system/setpicker";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { useCreateTeam } from "./team-dialogs";

type Team = components["schemas"]["Team"];

/**
 * The teams a member is on, in the invite and edit dialogs. One field for both,
 * so the two dialogs offer the same list with the same words.
 */
export function MemberTeamsField({
  value,
  onChange,
  canManage,
  joinable,
}: Readonly<{
  value: readonly string[];
  onChange: (next: string[]) => void;
  /** Only an admin places a member on a team or creates one. */
  canManage: boolean;
  /** Absent, every team may be joined; a deactivated member may only leave. */
  joinable?: (teamId: string) => boolean;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const roster = useRosterWalk("team", true);
  const partial = useRosterPartial("team", true);
  const teams = roster.data?.entries.filter(isTeam) ?? [];
  const create = useCreateTeam();
  const names = new Map(teams.map((team) => [team.id, team.name]));
  const options: SetPickerOption[] = teams.map((team) => {
    const parent = team.parent_team_id
      ? names.get(team.parent_team_id)
      : undefined;
    const count = team.member_count;
    return {
      value: team.id,
      label: team.name,
      detail: [
        parent ? t("users.teamParent", { name: parent }) : "",
        count === undefined
          ? ""
          : plural("users.teamMemberCount", count, {
              count: formatNumber(count, locale),
            }),
      ]
        .filter(Boolean)
        .join(" · "),
      disabled: joinable ? !joinable(team.id) : false,
    };
  });
  // "No teams yet" is a claim about the workspace, so only a roster read to
  // its end may make it.
  const knownEmpty = roster.isSuccess && teams.length === 0 && !partial;
  return (
    <>
      <QueryStates query={roster} pendingLabel={t("users.teamsLabel")}>
        {null}
      </QueryStates>
      <SetPicker
        legend={t("users.teamsLabel")}
        options={options}
        values={value}
        onChange={onChange}
        disabled={!roster.isSuccess || partial || !canManage}
        none={knownEmpty ? t("users.noTeamsYet") : t("users.teamsNone")}
        addLabel={knownEmpty ? t("users.newTeamOpen") : t("users.teamsAdd")}
        searchLabel={
          knownEmpty ? t("users.teamNameLabel") : t("users.teamsSearch")
        }
        noMatch={t("users.teamsNoMatch")}
        doneLabel={t("users.teamsDone")}
        empty={t("users.teamsEmpty")}
        removeLabel={(item) => t("users.teamsRemove", { team: item.label })}
        create={{
          label: (name) => t("users.teamsCreate", { name }),
          onCreate: (name) =>
            create.mutate(name, {
              onSuccess: (team) => onChange([...value, team.id]),
            }),
          pending: create.isPending,
          error: create.error,
        }}
      />
      <RosterPartialNote partial={partial} />
    </>
  );
}

function isTeam(entry: { id: string }): entry is Team {
  return "name" in entry;
}
